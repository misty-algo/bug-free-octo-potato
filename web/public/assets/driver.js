// 司机端页面：档案 → 出车/听单抢单 → 接单大厅 → 行程推进 → 收入与提现。
// 视觉与交互流对齐 Figma 原型（花小猪打车平台 · 三端联调交互原型），
// 数据全部来自真实网关接口；「生成一单」用于在联调时模拟乘客下单。
import { api, fy, esc, toast, randomKey, stateName, DRIVER_TOKEN_KEY } from './api.js';

const STORE_KEY = 'huazhu.driverId';
// 活动任务（进行中订单）持久化：刷新页面不会丢掉司机地图的乘客位置标记。
const DRV_CURRENT_KEY = 'huazhu.driverCurrent';

const S = {
  tab: 'hall',
  driverId: localStorage.getItem(STORE_KEY) || '',
  // 登录态：令牌与档案都在本地后，未登录时先展示登录页。
  token: localStorage.getItem(DRIVER_TOKEN_KEY) || '',
  loggedIn: Boolean(localStorage.getItem(DRIVER_TOKEN_KEY)),
  login: {
    phone: '13800000001',
    code: '',
    name: '',
    // needName 由登录接口提示"首次登录需填姓名"后置起，展示建档字段。
    needName: false,
    sent: false,
    error: '',
  },
  driver: null,
  orders: [],        // 接单大厅 items
  current: null,     // 我当前接下的订单（ACCEPTED / IN_TRIP）
  invite: null,      // 抢单成功待确认（grab 返回的订单，带 version）
  todayDone: 0,
  todayIncome: 0,
  flows: null,
  withdrawals: null,
  // 两个长列表各自独立的查询条件与分页状态（每页 10 条，total 来自接口）。
  flowQuery: { from: '', to: '', page: 1, pageSize: 10, total: 0 },
  wdQuery: { from: '', to: '', status: '', page: 1, pageSize: 10, total: 0 },
  withdrawTotalFen: 0,
  history: null,     // 接单记录 items（含乘客评价/投诉）
  notices: null,     // 反馈通知 items（乘客评价/投诉）
  sheet: null,       // { type: 'create' } 首次建档弹层
};

// 进行中任务落盘：刷新页面后司机大厅地图仍能标出乘客上车点。
// 放在 S 字面量之后，避免被字面量里同名的 current:null 覆盖。
try {
  const savedCurrent = JSON.parse(localStorage.getItem(DRV_CURRENT_KEY));
  if (savedCurrent && savedCurrent.id) S.current = savedCurrent;
} catch (e) { void e; }
function saveDrvCurrent() {
  try {
    localStorage.setItem(DRV_CURRENT_KEY, S.current ? JSON.stringify(S.current) : '');
  } catch (e) { void e; }
}
window.saveDrvCurrent = saveDrvCurrent;

const $screen = document.getElementById('drvScreen');
const $overlay = document.getElementById('drvOverlay');
const $nav = document.getElementById('drvNav');

// ---------------------------------------------------------------- 渲染

function renderAll() {
  // 未登录：只渲染登录页，隐藏底部导航，避免未授权请求刷屏。
  if (!S.loggedIn) {
    $screen.innerHTML = drvLoginView();
    $nav.innerHTML = '';
    $overlay.style.display = 'none';
    $overlay.innerHTML = '';
    return;
  }
  if (S.tab === 'income') $screen.innerHTML = drvIncomeView();
  else if (S.tab === 'me') $screen.innerHTML = drvMeView();
  else $screen.innerHTML = drvHallView();

  $nav.innerHTML =
    `<div class="${S.tab === 'hall' ? 'on' : ''}" data-tab="hall">接单大厅</div>` +
    `<div class="${S.tab === 'income' ? 'on' : ''}" data-tab="income">收入</div>` +
    `<div class="${S.tab === 'me' ? 'on' : ''}" data-tab="me">我的</div>`;
  renderOverlay();
  // 大厅页渲染后初始化真实地图（容器必须已在 DOM 中）。
  if (S.tab === 'hall') renderDrvMap();
}

// ---------------------------------------------------------------- 司机登录

// drvLoginView 司机登录页：手机号 + 验证码登录；号码未建档时自动建档，
// 所以这里同时提供姓名输入（首次登录时展示）。
function drvLoginView() {
  const login = S.login;
  return '<div class="drvtop" style="padding:22px 16px;">' +
    '<b style="font-size:17px;">司机登录</b>' +
    '<div style="font-size:11px;opacity:.85;margin-top:2px;">手机号验证码登录 · 未注册自动建档</div></div>' +
    '<div class="card" style="margin-top:14px;">' +
    '<div style="display:flex;gap:6px;margin-bottom:8px;">' +
    `<input id="lgPhone" type="tel" maxlength="11" placeholder="手机号" value="${esc(login.phone)}" style="flex:1">` +
    '<button class="btn blue" style="flex:none;padding:8px 14px;" onclick="drvSendCode()">' +
    (login.sent ? '重发验证码' : '获取验证码') + '</button></div>' +
    `<input id="lgCode" maxlength="6" placeholder="验证码" value="${esc(login.code)}" style="width:100%;box-sizing:border-box;margin-bottom:8px;">` +
    (login.needName
      ? `<input id="lgName" placeholder="首次登录请填写姓名（自动建档）" value="${esc(login.name)}" style="width:100%;box-sizing:border-box;margin-bottom:8px;">`
      : '') +
    '<button class="btn dark" style="width:100%;" onclick="drvLogin()">登录</button>' +
    (login.error ? `<div class="note" style="color:#f56c6c;margin-top:8px;">${esc(login.error)}</div>` : '') +
    '<div class="note" style="margin-top:8px;">本地联调环境：验证码会随下发响应直接返回并自动填入。</div>' +
    '</div>';
}

async function drvSendCode() {
  const phone = (document.getElementById('lgPhone') || {}).value || '';
  S.login.phone = phone;
  S.login.error = '';
  try {
    const data = await api('/driver/auth/sms-code', { method: 'POST', body: { phone } });
    // 生产环境由短信通道下发，这里直接回填便于联调。
    S.login.code = data.debug_code || '';
    S.login.sent = true;
    toast(data.debug_code ? `验证码已发送：${data.debug_code}` : '验证码已发送');
  } catch (error) {
    S.login.error = error.message;
  }
  renderAll();
}

async function drvLogin() {
  const login = S.login;
  login.phone = (document.getElementById('lgPhone') || {}).value || login.phone;
  login.code = (document.getElementById('lgCode') || {}).value || '';
  if (login.needName) login.name = (document.getElementById('lgName') || {}).value || '';
  login.error = '';
  try {
    const data = await api('/driver/auth/login', {
      method: 'POST',
      body: { phone: login.phone, code: login.code, name: login.name },
    });
    // 登录态：令牌落盘，换设备重新登录即可找回档案与全部流水。
    S.token = data.token || '';
    S.loggedIn = true;
    S.driverId = (data.driver && data.driver.id) || '';
    S.driver = data.driver || null;
    localStorage.setItem(DRIVER_TOKEN_KEY, S.token);
    localStorage.setItem(STORE_KEY, S.driverId);
    toast(data.is_new ? '建档成功，已登录' : '登录成功');
    // 先切出登录页进入大厅，再拉数据填充内容。
    renderAll();
    startAfterLogin();
  } catch (error) {
    // 首次登录缺少姓名：后端返回 400，这里补上姓名输入框再让用户输入。
    if (/name_required/.test(error.message)) login.needName = true;
    login.error = error.message;
    renderAll();
  }
}

function drvLogout() {
  localStorage.removeItem(DRIVER_TOKEN_KEY);
  localStorage.removeItem(STORE_KEY);
  localStorage.removeItem(DRV_CURRENT_KEY);
  S.token = '';
  S.loggedIn = false;
  S.driver = null;
  S.driverId = '';
  S.login = { phone: S.login.phone, code: '', name: '', needName: false, sent: false, error: '' };
  S.tab = 'hall';
  toast('已退出登录');
  renderAll();
}

function renderOverlay() {
  // 弹层机制保留：后续如需在司机端加确认框/表单，按 S.sheet.type 扩展即可。
  if (!S.sheet) { $overlay.style.display = 'none'; $overlay.innerHTML = ''; return; }
  $overlay.style.display = 'flex';
  $overlay.innerHTML = `<div class="sheet">${S.sheet.body || ''}</div>`;
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
  // 登录即建档，所以这里没有建档入口：档案加载不到只可能是会话失效或网络异常。
  if (!d) {
    return '<div class="card" style="margin-top:14px;text-align:center;color:#909399;padding:26px 0;">' +
      '档案加载中或登录已失效' +
      '<div class="btnrow" style="margin-top:10px;">' +
      '<button class="btn ghost" onclick="drvLogout()">退出并重新登录</button></div></div>';
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
    `<span style="font-size:10px;opacity:.8;">今日完单 ${S.todayDone} · 收入 ¥${fy(S.todayIncome)}</span></div></div>` +
    // 真实全国地图：显示当前任务的起终点位置，可拖动缩放。
    '<div class="hubmap" id="drvMap" style="min-height:150px;margin-top:10px;"></div>';

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
    `<div style="font-size:11px;color:#606266;margin-top:3px;">乘客 ${esc(order.passenger_name || '-')} · 司机收入 ¥${fy(order.driver_income_fen)}</div>`;
  // 乘客请求更改下车点：司机同意后新终点生效，拒绝则维持原终点。
  const dc = order.destination_change;
  if (dc && dc.state === 'pending') {
    h += '<div style="background:#fdf6ec;border:1px solid #f5dab1;border-radius:8px;padding:8px 10px;margin-top:8px;">' +
      '<b style="color:#e6a23c;font-size:12px;">乘客请求更改下车点</b>' +
      `<div class="note">新终点：${esc(dc.new_name || '-')}（原终点：${esc((order.destination && order.destination.name) || '-')}）</div>` +
      '<div class="btnrow" style="margin-top:6px;">' +
      `<button class="btn green" onclick="drvDestDecision('${esc(order.id)}', true)">同意更改</button>` +
      `<button class="btn ghost" onclick="drvDestDecision('${esc(order.id)}', false)">拒绝</button></div></div>`;
  } else if (dc && dc.state === 'rejected') {
    h += '<div class="note" style="color:#909399;">您已拒绝乘客的下车点更改，行程终点不变</div>';
  } else if (dc && dc.state === 'approved') {
    h += '<div class="note" style="color:#67c23a;">已同意更改下车点：' + esc(dc.new_name || '') + '</div>';
  }
  h += (order.status === 'ACCEPTED'
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

  h += '<div class="card"><h4>收入流水</h4>' +
    drvRangeFilter('fl', S.flowQuery, 'drvFlowSearch()') +
    (S.flows && S.flows.length ? renderFlowRows() : '<div class="note">该条件下暂无流水</div>') +
    drvPager('fl', S.flowQuery, 'drvFlowPage');
  h += '</div>';

  h += '<div class="card"><h4>提现记录' + (S.withdrawTotalFen ? `（累计已到账 <b style="color:#16a34a;">¥${fy(S.withdrawTotalFen)}</b>）` : '') + '</h4>' +
    drvRangeFilter('wd', S.wdQuery, 'drvWdSearch()') +
    '<div style="display:flex;gap:6px;margin-top:6px;"><select id="wdStatus" onchange="drvWdSearch()" style="flex:1;font-size:12px;">' +
    WD_STATUS_OPTIONS.map((o) => `<option value="${o.value}"${S.wdQuery.status === o.value ? ' selected' : ''}>${o.label}</option>`).join('') +
    '</select></div>' +
    (S.withdrawals && S.withdrawals.length ? renderWithdrawRows() : '<div class="note">该条件下暂无提现记录</div>') +
    drvPager('wd', S.wdQuery, 'drvWdPage');
  h += '</div>';
  return h;
}

// ── 列表筛选与分页（收入流水 / 提现记录共用）──

const WD_STATUS_OPTIONS = [
  { value: '', label: '全部状态' },
  { value: 'reviewing', label: '审核中' },
  { value: 'approved', label: '待打款' },
  { value: 'paying', label: '打款中' },
  { value: 'success', label: '已到账' },
  { value: 'rejected', label: '已驳回' },
];

// drvRangeFilter 起止日期 + 查询：按记账日/申请日过滤，避免列表无限变长。
function drvRangeFilter(prefix, query, searchFn) {
  return '<div style="display:flex;gap:6px;align-items:center;margin-bottom:8px;">' +
    `<input id="${prefix}From" type="date" value="${esc(query.from || '')}" onchange="${searchFn}" style="flex:1;font-size:12px;">` +
    `<span style="color:#909399;">~</span>` +
    `<input id="${prefix}To" type="date" value="${esc(query.to || '')}" onchange="${searchFn}" style="flex:1;font-size:12px;">` +
    `<button class="btn blue" style="flex:none;padding:6px 12px;font-size:12px;" onclick="${searchFn}">查询</button></div>`;
}

// drvPager 上一页/下一页：页码与总数由接口 total 换算。
function drvPager(prefix, query, pageFn) {
  const size = query.pageSize || 10;
  const pages = Math.max(1, Math.ceil((query.total || 0) / size));
  return `<div style="display:flex;justify-content:space-between;align-items:center;margin-top:8px;font-size:12px;color:#606266;">` +
    `<button class="btn" style="padding:4px 10px;font-size:12px;" ${query.page <= 1 ? 'disabled' : ''} onclick="${pageFn}(-1)">上一页</button>` +
    `<span>第 ${query.page}/${pages} 页 · 共 ${query.total || 0} 条</span>` +
    `<button class="btn" style="padding:4px 10px;font-size:12px;" ${query.page >= pages ? 'disabled' : ''} onclick="${pageFn}(1)">下一页</button></div>`;
}

function renderFlowRows() {
  let h = '';
  for (const f of S.flows || []) {
    // 流水金额嵌在 amount 对象里（driver_income_fen），旧字段做兜底。
    const raw = f.amount && typeof f.amount === 'object'
      ? f.amount.driver_income_fen
      : (f.amount != null ? f.amount : f.driver_income_fen);
    h += `<div style="display:flex;justify-content:space-between;padding:5px 0;border-bottom:1px dashed #f0f0f0;font-size:12px;">` +
      `<span>${esc(f.flow_type || f.type || '流水')} · 记入${esc(f.bucket_to || '-')}桶` +
      `<br><small style="color:#909399;">${esc(f.accounting_date || f.created_at || '')}</small></span>` +
      `<b style="color:#67c23a;">+¥${fy(raw)}</b></div>`;
  }
  return h;
}

function renderWithdrawRows() {
  let h = '';
  for (const w of S.withdrawals || []) {
    // 提现状态中文化 + 着色：审核中橙、已到账绿、驳回灰，管理端审核结果一目了然。
    const wStatus = { reviewing: '审核中', approved: '待打款', paying: '打款中', success: '已到账', rejected: '已驳回' }[w.status || ''] || (w.status || '-');
    const wColor = { reviewing: '#d97706', approved: '#2563eb', paying: '#2563eb', success: '#16a34a', rejected: '#9ca3af' }[w.status || ''] || '#374151';
    h += `<div style="padding:5px 0;border-bottom:1px dashed #f0f0f0;font-size:12px;">` +
      `<div style="display:flex;justify-content:space-between;">` +
      `<span>${esc(w.created_at || '')}</span><b style="color:${wColor};">¥${fy(w.amount_fen)} · ${esc(wStatus)}</b></div>`;
    // 驳回理由必须让司机看到：管理端驳回时必填，这里原样展示。
    if (w.status === 'rejected' && w.reason) {
      h += `<div style="color:#9ca3af;margin-top:2px;">驳回理由：${esc(w.reason)}</div>`;
    }
    h += '</div>';
  }
  return h;
}

// drvStars 星级文本：实心星表示得分，空心星补齐 5 颗。
function drvStars(n) {
  const v = Math.max(0, Math.min(5, Number(n) || 0));
  return '★★★★★'.slice(0, v) + '☆☆☆☆☆'.slice(0, 5 - v);
}

// fmtTime 把 RFC3339 时间截成 YYYY-MM-DD HH:mm。
function fmtTime(s) {
  if (!s) return '';
  return String(s).replace('T', ' ').slice(0, 16);
}

// renderNoticesCard 反馈通知：乘客评价/投诉后司机可见。
function renderNoticesCard() {
  const list = S.notices || [];
  let h = '<div class="card"><h4>反馈通知</h4>';
  if (!list.length) {
    h += '<div class="note">暂无乘客评价或投诉通知</div>';
  } else {
    h += list.map((n) => {
      const isC = n.type === 'complaint';
      const tag = isC
        ? '<span class="chip" style="background:#fef0f0;color:#f56c6c;">投诉</span>'
        : '<span class="chip" style="background:#f0f9eb;color:#67c23a;">评价</span>';
      return '<div style="padding:8px 0;border-bottom:1px solid #f2f2f2;">' +
        '<div style="display:flex;justify-content:space-between;align-items:center;gap:8px;">' +
          '<span style="font-size:12px;">' + tag + ' <b>' + esc(n.title || '') + '</b></span>' +
          '<span style="font-size:10px;color:#909399;">' + esc(fmtTime(n.at)) + '</span>' +
        '</div>' +
        (n.content ? '<div style="font-size:11px;color:#606266;margin-top:3px;">' + esc(n.content) + '</div>' : '') +
      '</div>';
    }).join('');
  }
  h += '</div>';
  return h;
}

// renderHistoryCard 接单记录：展示每单乘客评价是否满意、是否有投诉内容。
function renderHistoryCard() {
  const list = S.history || [];
  let h = '<div class="card"><h4>接单记录</h4>';
  if (S.history == null) {
    h += '<div class="note">加载中…</div>';
  } else if (!list.length) {
    h += '<div class="note">暂无接单记录</div>';
  } else {
    h += list.map((o) => {
      const rating = o.rating;
      const complaint = o.complaint;
      let fb;
      if (rating) {
        fb = '<span style="color:#e6a23c;">' + drvStars(rating.score) + '</span> ' +
          (rating.content ? esc(rating.content) : '<span style="color:#909399;">仅打分</span>');
      } else {
        fb = '<span style="color:#909399;">乘客未评价</span>';
      }
      if (complaint) {
        fb += '<div style="color:#f56c6c;margin-top:3px;">投诉（' + esc(complaint.type || '') + '）：' + esc(complaint.content || '') + '</div>';
      }
      return '<div style="padding:9px 0;border-bottom:1px solid #f2f2f2;">' +
        '<div style="display:flex;justify-content:space-between;align-items:center;gap:8px;">' +
          '<b style="font-size:12px;">' + esc((o.origin && o.origin.name) || '') + ' → ' + esc((o.destination && o.destination.name) || '') + '</b>' +
          '<span class="chip st-' + esc(o.status) + '">' + esc(stateName(o.status)) + '</span>' +
        '</div>' +
        '<div style="font-size:11px;color:#606266;margin-top:3px;">乘客 ' + esc(o.passenger_name || '-') +
          ' · 收入 ¥' + fy(o.driver_income_fen) + ' · ' + esc(fmtTime(o.created_at)) + '</div>' +
        '<div style="font-size:11px;margin-top:4px;">' + fb + '</div>' +
      '</div>';
    }).join('');
  }
  h += '</div>';
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
  // 乘客评价/投诉后的通知 + 司机接单记录（含每单评价是否满意、投诉内容）。
  h += renderNoticesCard();
  h += renderHistoryCard();
  h += '<div class="card"><h4>服务规范</h4><div style="font-size:11px;color:#606266;line-height:1.8;">' +
    '· 接人话术：您好，请问尾号是XXXX吗？<br>· 禁止线下交易、绕路、拒载；红线行为将依证据处置。<br>' +
    '· 行驶中以安全为主，完整规范可停车后查看。</div></div>';
  h += '<div class="card"><h4>安全中心</h4><div style="font-size:11px;color:#606266;line-height:1.7;">' +
    '紧急求助将生成安全事件编号并上报后台安全队列（演示环境不实际拨号）。' +
    '<div class="btnrow"><button class="btn red" onclick="drvSos()">紧急求助（演示）</button></div></div></div>';
  // 账号区：显示登录手机号并给出退出入口，换设备时从这里退出再用手机号登录。
  h += '<div class="card"><h4>账号</h4><div class="feelist">' +
    `<div><span>登录手机号</span><span>${esc(d.phone || '-')}</span></div>` +
    `<div><span>登录状态</span><span style="color:#67c23a;">已登录</span></div></div>` +
    '<div class="note">退出后可用同一手机号重新登录，档案、收入与提现记录都会保留。</div>' +
    '<div class="btnrow" style="margin-top:8px;"><button class="btn ghost" onclick="drvLogout()">退出登录</button></div></div>';
  return h;
}

// ---------------------------------------------------------------- 动作

async function loadProfile() {
  if (!S.driverId) return null;
  const driver = await api(`/driver/profile/${S.driverId}`);
  S.driver = driver;
  if (S.current && ['ENDED', 'CANCELLED'].includes(S.current.status)) S.current = null;
  saveDrvCurrent();
  return driver;
}

// loadHistory 拉取接单记录与反馈通知（乘客评价/投诉），供"我的"页展示。
async function loadHistory() {
  if (!S.driverId) return;
  try {
    const [hist, notices] = await Promise.all([
      api(`/driver/history?driver_id=${encodeURIComponent(S.driverId)}&limit=20`),
      api(`/driver/notices?driver_id=${encodeURIComponent(S.driverId)}&limit=10`),
    ]);
    S.history = (hist && hist.items) || [];
    S.notices = (notices && notices.items) || [];
  } catch (error) {
    if (S.history == null) S.history = [];
    if (S.notices == null) S.notices = [];
    toast(`拉取接单记录失败：${error.message}`);
  }
  if (S.tab === 'me') renderAll();
}

async function toggleDuty() {
  if (!S.driver) return;
  const target = S.driver.status === 'offline' ? 'idle' : 'offline';
  try {
    await api(`/driver/profile/${S.driverId}/status`, { method: 'POST', body: { status: target } });
    if (target === 'idle') {
      // 出车时立刻上报一次心跳，满足 60s 心跳窗口判定；坐标用真实定位。
      const pos = await drvPosition();
      await api(`/driver/profile/${S.driverId}/heartbeat`, { method: 'POST', body: { lat: pos.lat, lng: pos.lng } });
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
  // 排序坐标用司机真实位置，定位不可用时回退演示点。
  const pos = drvGeo || { lat: 31.2054, lng: 121.601 };
  const data = await api(`/driver/hall/orders?driver_id=${encodeURIComponent(S.driverId)}&lat=${pos.lat}&lng=${pos.lng}&limit=20`);
  S.orders = data.items || [];
  const mine = S.orders.find((o) => o.driver_id === S.driverId && ['ACCEPTED', 'IN_TRIP'].includes(o.status));
  if (mine) {
    S.current = mine;
    S.invite = null;
  }
  // 活动任务可能不在大厅列表里（已接单），保留启动时的本地恢复值。
  saveDrvCurrent();
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
    saveDrvCurrent();
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
    saveDrvCurrent();
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
    saveDrvCurrent();
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
    saveDrvCurrent();
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
    // 今日统计单独按记账日查：与流水列表的筛选/分页互不影响。
    const todayKey = bizDateKey(new Date());
    const todayFlows = await api(`/driver/income/flows?driver_id=${encodeURIComponent(S.driverId)}` +
      `&from=${todayKey}&to=${todayKey}&page=1&page_size=100`);
    let done = 0;
    let income = 0;
    for (const f of todayFlows.items || todayFlows.list || []) {
      if ((f.flow_type || f.type) !== 'income') continue;
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

// bizDateKey 取业务时区（Asia/Shanghai）的 yyyy-mm-dd，与后端记账日口径一致。
function bizDateKey(date) {
  const shifted = new Date(date.getTime() + 8 * 3600 * 1000);
  return shifted.toISOString().slice(0, 10);
}

// ── 收入流水：日期筛选 + 分页 ──

async function loadFlows() {
  if (!S.driverId) return;
  const q = S.flowQuery;
  try {
    const url = `/driver/income/flows?driver_id=${encodeURIComponent(S.driverId)}` +
      `&page=${q.page}&page_size=${q.pageSize}` +
      (q.from ? `&from=${encodeURIComponent(q.from)}` : '') +
      (q.to ? `&to=${encodeURIComponent(q.to)}` : '');
    const data = await api(url);
    S.flows = data.items || data.list || [];
    q.total = data.total || 0;
  } catch (error) {
    S.flows = [];
    q.total = 0;
  }
  renderAll();
}

function drvFlowSearch() {
  S.flowQuery.from = (document.getElementById('flFrom') || {}).value || '';
  S.flowQuery.to = (document.getElementById('flTo') || {}).value || '';
  S.flowQuery.page = 1;
  loadFlows();
}

function drvFlowPage(delta) {
  const next = S.flowQuery.page + delta;
  if (next < 1) return;
  S.flowQuery.page = next;
  loadFlows();
}

// ── 提现记录：日期 + 状态筛选 + 分页 ──

async function loadWithdrawals() {
  if (!S.driverId) return;
  const q = S.wdQuery;
  try {
    const url = `/driver/income/withdrawals?driver_id=${encodeURIComponent(S.driverId)}` +
      `&page=${q.page}&page_size=${q.pageSize}` +
      (q.from ? `&from=${encodeURIComponent(q.from)}` : '') +
      (q.to ? `&to=${encodeURIComponent(q.to)}` : '') +
      (q.status ? `&status=${encodeURIComponent(q.status)}` : '');
    const data = await api(url);
    S.withdrawals = data.items || data.list || [];
    q.total = data.total || 0;
  } catch (error) {
    S.withdrawals = [];
    q.total = 0;
  }
  // 提现状态变化（驳回退款 / 打款出账）会同时改变余额与流水，
  // 检测到变化就联动刷新，避免余额卡片停留在申请提现时的快照。
  const sig = (S.withdrawals || []).map((w) => w.status).join(',');
  if (S.wdStatusSig !== undefined && sig !== S.wdStatusSig) {
    loadProfile();
    loadFlows();
  }
  S.wdStatusSig = sig;
  // 累计提现独立于筛选条件：始终统计该司机全部已到账单据。
  loadWithdrawTotal();
  renderAll();
}

// loadWithdrawTotal 累计已到账金额单独取数，避免被列表筛选切走。
async function loadWithdrawTotal() {
  if (!S.driverId) return;
  try {
    const data = await api(`/driver/income/withdrawals?driver_id=${encodeURIComponent(S.driverId)}` +
      `&status=success&page=1&page_size=100`);
    S.withdrawTotalFen = (data.items || data.list || []).reduce((sum, w) => sum + (w.amount_fen || 0), 0);
  } catch (error) {
    // 累计值取不到时保留上一次结果，不影响列表。
  }
}

function drvWdSearch() {
  S.wdQuery.from = (document.getElementById('wdFrom') || {}).value || '';
  S.wdQuery.to = (document.getElementById('wdTo') || {}).value || '';
  S.wdQuery.status = (document.getElementById('wdStatus') || {}).value || '';
  S.wdQuery.page = 1;
  loadWithdrawals();
}

function drvWdPage(delta) {
  const next = S.wdQuery.page + delta;
  if (next < 1) return;
  S.wdQuery.page = next;
  loadWithdrawals();
}

// 提现审核结果轮询：存在"审核中/待打款/打款中"单据时每 8 秒拉一次，
// 管理端通过/驳回/打款后司机端无需手动刷新即可看到状态变化；全部到终态则自动停止。
setInterval(() => {
  if (S.tab !== 'income' || !S.driverId) return;
  const pending = (S.withdrawals || []).some((w) => ['reviewing', 'approved', 'paying'].includes(w.status));
  if (pending) loadWithdrawals();
}, 8000);

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
    // 回到第 1 页并清掉状态筛选，保证新提交的申请一定看得见。
    S.wdQuery.page = 1;
    S.wdQuery.status = '';
    await loadProfile();
    await loadIncome();
    loadWithdrawals();
  } catch (error) {
    toast(`提现失败：${error.message}`);
  }
}

function sos() { toast('安全事件已上报后台安全队列（联调演示，未实际拨号）'); }

// ---------------------------------------------------------------- 轮询与启动

// drvGeo 缓存的司机真实坐标（WGS84）；定位不可用时回退到当前任务上车点。
var drvGeo = null;

// drvPosition 取司机上报用的坐标：优先浏览器定位，其次当前任务上车点，最后演示点。
// 心跳坐标决定乘客端看到的司机车辆位置，所以不能写死。
function drvPosition() {
  return new Promise((resolve) => {
    const fallback = () => {
      const active = S.current || S.invite || S.orders.find((o) => o.status === 'WAITING');
      if (active && active.origin && active.origin.lat) {
        resolve({ lat: active.origin.lat, lng: active.origin.lng });
        return;
      }
      resolve({ lat: 31.2054, lng: 121.601 });
    };
    if (drvGeo) { resolve(drvGeo); return; }
    if (!navigator.geolocation) { fallback(); return; }
    navigator.geolocation.getCurrentPosition((pos) => {
      drvGeo = { lat: pos.coords.latitude, lng: pos.coords.longitude };
      resolve(drvGeo);
    }, fallback, { enableHighAccuracy: true, timeout: 6000, maximumAge: 30000 });
  });
}

let pollTimer = null;
function schedulePoll() {
  if (pollTimer) clearInterval(pollTimer);
  pollTimer = setInterval(async () => {
    if (!S.driverId || S.tab !== 'hall' || S.sheet) return;
    try {
      // 出车状态下顺带续心跳：后端按 60 秒心跳窗口判定收车。
      if (S.driver && S.driver.status !== 'offline') {
        const pos = await drvPosition();
        await api(`/driver/profile/${S.driverId}/heartbeat`, { method: 'POST', body: { lat: pos.lat, lng: pos.lng } });
      }
      await loadProfile();
      await loadHall();
    } catch { /* 网关暂不可达时下一轮重试 */ }
  }, 5000);
}

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
window.drvFlowSearch = drvFlowSearch;
window.drvFlowPage = drvFlowPage;
window.drvWdSearch = drvWdSearch;
window.drvWdPage = drvWdPage;
window.drvSendCode = drvSendCode;
window.drvLogin = drvLogin;
window.drvLogout = drvLogout;

$nav.addEventListener('click', (event) => {
  const tab = event.target.dataset && event.target.dataset.tab;
  if (!tab) return;
  S.tab = tab;
  // 收入页三个数据源：概览/对比、流水列表、提现列表，各自分页互不干扰。
  if (tab === 'income') { loadIncome(); loadFlows(); loadWithdrawals(); }
  // 我的页：拉取接单记录与反馈通知（乘客评价/投诉）。
  if (tab === 'me') { loadHistory(); }
  renderAll();
});

// 手机状态栏时钟。
function tickClock() {
  const el = document.getElementById('drvClock');
  if (el) el.textContent = new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
}
tickClock();
setInterval(tickClock, 30000);

// startAfterLogin 登录成功后的冷启动：拉取档案与大厅数据并开启轮询。
function startAfterLogin() {
  loadProfile()
    .then(() => loadHall())
    .catch(() => toast('档案加载失败，请检查网络或重新登录'));
  schedulePoll();
}

renderAll();
// 已有令牌视为已登录：直接恢复会话；否则停在登录页等待手机号验证码登录。
if (S.loggedIn && S.driverId) startAfterLogin();

// ============ 司机端真实地图 / 下车点变更裁决 ============

// dotDrvIcon div 圆点标记（起终点/定位）。
function dotDrvIcon(color) {
  return L.divIcon({
    className: '',
    html: '<span style="display:inline-block;width:14px;height:14px;border-radius:50%;' +
      `background:${color};border:3px solid #fff;box-shadow:0 1px 5px rgba(0,0,0,.4);"></span>`,
    iconSize: [14, 14], iconAnchor: [7, 7],
  });
}

// ============ 坐标系转换：WGS84 → GCJ02（火星坐标） ============
// 业务数据（POI、GPS、订单坐标）存 WGS84；高德底图是 GCJ02，渲染前转换。

function drvTransformLat(x, y) {
  let r = -100 + 2 * x + 3 * y + 0.2 * y * y + 0.1 * x * y + 0.2 * Math.sqrt(Math.abs(x));
  r += (20 * Math.sin(6 * x * Math.PI) + 20 * Math.sin(2 * x * Math.PI)) * 2 / 3;
  r += (20 * Math.sin(y * Math.PI) + 40 * Math.sin(y / 3 * Math.PI)) * 2 / 3;
  r += (160 * Math.sin(y / 12 * Math.PI) + 320 * Math.sin(y * Math.PI / 30)) * 2 / 3;
  return r;
}

function drvTransformLng(x, y) {
  let r = 300 + x + 2 * y + 0.1 * x * x + 0.1 * x * y + 0.1 * Math.sqrt(Math.abs(x));
  r += (20 * Math.sin(6 * x * Math.PI) + 20 * Math.sin(2 * x * Math.PI)) * 2 / 3;
  r += (20 * Math.sin(x * Math.PI) + 40 * Math.sin(x / 3 * Math.PI)) * 2 / 3;
  r += (150 * Math.sin(x / 12 * Math.PI) + 300 * Math.sin(x / 30 * Math.PI)) * 2 / 3;
  return r;
}

// drvGcj 把 WGS84 经纬度转成 GCJ02 数组 [lat, lng]；境外坐标原样返回。
function drvGcj(lat, lng) {
  const a = 6378245.0, ee = 0.00669342162296594323;
  if (lng < 72.004 || lng > 137.8347 || lat < 0.8293 || lat > 55.8271) return [lat, lng];
  let dLat = drvTransformLat(lng - 105, lat - 35);
  let dLng = drvTransformLng(lng - 105, lat - 35);
  const radLat = lat / 180 * Math.PI;
  let magic = Math.sin(radLat); magic = 1 - ee * magic * magic;
  const sqrtMagic = Math.sqrt(magic);
  dLat = (dLat * 180) / ((a * (1 - ee)) / (magic * sqrtMagic) * Math.PI);
  dLng = (dLng * 180) / (a / sqrtMagic * Math.cos(radLat) * Math.PI);
  return [lat + dLat, lng + dLng];
}

// renderDrvMap 初始化大厅页真实地图：高德底图，显示当前任务/候选单的起终点，可拖动看全国。
// 初始化后自动定位到司机当前位置（浏览器授权），并提供手动定位按钮。
function renderDrvMap() {
  const el = document.getElementById('drvMap');
  // 轮询每 5 秒重建页面 DOM，地图容器会被替换。销毁旧实例前记下当前视野，
  // 新实例按该视野还原，避免司机手动滑动查看别的地区时被拉回定位点。
  if (window.drvMapInstance) {
    try {
      window.drvMapLastView = {
        center: window.drvMapInstance.getCenter(),
        zoom: window.drvMapInstance.getZoom(),
      };
    } catch (e) { void e; }
    try { window.drvMapInstance.remove(); } catch (e) { void e; }
    window.drvMapInstance = null;
  }
  if (!el || typeof L === 'undefined') return;
  const restoredView = window.drvMapLastView;
  // 动画用 Leaflet 默认（缩放、瓦片淡入、标记缩放、拖动惯性），与乘客端一致。
  window.drvMapInstance = L.map(el, { zoomControl: false, attributionControl: false });
  // 高德栅格底图（国内可达；OSM 瓦片在境内网络超时）。
  L.tileLayer('https://webrd0{s}.is.autonavi.com/appmaptile?x={x}&y={y}&z={z}&lang=zh_cn&size=1&scale=1&style=8',
    { subdomains: '1234', minZoom: 3, maxZoom: 18 }).addTo(window.drvMapInstance);
  const active = S.current || S.invite || S.orders.find((o) => o.status === 'WAITING');
  // 订单坐标是 WGS84，上图前转 GCJ02。
  const pts = [];
  if (active && active.origin && active.origin.lat) pts.push(drvGcj(active.origin.lat, active.origin.lng));
  if (active && active.destination && active.destination.lat) pts.push(drvGcj(active.destination.lat, active.destination.lng));
  if (!pts.length) {
    // 无任务：首次给全国视野，随后自动定位飞到当前位置。
    if (!restoredView) window.drvMapInstance.setView([35.8617, 104.1954], 4);
  } else {
    // 上车点即乘客位置：用乘客头像标记，和目的地红点区分。
    pts.forEach((p, i) => L.marker(p, {
      icon: i === 0 ? L.divIcon({
        className: '',
        html: '<span style="display:inline-flex;width:24px;height:24px;border-radius:50%;background:#19be6b;' +
          'color:#fff;align-items:center;justify-content:center;font-size:13px;border:2px solid #fff;' +
          'box-shadow:0 1px 5px rgba(0,0,0,.4);">乘</span>',
        iconSize: [24, 24], iconAnchor: [12, 12],
      }) : dotDrvIcon('#f56c6c'),
    })
      .addTo(window.drvMapInstance)
      .bindTooltip(i === 0 ? '乘客位置（上车点）' : '目的地'));
    if (pts.length > 1) {
      L.polyline(pts, { color: '#2f6fed', weight: 3, dashArray: '6,8' }).addTo(window.drvMapInstance);
    }
    // 有保存视野时不再按订单坐标重设视野，保留司机正在查看的区域。
    if (!restoredView) {
      if (pts.length === 1) window.drvMapInstance.setView(pts[0], 14);
      else window.drvMapInstance.fitBounds(L.latLngBounds(pts).pad(0.35));
    }
  }
  // 定位按钮（右上角）。
  const LocateCtl = L.Control.extend({
    onAdd() {
      const btn = L.DomUtil.create('button', 'map-locate-btn');
      btn.innerHTML = '◎';
      btn.title = '定位到我的位置';
      btn.type = 'button';
      L.DomEvent.on(btn, 'click', (e) => { L.DomEvent.stop(e); locateDrvMap(true, true); });
      return btn;
    },
  });
  window.drvMapInstance.addControl(new LocateCtl({ position: 'topright' }));
  if (restoredView && restoredView.center) {
    window.drvMapInstance.setView(restoredView.center, restoredView.zoom, { animate: false });
  }
  // 只在首次进入大厅时自动定位并居中；之后仅刷新定位蓝点，不改变当前视野。
  locateDrvMap(false, !window.drvMapAutoCentered);
  window.drvMapAutoCentered = true;
  // 容器尺寸就绪后重算一次；尺寸没变时是空操作。校验实例未被重建。
  setTimeout(() => {
    const map = window.drvMapInstance;
    if (map && map.getContainer && map.getContainer().isConnected) {
      try { map.invalidateSize(); } catch (e) { void e; }
    }
  }, 80);
}

// drvLocateMarker 司机定位蓝点。用 var 声明避免模块提升问题。
var drvLocateMarker = null;

// locateDrvMap 司机端地理定位：成功后画蓝点；仅在手动定位或首次进入时居中。
function locateDrvMap(manual, recenter = false) {
  if (!window.drvMapInstance || !navigator.geolocation) {
    if (manual) toast('当前浏览器不支持定位');
    return;
  }
  navigator.geolocation.getCurrentPosition((pos) => {
    // 定位回调是异步的：取一次当前实例并确认容器仍在文档中，避免地图重建后
    // 把标记加进已销毁的实例（动画开启后图层操作落在死实例上会抛错）。
    const map = window.drvMapInstance;
    const mapEl = map && map.getContainer ? map.getContainer() : null;
    if (!map || !mapEl || !mapEl.isConnected) return;
    // GPS 是 WGS84，上图前转 GCJ02。
    const latlng = drvGcj(pos.coords.latitude, pos.coords.longitude);
    if (drvLocateMarker) drvLocateMarker.remove();
    drvLocateMarker = L.marker(latlng, {
      icon: L.divIcon({
        className: '',
        html: '<span style="position:relative;display:inline-block;width:22px;height:22px;">' +
          '<span style="position:absolute;inset:0;border-radius:50%;background:#2f6fed;opacity:.25;"></span>' +
          '<span style="position:absolute;inset:6px;border-radius:50%;background:#2f6fed;border:2.5px solid #fff;box-shadow:0 1px 6px rgba(0,0,0,.4);"></span></span>',
        iconSize: [22, 22], iconAnchor: [11, 11],
      }),
    }).addTo(map).bindTooltip('我的位置');
    // 只有手动点定位按钮或首次进入时才改变视野；周期性定位只更新蓝点，
    // 否则司机正在查看别的地区会被反复拉回定位点。
    if (recenter) {
      // 尺寸为 0 时 flyTo 会除零算出 NaN 坐标，退化为无动画定位。
      const size = map.getSize();
      if (size.x > 0 && size.y > 0) map.flyTo(latlng, 15, { duration: 0.6 });
      else map.setView(latlng, 15, { animate: false });
    }
    if (manual) toast('已定位到当前位置');
  }, (err) => {
    if (manual) {
      if (err.code === err.PERMISSION_DENIED) toast('定位被拒绝：请在浏览器地址栏允许位置权限');
      else toast('定位失败，请稍后重试');
    }
  }, { enableHighAccuracy: true, timeout: 8000, maximumAge: 30000 });
}

// drvDestDecision 司机裁决乘客的下车点变更：同意即按新终点继续行程。
async function drvDestDecision(orderId, approve) {
  if (!S.driverId) { toast('请先登录'); return; }
  try {
    await api('/driver/hall/orders/destination-decision', {
      method: 'POST',
      body: { driverId: S.driverId, orderId, approve },
    });
    toast(approve ? '已同意更改下车点' : '已拒绝更改');
    await loadHall();
  } catch (error) {
    toast(`操作失败：${error.message}`);
  }
}

window.drvDestDecision = drvDestDecision;