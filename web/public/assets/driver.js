// 司机端页面：档案 → 出车/听单抢单 → 接单大厅 → 行程推进 → 收入与提现。
// 视觉与交互流对齐 Figma 原型（花小猪打车平台 · 三端联调交互原型），
// 数据全部来自真实网关接口；「生成一单」用于在联调时模拟乘客下单。
import { api, fy, esc, toast, randomKey, stateName } from './api.js';

const STORE_KEY = 'huazhu.driverId';

const S = {
  tab: 'hall',
  driverId: localStorage.getItem(STORE_KEY) || '',
  driver: null,
  orders: [],        // 接单大厅 items
  current: null,     // 我当前接下的订单（ACCEPTED / IN_TRIP）
  invite: null,      // 抢单成功待确认（grab 返回的订单，带 version）
  todayDone: 0,
  todayIncome: 0,
  flows: null,
  withdrawals: null,
  sheet: null,       // { type: 'create' } 首次建档弹层
};

const $screen = document.getElementById('drvScreen');
const $overlay = document.getElementById('drvOverlay');
const $nav = document.getElementById('drvNav');

// ---------------------------------------------------------------- 渲染

function renderAll() {
  if (S.tab === 'income') $screen.innerHTML = drvIncomeView();
  else if (S.tab === 'me') $screen.innerHTML = drvMeView();
  else $screen.innerHTML = drvHallView();

  $nav.innerHTML =
    `<div class="${S.tab === 'hall' ? 'on' : ''}" data-tab="hall">接单大厅</div>` +
    `<div class="${S.tab === 'income' ? 'on' : ''}" data-tab="income">收入</div>` +
    `<div class="${S.tab === 'me' ? 'on' : ''}" data-tab="me">我的</div>`;
  renderOverlay();
}

function renderOverlay() {
  if (!S.sheet) { $overlay.style.display = 'none'; $overlay.innerHTML = ''; return; }
  $overlay.style.display = 'flex';
  let body = '';
  if (S.sheet.type === 'create') body = createSheet();
  $overlay.innerHTML = `<div class="sheet">${body}</div>`;
}

function statusText(d) {
  if (!d) return '未建档';
  if (d.status === 'offline') return '已收车';
  if (d.status === 'idle') return '空闲·出车中';
  if (d.status === 'busy') return '忙碌·任务中';
  return d.status;
}

function drvTopBar(title, subtitle) {
  return `<div class="drvtop"><b style="font-size:15px;">${esc(title)}</b>` +
    `<div style="font-size:10px;opacity:.85;">${subtitle}</div></div>`;
}

function drvHallView() {
  const d = S.driver;
  if (!d) {
    return '<div class="card" style="margin-top:14px;"><h4>司机建档</h4>' +
      '<div class="note" style="margin-bottom:8px;">首次使用先创建司机档案（本地联调，无需真实资质）。</div>' +
      '<div style="display:flex;gap:6px;margin-bottom:8px;">' +
      '<input id="drvName" placeholder="姓名" value="王师傅" style="flex:1">' +
      '<input id="drvPhone" type="tel" placeholder="手机号" value="13800000099" style="flex:1"></div>' +
      '<button class="btn blue" style="width:100%;" onclick="drvCreate()">创建司机</button></div>' +
      '<div class="card" style="text-align:center;color:#909399;padding:26px 0;">建档后即可出车接单</div>';
  }

  let h = '<div class="drvtop"><div class="row1"><div><b style="font-size:15px;">' + esc(d.name) + '</b>' +
    `<div style="font-size:10px;opacity:.85;">${esc(d.driver_no || '')} · 沪AD·88888 · ★${d.rating == null ? '-' : d.rating} · ${statusText(d)}</div></div>` +
    '<div style="display:flex;gap:6px;align-items:center;">' +
    `<button class="dutybtn" style="${d.status === 'offline' ? 'background:#67c23a;color:#fff;' : 'background:#fff;color:#303133;'}" onclick="drvToggleDuty()">` +
    `${d.status === 'offline' ? '出车' : '收车'}</button></div></div>` +
    '<div style="display:flex;justify-content:space-between;margin-top:10px;align-items:center;">' +
    '<div class="modebox">' +
    `<button class="${d.mode === 'listen' ? 'on' : ''}" onclick="drvSetMode('listen')">听单</button>` +
    `<button class="${d.mode === 'grab' ? 'on' : ''}" onclick="drvSetMode('grab')">⚡ 抢单</button></div>` +
    `<span style="font-size:10px;opacity:.8;">今日完单 ${S.todayDone} · 收入 ¥${fy(S.todayIncome)}</span></div></div>`;

  // 抢单预占：grab 成功后需在窗口内确认接单（version 乐观锁）。
  if (S.invite) {
    h += '<div class="invitebar" style="background:#ecf5ff;border-color:#b3d8ff;">' +
      `<b>⚡ 抢单预占中</b><div style="font-size:11px;color:#409eff;margin:4px 0;">任务与司机双向锁定，请尽快确认接单</div>` +
      '<div class="btnrow" style="margin-top:6px;">' +
      '<button class="btn blue" onclick="drvAccept()">确认接单</button>' +
      '<button class="btn ghost" onclick="drvAbandon()">放弃</button></div></div>';
  }

  // 忙碌 → 当前任务卡。
  if (d.status === 'busy' && S.current) {
    h += drvTripCard(S.current);
  } else {
    // 已被自己预占的单不再出现在可抢列表里（invite 未确认前避免重复抢单入口）。
    const pool = S.orders.filter((o) => o.status === 'WAITING' && o.driver_id !== S.driverId &&
      (!S.invite || o.id !== S.invite.id));
    if (pool.length && d.status === 'idle') {
      for (const o of pool.slice(0, 4)) {
        h += poolCard(o);
      }
    } else if (S.current && ['ACCEPTED', 'IN_TRIP'].includes(S.current.status)) {
      h += drvTripCard(S.current);
    } else {
      h += `<div class="card" style="text-align:center;color:#909399;padding:26px 0;">` +
        `${d.status === 'offline' ? '已收车，点击上方"出车"开始接单' : '暂无新订单，等待派单推送…'}</div>`;
    }
    if (d.status === 'idle') {
      h += '<div class="card"><div class="note">联调辅助：乘客端下单需要实名账号，这里可以直接生成演示订单。</div>' +
        '<div class="btnrow"><button class="btn ghost" onclick="drvSeedOrder()">＋ 生成一单（模拟乘客下单）</button></div></div>';
    }
  }
  return h;
}

function poolCard(o) {
  return `<div class="poolcard"><div class="route"><span class="dots">` +
    '<span class="dot" style="width:8px;height:8px;border-radius:50%;background:#67c23a;display:block;"></span>' +
    '<span style="width:2px;height:10px;background:#dcdfe6;margin-left:3px;"></span>' +
    '<span class="dot" style="width:8px;height:8px;border-radius:50%;background:#f56c6c;display:block;"></span></span>' +
    `<span><b>${esc((o.origin && o.origin.name) || '起点')}</b><br><b>${esc((o.destination && o.destination.name) || '终点')}</b></span></div>` +
    `<div class="meta"><span>预估收入 ¥${fy(o.driver_income_fen)}</span>` +
    `<span>${esc(o.passenger_name || '乘客')} · ${o.passenger_count || 1}人</span>` +
    `<span class="chip st-${esc(o.status)}">${esc(stateName(o.status))}</span></div>` +
    `<button class="btn blue" style="width:100%;" onclick="drvGrab('${esc(o.id)}')">⚡ 立即抢单</button></div>`;
}

function drvTripCard(order) {
  let h = `<div class="card" style="margin-top:12px;"><h4>当前任务</h4>`;
  h += `<div class="suborder"><div style="display:flex;justify-content:space-between;">` +
    `<b style="font-size:12px;">${esc((order.origin && order.origin.name) || '')} → ${esc((order.destination && order.destination.name) || '')}</b>` +
    `<span class="chip st-${esc(order.status)}">${esc(stateName(order.status))}</span></div>` +
    `<div style="font-size:11px;color:#606266;margin-top:3px;">乘客 ${esc(order.passenger_name || '-')} · 司机收入 ¥${fy(order.driver_income_fen)}</div>` +
    (order.status === 'ACCEPTED'
      ? '<div class="btnrow" style="margin-top:6px;"><button class="btn blue" onclick="drvStart()">开始行程</button></div>' +
        '<div class="btnrow"><button class="btn ghost" onclick="drvRelease()">↩ 退单重新派单</button></div>'
      : '<div class="btnrow" style="margin-top:6px;"><button class="btn green" onclick="drvComplete()">确认送达，结束行程</button></div>');
  h += '</div></div>';
  return h;
}

function drvIncomeView() {
  const d = S.driver;
  if (!d) return '<div class="card" style="text-align:center;color:#909399;padding:30px 0;">请先在大厅完成司机建档</div>';
  const funds = d.funds || {};
  let h = drvTopBar('我的收入', `完单按结束日 · 收入按记账日 · ${esc(d.driver_no || '')}`);
  // 收入对比卡：今日 vs 昨日完单量/收入柱状图与净差。
  const cmp = S.compare;
  const tInc = (cmp && cmp.today && cmp.today.income_fen) || 0;
  const yInc = (cmp && cmp.yesterday && cmp.yesterday.income_fen) || 0;
  const tCount = (cmp && cmp.today && cmp.today.order_count) != null ? cmp.today.order_count : (S.todayDone || 0);
  const maxBar = Math.max(tInc, yInc, 1);
  const barH = (v) => Math.max(4, Math.round((v / maxBar) * 52));
  const diff = tInc - yInc;
  const diffText = diff >= 0 ? `今日比昨天多挣 ¥${fy(diff)}` : `今天比昨天少挣 ¥${fy(-diff)}`;
  h += `<div class="card">` +
    `<div class="incgrid">` +
    `<div class="cell"><b>${tCount}</b><span>今日完单量</span></div>` +
    `<div class="cell"><b>¥${fy(tInc)}</b><span>今日收入</span></div></div>` +
    `<div style="display:flex;align-items:flex-end;justify-content:center;gap:44px;height:88px;margin:12px 0 2px;">` +
    `<div style="text-align:center;"><div style="width:36px;height:${barH(tInc)}px;background:#2f6fed;border-radius:4px 4px 0 0;margin:0 auto;"></div>` +
    `<div style="font-size:11px;margin-top:4px;">今日</div><div style="font-size:11px;color:#606266;">¥${fy(tInc)}</div></div>` +
    `<div style="text-align:center;"><div style="width:36px;height:${barH(yInc)}px;background:#a8c6fa;border-radius:4px 4px 0 0;margin:0 auto;"></div>` +
    `<div style="font-size:11px;margin-top:4px;">昨日</div><div style="font-size:11px;color:#606266;">¥${fy(yInc)}</div></div></div>` +
    `<div style="text-align:center;font-size:13px;font-weight:600;color:${diff >= 0 ? '#67c23a' : '#f56c6c'};">${diffText}` +
    (cmp && cmp.percent_text ? `<small style="color:#909399;font-weight:400;">（${esc(cmp.percent_text)}）</small>` : '') + `</div>` +
    `<div class="note">今日截至当前，昨日为全天；完单按行程结束日、收入按记账日统计。</div></div>`;

  h += '<div class="card"><h4>余额（资金桶）</h4><div class="bucket">' +
    `<div><b>¥${fy(funds.available_fen)}</b><span>可用A</span></div>` +
    `<div><b>¥${fy(funds.pending_fen)}</b><span>待结算U</span></div>` +
    `<div><b>¥${fy(funds.withdraw_fen)}</b><span>提现中W</span></div></div>` +
    '<div style="display:flex;gap:6px;margin-top:10px;">' +
    '<input id="wdAmount" type="number" placeholder="提现金额(元)" min="10" style="flex:1">' +
    '<button class="btn blue" style="flex:none;padding:8px 16px;" onclick="drvWithdraw()">申请提现</button></div>' +
    `<div class="note">提现规则：单笔 ¥${fy(S.rule.min_withdraw_fen)} ~ ¥${fy(S.rule.max_withdraw_fen)}，单日 ${S.rule.daily_max_count} 笔 / ¥${fy(S.rule.daily_limit_fen)}；单笔 ≥ ¥${fy(S.rule.dual_approval_fen)} 需双人审批。</div></div>`;

  h += '<div class="card"><h4>收入流水</h4>';
  if (!S.flows || !S.flows.length) h += '<div class="note">暂无流水，完单后自动记账</div>';
  for (const f of (S.flows || []).slice(0, 12)) {
    // 流水金额嵌在 amount 对象里（driver_income_fen），旧字段做兜底。
    const raw = f.amount && typeof f.amount === 'object'
      ? f.amount.driver_income_fen
      : (f.amount != null ? f.amount : f.driver_income_fen);
    h += `<div style="display:flex;justify-content:space-between;padding:5px 0;border-bottom:1px dashed #f0f0f0;font-size:12px;">` +
      `<span>${esc(f.flow_type || f.type || '流水')} · 记入${esc(f.bucket_to || '-')}桶` +
      `<br><small style="color:#909399;">${esc(f.accounting_date || f.created_at || '')}</small></span>` +
      `<b style="color:#67c23a;">+¥${fy(raw)}</b></div>`;
  }
  h += '</div>';

  if (S.withdrawals && S.withdrawals.length) {
    h += '<div class="card"><h4>提现记录</h4>';
    for (const w of S.withdrawals.slice(0, 8)) {
      // 提现状态中文化：司机看得懂单子卡在哪一步。
      const wStatus = { reviewing: '审核中', approved: '待打款', paying: '打款中', success: '已到账', rejected: '已驳回' }[w.status || ''] || (w.status || '-');
      h += `<div style="display:flex;justify-content:space-between;padding:5px 0;border-bottom:1px dashed #f0f0f0;font-size:12px;">` +
        `<span>${esc(w.created_at || '')}</span><b>¥${fy(w.amount_fen)} · ${esc(wStatus)}</b></div>`;
    }
    h += '</div>';
  }
  return h;
}

function drvMeView() {
  const d = S.driver;
  if (!d) return '<div class="card" style="text-align:center;color:#909399;padding:30px 0;">请先在大厅完成司机建档</div>';
  let h = '<div class="drvtop"><div class="drvcard">' +
    `<div class="avatar" style="background:#fff;">${esc((d.name || '司').slice(0, 1))}</div>` +
    `<div><b style="font-size:15px;">${esc(d.name)}</b>` +
    `<div style="font-size:10px;opacity:.85;">★${d.rating == null ? '-' : d.rating} · 完成率 ${(Number(d.success_rate || 0) * 100).toFixed(0)}% · 已实名</div></div></div></div>`;
  h += '<div class="card"><h4>司机信息</h4><div class="feelist">' +
    `<div><span>司机编号</span><span>${esc(d.driver_no || '-')}</span></div>` +
    `<div><span>车型 / 核载</span><span>${esc(d.car_type || '-')} · ${d.seats || 4}人</span></div>` +
    `<div><span>服务城市</span><span>${esc(d.city_code || '-')}</span></div>` +
    `<div><span>当前状态</span><span>${esc(statusText(d))} · ${d.mode === 'listen' ? '听单' : '抢单'}</span></div></div></div>`;
  h += '<div class="card"><h4>服务规范</h4><div style="font-size:11px;color:#606266;line-height:1.8;">' +
    '· 接人话术：您好，请问尾号是XXXX吗？<br>· 禁止线下交易、绕路、拒载；红线行为将依证据处置。<br>' +
    '· 行驶中以安全为主，完整规范可停车后查看。</div></div>';
  h += '<div class="card"><h4>安全中心</h4><div style="font-size:11px;color:#606266;line-height:1.7;">' +
    '紧急求助将生成安全事件编号并上报后台安全队列（演示环境不实际拨号）。' +
    '<div class="btnrow"><button class="btn red" onclick="drvSos()">紧急求助（演示）</button></div></div></div>';
  return h;
}

function createSheet() {
  let h = '<h3>司机建档 <button class="x" onclick="drvCloseSheet()">✕</button></h3>';
  h += '<div class="note" style="margin-bottom:8px;">创建司机档案后即可出车接单。</div>';
  h += '<div style="display:flex;gap:6px;margin-bottom:8px;">' +
    '<input id="drvName" placeholder="姓名" value="王师傅" style="flex:1">' +
    '<input id="drvPhone" type="tel" placeholder="手机号" value="13800000099" style="flex:1"></div>' +
    '<button class="btn blue" style="width:100%;" onclick="drvCreate()">创建司机</button>';
  if (S.sheet.error) h += `<div class="note" style="color:#f56c6c;">${esc(S.sheet.error)}</div>`;
  return h;
}

// ---------------------------------------------------------------- 动作

function closeSheet() { S.sheet = null; renderAll(); }

async function loadProfile() {
  if (!S.driverId) return null;
  const driver = await api(`/driver/profile/${S.driverId}`);
  S.driver = driver;
  if (S.current && ['ENDED', 'CANCELLED'].includes(S.current.status)) S.current = null;
  return driver;
}

async function createDriver() {
  const name = (document.getElementById('drvName') || {}).value || '王师傅';
  const phone = (document.getElementById('drvPhone') || {}).value || '13800000099';
  try {
    const driver = await api('/driver/profile', {
      method: 'POST', body: { name: name.trim(), phone: phone.trim() },
    });
    S.driverId = driver.id;
    localStorage.setItem(STORE_KEY, driver.id);
    S.driver = driver;
    S.sheet = null;
    toast(`司机已创建：${driver.driver_no || driver.id}`);
    renderAll();
  } catch (error) {
    S.sheet = { type: 'create', error: error.message };
    renderAll();
  }
}

async function toggleDuty() {
  if (!S.driver) return;
  const target = S.driver.status === 'offline' ? 'idle' : 'offline';
  try {
    await api(`/driver/profile/${S.driverId}/status`, { method: 'POST', body: { status: target } });
    if (target === 'idle') {
      // 出车时立刻上报一次心跳，满足 60s 心跳窗口判定。
      await api(`/driver/profile/${S.driverId}/heartbeat`, { method: 'POST', body: { lat: 31.2054, lng: 121.601 } });
    }
    await loadProfile();
    toast(target === 'idle' ? '已出车，开始接单' : '已收车');
  } catch (error) {
    toast(`操作失败：${error.message}`);
  }
  renderAll();
  schedulePoll();
}

async function setMode(mode) {
  if (!S.driver) return;
  try {
    await api(`/driver/profile/${S.driverId}/mode`, { method: 'POST', body: { mode } });
    await loadProfile();
    toast(mode === 'listen' ? '已切换为听单模式' : '已切换为抢单模式');
  } catch (error) {
    toast(`操作失败：${error.message}`);
  }
  renderAll();
}

async function loadHall() {
  if (!S.driverId) return;
  const data = await api(`/driver/hall/orders?driver_id=${encodeURIComponent(S.driverId)}&lat=31.2054&lng=121.601&limit=20`);
  S.orders = data.items || [];
  const mine = S.orders.find((o) => o.driver_id === S.driverId && ['ACCEPTED', 'IN_TRIP'].includes(o.status));
  if (mine) {
    S.current = mine;
    S.invite = null;
  }
  renderAll();
}

async function seedOrder() {
  try {
    await api('/driver/hall/orders', {
      method: 'POST',
      body: {
        passenger_name: '演示乘客',
        origin: { name: '世纪大道地铁站', lat: 31.2336, lng: 121.5399 },
        destination: { name: '虹桥火车站', lat: 31.1979, lng: 121.3208 },
        passenger_paid_fen: 5000,
        platform_coupon_fen: 0,
        subsidy_fen: 0,
        passenger_count: 1,
        city_code: '310000',
      },
    });
    toast('已生成一单');
    await loadHall();
  } catch (error) {
    toast(`生成失败：${error.message}`);
  }
}

async function grab(orderId) {
  if (!S.driver || S.driver.status !== 'idle') { toast('请先出车（状态需为空闲）'); return; }
  try {
    const order = await api('/driver/hall/orders/grab', {
      method: 'POST',
      body: { driver_id: S.driverId, order_id: orderId, idempotency_key: randomKey('grab') },
    });
    S.invite = order;
    toast('抢单成功，请确认接单');
  } catch (error) {
    toast(`抢单失败：${error.message}`);
  }
  renderAll();
}

async function acceptInvite() {
  if (!S.invite) return;
  try {
    const order = await api('/driver/hall/orders/accept', {
      method: 'POST',
      body: { driver_id: S.driverId, order_id: S.invite.id, version: S.invite.version },
    });
    S.current = order;
    S.invite = null;
    toast('接单成功');
    await loadProfile();
  } catch (error) {
    S.invite = null;
    toast(`确认接单失败：${error.message}`);
  }
  renderAll();
}

function abandonInvite() { S.invite = null; toast('已放弃该单'); renderAll(); }

async function startTrip() {
  if (!S.current) return;
  try {
    const order = await api('/driver/hall/orders/start', {
      method: 'POST',
      body: { driver_id: S.driverId, order_id: S.current.id },
    });
    S.current = { ...S.current, ...order, status: 'IN_TRIP' };
    toast('行程已开始');
  } catch (error) {
    toast(`开始行程失败：${error.message}`);
  }
  renderAll();
}

async function completeTrip() {
  if (!S.current) return;
  try {
    await api('/driver/hall/orders/complete', {
      method: 'POST',
      body: { driver_id: S.driverId, order_id: S.current.id, distance_meters: 5200, duration_sec: 900 },
    });
    S.current = null;
    // 送达只是行程结束：收入要等乘客支付回调后才记账，避免"未支付先计收入"。
    toast('行程已结束，待乘客支付后计入收入');
    await loadProfile();
    await loadIncome();
  } catch (error) {
    toast(`结束行程失败：${error.message}`);
  }
  renderAll();
}

async function releaseOrder() {
  if (!S.current && !S.invite) return;
  const orderId = S.current ? S.current.id : S.invite.id;
  try {
    await api('/driver/hall/orders/release', {
      method: 'POST',
      body: { driver_id: S.driverId, order_id: orderId, reason: '司机主动退单' },
    });
    S.current = null;
    S.invite = null;
    toast('已退单，订单重新派发');
    await loadProfile();
    await loadHall();
  } catch (error) {
    toast(`退单失败：${error.message}`);
  }
  renderAll();
}

const S_RULE_DEFAULT = { min_withdraw_fen: 1000, max_withdraw_fen: 200000, daily_limit_fen: 500000, daily_max_count: 3, dual_approval_fen: 50000 };
S.rule = { ...S_RULE_DEFAULT };

async function loadIncome() {
  if (!S.driverId) return;
  try {
    const rule = await api(`/driver/income/summary?driver_id=${encodeURIComponent(S.driverId)}`);
    S.rule = { ...S_RULE_DEFAULT, ...rule };
    // 今日/昨日收益对比（完单量按结束日、收入按记账日，D11 口径）。
    try { S.compare = await api(`/driver/income/compare?driver_id=${encodeURIComponent(S.driverId)}`); }
    catch { S.compare = null; }
    const flows = await api(`/driver/income/flows?driver_id=${encodeURIComponent(S.driverId)}&page=1&page_size=100`);
    S.flows = flows.items || flows.list || [];
    // 今日统计从收入流水聚合：刷新或换设备后仍以账本为准，不再依赖会话内存。
    const today = new Date().toISOString().slice(0, 10);
    let done = 0;
    let income = 0;
    for (const f of S.flows) {
      const day = String(f.accounting_date || f.created_at || '').slice(0, 10);
      if (day !== today || (f.flow_type || f.type) !== 'income') continue;
      done += 1;
      income += (f.amount && typeof f.amount === 'object' ? f.amount.driver_income_fen : f.driver_income_fen) || 0;
    }
    S.todayDone = done;
    S.todayIncome = income;
  } catch (error) {
    toast(`拉取收入失败：${error.message}`);
  }
  renderAll();
}

async function loadWithdrawals() {
  if (!S.driverId) return;
  try {
    const data = await api(`/driver/income/withdrawals?driver_id=${encodeURIComponent(S.driverId)}`);
    S.withdrawals = data.items || data.list || [];
  } catch (error) {
    S.withdrawals = [];
  }
  renderAll();
}

function withdraw() {
  const input = document.getElementById('wdAmount');
  const amount = input ? Number(input.value) : 0;
  if (!amount || amount <= 0) { toast('请输入提现金额（元）'); return; }
  submitWithdraw(Math.round(amount * 100));
}

async function submitWithdraw(amountFen) {
  try {
    const data = await api('/driver/income/withdraw', {
      method: 'POST',
      body: { driver_id: S.driverId, amount_fen: amountFen, idempotency_key: randomKey('wd') },
    });
    // 申请只是预占（可用A→提现中W）：出账必须等管理端审核通过，不能"一点就到账"。
    toast(`提现申请已提交，等待平台审核${data.dual_approval ? '（大额单需双人审批）' : ''}`);
    await loadProfile();
    await loadIncome();
    loadWithdrawals();
  } catch (error) {
    toast(`提现失败：${error.message}`);
  }
}

function sos() { toast('安全事件已上报后台安全队列（联调演示，未实际拨号）'); }

// ---------------------------------------------------------------- 轮询与启动

let pollTimer = null;
function schedulePoll() {
  if (pollTimer) clearInterval(pollTimer);
  pollTimer = setInterval(async () => {
    if (!S.driverId || S.tab !== 'hall' || S.sheet) return;
    try {
      // 出车状态下顺带续心跳：后端按 60 秒心跳窗口判定收车。
      if (S.driver && S.driver.status !== 'offline') {
        await api(`/driver/profile/${S.driverId}/heartbeat`, { method: 'POST', body: { lat: 31.2054, lng: 121.601 } });
      }
      await loadProfile();
      await loadHall();
    } catch { /* 网关暂不可达时下一轮重试 */ }
  }, 5000);
}

window.drvCreate = createDriver;
window.drvCloseSheet = closeSheet;
window.drvToggleDuty = toggleDuty;
window.drvSetMode = setMode;
window.drvSeedOrder = seedOrder;
window.drvGrab = grab;
window.drvAccept = acceptInvite;
window.drvAbandon = abandonInvite;
window.drvStart = startTrip;
window.drvComplete = completeTrip;
window.drvRelease = releaseOrder;
window.drvWithdraw = withdraw;
window.drvSos = sos;

$nav.addEventListener('click', (event) => {
  const tab = event.target.dataset && event.target.dataset.tab;
  if (!tab) return;
  S.tab = tab;
  if (tab === 'income') { loadIncome(); loadWithdrawals(); }
  renderAll();
});

// 手机状态栏时钟。
function tickClock() {
  const el = document.getElementById('drvClock');
  if (el) el.textContent = new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
}
tickClock();
setInterval(tickClock, 30000);

renderAll();
if (S.driverId) {
  loadProfile()
    .then(() => loadHall())
    .catch(() => toast('上次记录的司机档案加载失败，可重新建档'));
  schedulePoll();
}
