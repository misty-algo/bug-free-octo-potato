// 乘客端页面：登录 → 选起终点 → 估价 → 选车型 → 呼叫。
// 视觉与交互流对齐 Figma 原型（花小猪打车平台 · 三端联调交互原型），
// 数据全部来自真实网关接口；订单列表为会话内状态（网关暂无订单查询接口）。
import { api, fy, esc, toast, randomKey, stateName } from './api.js';

// 与原型一致的演示地点（含经纬度，供下单与估价使用）。
const POIS = [
  { id: 'p1', name: '世纪大道地铁站', addr: '浦东新区·世纪大道100号', lng: 121.5399, lat: 31.2336 },
  { id: 'p2', name: '张江高科站', addr: '浦东新区·科苑路', lng: 121.5869, lat: 31.2045 },
  { id: 'p3', name: '陆家嘴金融中心', addr: '浦东新区·世纪大道501号', lng: 121.5055, lat: 31.2397 },
  { id: 'p4', name: '人民广场', addr: '黄浦区·人民大道', lng: 121.4737, lat: 31.2304 },
  { id: 'p5', name: '迪士尼乐园', addr: '浦东新区·川沙新镇', lng: 121.6679, lat: 31.1433 },
  { id: 'p6', name: '虹桥火车站', addr: '闵行区·申虹路', lng: 121.3208, lat: 31.1979 },
];

// 后端车型未拉到之前的兜底展示；成功拉到 /passenger/home 后用真实数据替换。
const FALLBACK_CARS = [
  { car_type: 1, name: '快车', available: true, min_fee: 1300, desc: '经济实惠 · 4座' },
  { car_type: 3, name: '专车', available: true, min_fee: 2500, desc: '舒适型 · 优享服务' },
  { car_type: 2, name: '拼车', available: true, min_fee: 800, desc: '拼成85折 · 未拼成不涨价' },
];
const CAR_DESC = { 1: '经济实惠 · 4座', 2: '拼成85折 · 未拼成不涨价', 3: '舒适型 · 优享服务' };

// 客服接口不可用时的兜底展示（对齐契约 3.7.3 的 FAQ 题集；接 GetServiceConfig 后被真实数据覆盖）。
const FALLBACK_FAQ = [
  { q: '如何开具发票？', a: '行程结束后在「订单」页进入对应订单，点击「开具发票」提交抬头信息，电子发票通常 24 小时内发送到您的邮箱。' },
  { q: '费用与预估不一致怎么办？', a: '实际费用按里程与时长结算，遇堵车或改路线会有差异。若差异超过预估的 30%，可在订单详情页点击「费用申诉」，客服 24 小时内核实并多退少补。' },
  { q: '投诉多久有结果？', a: '提交投诉后客服会在 24 小时内首次跟进，一般 3 个工作日内给出处理结果，进度可在「我的工单」中查看。' },
  { q: '如何修改评价？', a: '行程结束后的评价在 24 小时内可修改一次：进入对应订单的评价卡片点击「修改评价」即可；超过 24 小时评价将存档不可更改。' },
  { q: '遗失物品怎么找回？', a: '请尽快拨打客服热线，提供订单号与遗失物品描述，客服会协助联系司机；物品通常保管 7 天，可到约定地点领取或选择寄回。' },
];

/** expireText 把 Unix 秒转成 YYYY-MM-DD 文本。 */
function expireText(seconds) {
  if (!seconds) return '-';
  return new Date(seconds * 1000).toLocaleDateString('zh-CN');
}

const S = {
  tab: 'home',
  accountId: '', token: '', phone: '',
  start: null, end: null,   // 选中的 POI 对象（来自内置列表或搜索结果）
  estimate: null,
  carType: 1,
  cars: FALLBACK_CARS,
  orders: [],
  sheet: null,
  codeSent: '',
  orderError: '',
  // 实名状态：state 为 UNVERIFIED/VERIFIED，mask 为服务端脱敏值。
  realname: null,
  // 我的券 / 领券中心 / 当前选中的券 / 客服页数据。
  coupons: [], claimable: [], couponId: '', service: null,
  // 本会话 + localStorage 记录的已领取券码，领券中心据此置灰。
  claimedNos: (() => {
    try { return new Set(JSON.parse(localStorage.getItem('huazhu.claimedNos') || '[]')); }
    catch { return new Set(); }
  })(),
  // 当前进行中订单（轮询 /orders/current 实时同步司机接单状态）。
  currentOrder: null, flowNotified: false,
  // 全屏页（service = 客服中心）与 FAQ 手风琴展开项。
  fullpage: null, faqOpen: null,
};

const $screen = document.getElementById('paxScreen');
const $overlay = document.getElementById('paxOverlay');
const $nav = document.getElementById('paxNav');

function carName(type) {
  const c = S.cars.find((x) => x.car_type === type);
  return c ? c.name : '快车';
}
function loggedIn() { return Boolean(S.accountId && S.token); }

// ---------------------------------------------------------------- 渲染

function renderAll() {
  // 全屏页（客服中心）覆盖主屏。
  if (S.fullpage === 'service') {
    $screen.innerHTML = serviceFullView();
    $nav.innerHTML = '';
    renderOverlay();
    return;
  }
  if (S.tab === 'orders') $screen.innerHTML = paxOrdersView();
  else if (S.tab === 'me') $screen.innerHTML = paxMeView();
  else if (activeOrder()) $screen.innerHTML = paxFlowView(activeOrder());
  else $screen.innerHTML = paxHomeView();

  $nav.innerHTML =
    `<div class="${S.tab === 'home' ? 'on' : ''}" data-tab="home">首页</div>` +
    `<div class="${S.tab === 'orders' ? 'on' : ''}" data-tab="orders">订单</div>` +
    `<div class="${S.tab === 'me' ? 'on' : ''}" data-tab="me">我的</div>`;
  renderOverlay();
}

function renderOverlay() {
  if (!S.sheet) { $overlay.style.display = 'none'; $overlay.innerHTML = ''; return; }
  $overlay.style.display = 'flex';
  let body = '';
  if (S.sheet.type === 'poi') body = poiSheet(S.sheet.kind);
  else if (S.sheet.type === 'login') body = loginSheet();
  else if (S.sheet.type === 'detail') body = orderDetailSheet(S.sheet.order);
  else if (S.sheet.type === 'realname') body = realNameSheet();
  else if (S.sheet.type === 'claim') body = claimCouponSheet();
  $overlay.innerHTML = `<div class="sheet">${body}</div>`;
}

/**
 * mapMock 渲染地图：城市底图（地块/水系/绿地/路网/地名）+ 定位蓝点 + 起终点标记。
 * 起终点未选时不画路线标记，只保留定位点与底图。
 */
function mapMock(start, end) {
  const pin = (x, y, color) =>
    `<circle cx="${x}" cy="${y}" r="5.5" fill="${color}" stroke="#fff" stroke-width="2"/>`;
  return '<div class="hubmap"><svg viewBox="0 0 300 170">' +
    '<rect width="300" height="170" fill="#f1f2f4"/>' +
    // 江水与水岸线。
    '<path d="M-6,130 C44,116 60,92 98,88 C138,84 158,110 198,114 C240,118 270,136 306,144 L306,172 L-6,172 Z" fill="#d3e7fa"/>' +
    '<path d="M-6,130 C44,116 60,92 98,88 C138,84 158,110 198,114 C240,118 270,136 306,144" stroke="#bcd9f4" stroke-width="1" fill="none"/>' +
    // 公园绿地。
    '<path d="M12,20 L88,12 L98,58 L22,66 Z" fill="#dcead0"/>' +
    '<path d="M224,104 L294,96 L300,150 L230,156 Z" fill="#dcead0"/>' +
    // 重点街区（浅紫地块）。
    '<path d="M110,24 L190,16 L200,64 L118,72 Z" fill="#e8e1fb"/>' +
    // 街区建筑面。
    '<rect x="30" y="96" width="46" height="26" rx="2" fill="#e6e8eb"/>' +
    '<rect x="112" y="106" width="58" height="22" rx="2" fill="#e6e8eb"/>' +
    '<rect x="212" y="18" width="52" height="34" rx="2" fill="#e6e8eb"/>' +
    '<rect x="20" y="30" width="34" height="22" rx="2" fill="#e6e8eb"/>' +
    // 主干道（白色路面 + 快速路黄色涂装）。
    '<g stroke="#fff" fill="none" stroke-linecap="round">' +
    '<path d="M0,86 L300,72" stroke-width="11"/><path d="M60,0 L72,170" stroke-width="9"/>' +
    '<path d="M202,0 L192,170" stroke-width="9"/>' +
    '<path d="M0,38 Q150,50 300,30" stroke-width="6.5"/><path d="M0,120 L300,110" stroke-width="6"/>' +
    '<path d="M134,0 L148,170" stroke-width="4.5"/><path d="M262,0 L252,170" stroke-width="4.5"/>' +
    '</g>' +
    '<path d="M0,86 L300,72" stroke="#f8d66c" stroke-width="1.2" fill="none"/>' +
    // 地名与路名（描边保证在路面上可读）。
    '<g fill="#8a9099" font-size="6.5" stroke="#fff" stroke-width="2" paint-order="stroke">' +
    '<text x="96" y="81">世纪大道</text><text x="204" y="42">张江高科技园区</text>' +
    '<text x="24" y="52">运河春天</text><text x="230" y="122">新城家园</text>' +
    '<text x="146" y="60">职业技术大学</text><text x="150" y="118">锦绣路</text>' +
    '</g>' +
    // POI 小图标。
    '<g><rect x="96" y="112" width="7" height="7" rx="1.5" fill="#4a90e2"/>' +
    '<circle cx="128" cy="34" r="3.2" fill="#f5a623"/><circle cx="240" cy="66" r="3.2" fill="#7ed321"/>' +
    '<rect x="46" y="60" width="6" height="6" rx="1" fill="#f56c6c"/></g>' +
    // 定位蓝点（脉冲光圈）。
    '<circle class="pulse" cx="66" cy="82" r="7" fill="#2f7cf6" opacity="0.35"/>' +
    '<circle cx="66" cy="82" r="4.6" fill="#2f7cf6" stroke="#fff" stroke-width="1.6"/>' +
    (start ? pin(60, 56, '#67c23a') : '') +
    (end ? pin(212, 50, '#f56c6c') : '') +
    (start && end ? '<path d="M60,56 Q136,86 212,50" stroke="#2f7cf6" stroke-width="2.6" fill="none" stroke-dasharray="5,4"/>' : '') +
    '</svg></div>';
}

function activeOrder() {
  const live = ['PENDING', 'WAITING', 'ACCEPTED', 'ARRIVED', 'IN_TRIP', 'IN_SERVICE', 'COMPLETED_UNPAID'];
  if (S.currentOrder && live.includes(S.currentOrder.state)) return S.currentOrder;
  return S.orders.find((o) => live.includes(o.state)) || null;
}

// ---------------------------------------------------------------- 行程状态轮询

let flowTimer = null;

/** startFlowPoll 每 4 秒回查当前订单：司机接单/到达/行程中/完单实时回传。 */
function startFlowPoll() {
  stopFlowPoll();
  flowTimer = setInterval(async () => {
    if (!S.currentOrder || !loggedIn()) { stopFlowPoll(); return; }
    try {
      const data = await api(`/passenger/orders/current?accountId=${encodeURIComponent(S.accountId)}`, {
        accountId: S.accountId, token: S.token,
      });
      // 查询路径上后端会先做派单状态懒同步；无进行中订单时结束轮询。
      const payload = data && (data.order || data);
      const no = payload && (payload.orderNo || payload.order_no);
      if (!payload || !no) { stopFlowPoll(); return; }
      const before = `${S.currentOrder.state}|${S.currentOrder.driverName}`;
      S.currentOrder.no = no || S.currentOrder.no;
      S.currentOrder.state = payload.state || S.currentOrder.state;
      S.currentOrder.driverName = payload.driverName || payload.driver_name || '';
      S.currentOrder.driverPlate = payload.driverPlate || payload.driver_plate || '';
      S.currentOrder.payFee = (payload.totalFee != null ? payload.totalFee : payload.total_fee) != null
        ? (payload.totalFee != null ? payload.totalFee : payload.total_fee) : S.currentOrder.payFee;
      // 待支付不是终态：保持轮询，支付完成后状态变为 COMPLETED 时自动收尾。
      const terminal = ['COMPLETED', 'CANCELLED', 'ENDED'].includes(S.currentOrder.state);
      if (terminal) stopFlowPoll();
      if (`${S.currentOrder.state}|${S.currentOrder.driverName}` !== before) {
        const name = S.currentOrder.driverName;
        if (name && !S.flowNotified) { toast(`司机 ${name} 已接单`); S.flowNotified = true; }
        else if (terminal) toast(stateName(S.currentOrder.state));
        renderAll();
      }
    } catch { /* 网关暂不可达时下一轮重试 */ }
  }, 4000);
}

function stopFlowPoll() {
  if (flowTimer) { clearInterval(flowTimer); flowTimer = null; }
}

/**
 * cancelRide 乘客在等待接单期间取消打车。
 * 后端为两段式：取消费为 0 时一次调用即取消；需收费时先返回报价，
 * 弹确认框带 confirmFee 再调一次。
 */
async function cancelRide() {
  const order = S.currentOrder;
  if (!order || !order.orderId) { toast('当前没有可取消的行程'); return; }
  if (!window.confirm('确定要取消本次打车吗？')) return;
  const body = { orderId: order.orderId, reason: '乘客主动取消' };
  try {
    let data = await api('/passenger/orders/cancel', {
      method: 'POST', accountId: S.accountId, token: S.token,
      body: { ...body, confirmFee: false },
    });
    if (data && data.needConfirm) {
      const fee = data.cancelFee != null ? data.cancelFee : 0;
      if (!window.confirm(`取消将产生取消费 ¥${fy(fee)}，确认取消吗？`)) return;
      data = await api('/passenger/orders/cancel', {
        method: 'POST', accountId: S.accountId, token: S.token,
        body: { ...body, confirmFee: true },
      });
    }
    S.currentOrder = null;
    S.flowNotified = false;
    stopFlowPoll();
    toast('已取消本次打车');
    await loadCoupons();
  } catch (error) {
    toast(`取消失败：${error.message}`);
  }
  renderAll();
}

/**
 * payOrder 待支付订单支付：创建支付单 → 渠道回调（演示环境模拟支付成功）。
 * 回调成功后网关联动下游：订单推进到已完成、司机域记账结算。
 */
async function payOrder(orderNo) {
  if (!loggedIn()) { toast('请先登录'); return; }
  const order = S.orders.find((o) => o.no === orderNo) ||
    (S.currentOrder && S.currentOrder.no === orderNo ? S.currentOrder : null);
  const fee = order && order.payFee != null ? order.payFee : 0;
  try {
    toast('正在拉起支付…');
    await api('/payments', {
      method: 'POST', accountId: S.accountId, token: S.token,
      body: {
        order_no: orderNo, subject: '花小猪打车·车费', amount: fee,
        channel: 'wechat', user_id: S.accountId,
        idempotent_key: `pay-${orderNo}`,
      },
    });
    const tradeNo = `demo-${Date.now()}`;
    await api('/payments/notify', {
      method: 'POST', accountId: S.accountId, token: S.token,
      body: { order_no: orderNo, trade_no: tradeNo, channel: 'wechat', paid_amount: fee, paid_at_unix: Math.floor(Date.now() / 1000) },
    });
    toast('支付成功，订单已完成');
    S.flowNotified = false;
    // 支付后订单进入终态：清掉行程卡回首页，订单列表同步刷新成已完成，
    // 否则待支付卡片会一直挂在行程视图上。
    stopFlowPoll();
    if (S.currentOrder) {
      S.currentOrder.state = 'COMPLETED';
      const row = S.orders.find((o) => o.no === S.currentOrder.no);
      if (row) row.state = 'COMPLETED';
    }
    S.currentOrder = null;
    S.tab = 'home';
    await loadOrderList();
  } catch (error) {
    toast(`支付失败：${error.message}`);
  }
  renderAll();
}

/** loadOrderList 从后端拉取订单列表（订单 tab 展示与待支付续付的数据源）。 */
async function loadOrderList() {
  if (!loggedIn()) return;
  try {
    const data = await api(`/passenger/orders?accountId=${encodeURIComponent(S.accountId)}&page_no=1&page_size=20`, {
      accountId: S.accountId, token: S.token,
    });
    const list = (data && (data.list || data.orders)) || [];
    S.orders = list.map((o) => ({
      no: o.order_no || o.orderNo || '-',
      orderId: o.order_id || o.orderId || '',
      state: o.state || 'PENDING',
      typeName: carName(o.car_type),
      payFee: o.total_fee != null ? o.total_fee : 0,
      startName: o.start_name || '-', endName: o.end_name || '-',
      startAddr: o.start_addr || '', endAddr: o.end_addr || '',
      createdAtText: o.created_at
        ? new Date(o.created_at * 1000).toLocaleString('zh-CN', { month: '2-digit', day: '2-digit', hour: '2-digit', minute: '2-digit' }) : '',
    }));
    renderAll();
  } catch { /* 拉取失败保留会话内列表 */ }
}

window.paxPayOrder = payOrder;

function paxHomeView() {
  const start = S.start;
  const end = S.end;
  const est = S.estimate;
  let h = '';

  h += `<div style="background:#ffc300;padding:10px 14px 4px;"><b style="font-size:15px;">花小猪打车</b>` +
    `<div style="font-size:10px;color:#6b5900;">${S.phone ? esc(S.phone) + ' · 已登录' : '未登录 · 叫车前需登录'}</div></div>`;

  h += mapMock(start, end);

  h += '<div class="card">' +
    `<div class="poirow" onclick="paxOpenPoi('start')"><span class="dot" style="background:#67c23a"></span>` +
    `<span class="txt"><b>${start ? esc(start.name) : '您在哪里上车？'}</b><span>${start ? esc(start.addr) : '点击选择上车点'}</span></span><span class="arrow">›</span></div>` +
    `<div class="poirow" onclick="paxOpenPoi('end')"><span class="dot" style="background:#f56c6c"></span>` +
    `<span class="txt"><b>${end ? esc(end.name) : '您要去哪里？'}</b><span>${end ? esc(end.addr) : '点击选择目的地'}</span></span><span class="arrow">›</span></div>`;

  if (est) {
    h += `<div class="note">预估 ${(est.distance_m / 1000).toFixed(1)}公里 · 约${Math.round(est.duration_s / 60)}分钟 · ` +
      `预估价 ¥${fy(est.pay_fee)} · 有效期 <b>${estLeft(est)}</b>s</div>`;
  }
  h += '</div>';

  if (S.orderError) {
    h += `<div class="card" style="background:#fef0f0;border:1px solid #fbc4c4;"><b style="color:#f56c6c;">⚠ 叫车未成功</b>` +
      `<div class="note" style="color:#f56c6c;">${esc(S.orderError)}</div></div>`;
  }

  // 实名是叫车前置条件：未认证时直接给出提交入口，不必等下单被拒。
  if (loggedIn() && S.realname && S.realname.state !== 'VERIFIED') {
    h += '<div class="card" style="background:#fff8e6;border:1px solid #f5d99b;"><b>⚠ 未完成实名认证</b>' +
      '<div class="note">平台要求实名认证后才能叫车；提交姓名与身份证号即可完成认证。</div>' +
      '<div class="btnrow" style="margin-top:8px;"><button class="btn yellow" onclick="paxOpenRealName()">立即实名认证</button></div></div>';
  }

  if (start && end) {
    h += '<div class="card typelist"><h4>选择车型</h4>';
    for (const c of S.cars) {
      const price = est && est.car_type === c.car_type ? `<b>¥${fy(est.pay_fee)}</b><small style="color:#909399;">预估</small>` : '';
      h += `<div class="trow ${S.carType === c.car_type ? 'on' : ''}" onclick="paxSetType(${c.car_type})">` +
        `<span><b>${esc(c.name)}</b><small>${esc(c.desc || CAR_DESC[c.car_type] || '')}</small></span>` +
        `<span class="pr">${price}</span></div>`;
    }
    h += '</div>';

    // 优惠券：行式券卡（面值 + 名称门槛 + 选中圆点），比 chip 更整齐易读。
    h += '<div class="card"><div class="cardhead"><h4>优惠券</h4>' +
      '<span class="link" onclick="paxOpenClaim()">领券中心 ›</span></div>';
    h += '<div class="couponlist">';
    h += `<div class="crow ${S.couponId ? '' : 'on'}" onclick="paxSelectCoupon('')">` +
      '<div class="meta"><b>不用券</b><small>按预估价原价下单</small></div><span class="pick"></span></div>';
    for (const c of S.coupons) {
      // 满减门槛高于本单预估价时禁选，避免选了也无法抵扣。
      const below = est && c.threshold > 0 && est.pay_fee < c.threshold;
      const cls = ['crow', S.couponId === c.coupon_id ? 'on' : '', below ? 'dis' : ''].join(' ').trim();
      h += `<div class="${cls}" ${below ? '' : `onclick="paxSelectCoupon('${esc(c.coupon_id)}')"`}>` +
        `<div class="face"><b>¥${fy(c.face_value)}</b>${c.threshold ? `<small>满${fy(c.threshold)}可用</small>` : '<small>无门槛</small>'}</div>` +
        `<div class="meta"><b>${esc(c.name || '打车券')}</b><small>到期 ${esc(expireText(c.expires_at))}${below ? ' · 未满门槛' : ''}</small></div>` +
        '<span class="pick"></span></div>';
    }
    if (!S.coupons.length) h += '<div class="note" style="padding:2px 0 4px;">暂无可用券，去领券中心逛逛</div>';
    h += '</div>';
    if (S.estimate && S.estimate.discount_fee > 0) {
      h += `<div class="note" style="color:#e6a23c;">已选券抵扣 ¥${fy(S.estimate.discount_fee)}，应付 ¥${fy(S.estimate.pay_fee)}</div>`;
    }
    h += '</div>';

    h += `<div class="callbar"><button onclick="paxCallCar()">呼叫${esc(carName(S.carType))}</button>` +
      `<div class="note">预估价格，最终按实际里程时长计费</div></div>`;
  }
  return h;
}

/** estLeft 估价剩余秒数：后端返回绝对 expires_at（Unix 秒）。 */
function estLeft(est) {
  if (!est) return 0;
  if (est.expires_at) return Math.max(0, Math.ceil(est.expires_at - Date.now() / 1000));
  return 30;
}

function paxFlowView(order) {
  const waiting = ['PENDING', 'WAITING'].includes(order.state);
  const headline = waiting ? '正在为您呼叫车辆' : `${stateName(order.state)}`;
  let h = `<div class="statbig"><div class="t1">${headline}</div>` +
    `<div class="t2">${waiting ? '订单已进入司机派单大厅 · 可切到「司机端」演示接单' : '状态实时同步自司机端'}</div></div>`;
  h += `<div class="card"><h4>${esc(order.typeName)} · ¥${fy(order.payFee)}（预估）</h4>` +
    `<div class="poirow" style="cursor:default"><span class="dot" style="background:#67c23a"></span><span class="txt"><b>${esc(order.startName)}</b><span>${esc(order.startAddr)}</span></span></div>` +
    `<div class="poirow" style="cursor:default"><span class="dot" style="background:#f56c6c"></span><span class="txt"><b>${esc(order.endName)}</b><span>${esc(order.endAddr)}</span></span></div>` +
    (order.driverName ? `<div class="poirow" style="cursor:default"><span class="dot" style="background:#2f6fed"></span>` +
      `<span class="txt"><b>司机 ${esc(order.driverName)}${order.driverPlate ? ' · ' + esc(order.driverPlate) : ''}</b><span>已接单，请在上车点等候</span></span></div>` : '') +
    `<div style="margin-top:6px;"><span class="chip st-${esc(flowChipState(order.state))}">${esc(stateName(order.state))}</span>` +
    `<span class="note" style="display:inline;margin-left:8px;">订单号 ${esc(order.no || '-')}</span></div></div>`;
  if (waiting) {
    h += '<div class="note" style="text-align:center;">状态每 4 秒自动刷新 · 司机接单后立即更新</div>';
    // 等待接单期间允许乘客无责取消；司机接单后需走客服协商。
    h += '<button class="btn" style="width:100%;margin-top:10px;background:#fff;color:#f56c6c;border:1px solid #fbc4c4;" onclick="paxCancelRide()">取消打车</button>';
  }
  // 行程结束即待支付：展示应付金额并引导乘客完成支付，支付成功后订单才算完成。
  if (order.state === 'COMPLETED_UNPAID') {
    h += `<div class="card" style="text-align:center;"><h4>行程结束 · 待支付</h4>` +
      `<div style="font-size:26px;color:#303133;margin:6px 0;">¥${fy(order.payFee)}</div>` +
      `<div class="note">支付完成后订单才会计入司机收入</div>` +
      `<button class="btn yellow" style="width:100%;margin-top:8px;" onclick="paxPayOrder('${esc(order.no)}')">立即支付 ¥${fy(order.payFee)}</button></div>`;
  }
  return h;
}

/** flowChipState 把乘客域状态映射到样式芯片（复用 WAITING 系配色）。 */
function flowChipState(state) {
  return { PENDING: 'WAITING', IN_SERVICE: 'IN_TRIP', COMPLETED_UNPAID: 'ENDED', COMPLETED: 'ENDED' }[state] || state;
}

function paxOrdersView() {
  let h = '<div style="background:#ffc300;padding:10px 14px 6px;"><b style="font-size:15px;">我的订单</b>' +
    '<div style="font-size:10px;color:#6b5900;">仅显示本人订单 · 金额按阶段标注</div></div>';
  if (!S.orders.length) h += '<div class="card" style="text-align:center;color:#909399;padding:30px 0;">暂无订单</div>';
  for (const o of S.orders) {
    const unpaid = o.state === 'COMPLETED_UNPAID';
    const feeLabel = ['COMPLETED', 'COMPLETED_UNPAID'].includes(o.state) ? '实付' : '预估';
    h += `<div class="card" onclick="paxOrderDetail('${esc(o.no)}')" style="cursor:pointer;">` +
      `<div style="display:flex;justify-content:space-between;margin-bottom:5px;"><b>${esc(o.typeName)} · ${esc(o.startName)} → ${esc(o.endName)}</b>` +
      `<span class="chip st-${esc(o.state)}">${esc(stateName(o.state))}</span></div>` +
      `<div style="display:flex;justify-content:space-between;font-size:11px;color:#909399;"><span>${esc(o.createdAtText || '')}</span>` +
      `<b style="color:#303133;">${feeLabel} ¥${fy(o.payFee)}</b></div>` +
      (unpaid ? `<button class="btn yellow" style="width:100%;margin-top:8px;" onclick="event.stopPropagation();paxPayOrder('${esc(o.no)}')">去支付 ¥${fy(o.payFee)}</button>` : '') +
      `</div>`;
  }
  return h;
}

function paxMeView() {
  let h = '<div style="background:#ffc300;padding:14px;"><div class="drvcard">' +
    '<div class="avatar" style="background:#fff;">乘</div><div><b>乘客</b>' +
    `<div style="font-size:11px;">${S.phone ? esc(S.phone) : '未登录'} · ${S.accountId ? '已登录' : '未登录'}</div></div></div></div>`;
  const rn = S.realname;
  const verified = rn && rn.state === 'VERIFIED';
  h += '<div class="card"><h4>实名认证</h4><div class="feelist">' +
    `<div><span>认证状态</span><span style="color:${verified ? '#67c23a' : '#e6a23c'};">${verified ? '已实名' : (loggedIn() ? '未认证' : '登录后可见')}</span></div>` +
    (verified ? `<div><span>姓名</span><span>${esc(rn.name_mask || '-')}</span></div>` +
      `<div><span>证件号</span><span>${esc(rn.id_no_mask || '-')}</span></div>` : '') + '</div>' +
    (verified ? '<div class="note">已通过实名认证，可直接叫车。</div>'
      : '<div class="note">平台要求实名认证后才能叫车；服务端只保存脱敏值，不保存姓名与证件号明文。</div>' +
        '<div class="btnrow" style="margin-top:8px;"><button class="btn yellow" onclick="paxOpenRealName()">立即实名认证</button></div>') +
    '</div>';
  h += '<div class="card"><h4>账号信息</h4><div class="feelist">' +
    `<div><span>账号 ID</span><span>${S.accountId ? esc(S.accountId) : '-'}</span></div>` +
    `<div><span>手机号</span><span>${S.phone ? esc(S.phone) : '-'}</span></div>` +
    `<div><span>我的优惠券</span><span>${S.coupons.length} 张可用</span></div></div></div>`;
  // 我的券：列出已领取未使用的券，点击即可选中并回到首页下单时抵扣。
  h += '<div class="card"><div class="cardhead"><h4>我的券</h4>' +
    '<span class="link" onclick="paxOpenClaim()">领券中心 ›</span></div>';
  h += '<div class="couponlist">';
  if (!S.coupons.length) h += '<div class="note" style="padding:2px 0 4px;">暂无可用券，去领券中心逛逛</div>';
  for (const c of S.coupons) {
    h += `<div class="crow ${S.couponId === c.coupon_id ? 'on' : ''}" onclick="paxPickCoupon('${esc(c.coupon_id)}')">` +
      `<div class="face"><b>¥${fy(c.face_value)}</b>${c.threshold ? `<small>满${fy(c.threshold)}可用</small>` : '<small>无门槛</small>'}</div>` +
      `<div class="meta"><b>${esc(c.name || '打车券')}</b><small>到期 ${esc(expireText(c.expires_at))}${S.couponId === c.coupon_id ? ' · 已选中' : ' · 可用'}</small></div>` +
      '<span class="pick"></span></div>';
  }
  h += '</div></div>';
  if (S.accountId) {
    h += '<div class="btnrow" style="margin:10px;"><button class="btn ghost" onclick="paxLogout()">退出登录</button></div>';
  }
  const hotline = (S.service && S.service.hotline) || '400-810-5335';
  h += '<div class="card"><h4>在线客服</h4>' +
    `<div style="font-size:12px;color:#606266;line-height:1.7;">统一热线：${esc(hotline)}<br>` +
    `服务时间：${esc((S.service && S.service.service_time) || '每日 07:00 - 23:00（节假日不休）')}</div>` +
    '<div class="btnrow" style="margin-top:8px;"><button class="btn blue" onclick="paxOpenService()">进入客服中心</button></div></div>';
  return h;
}

// ---------------------------------------------------------------- 弹层

function poiSheet(kind) {
  const list = S.sheet.results && S.sheet.results.length ? S.sheet.results : POIS;
  let h = '<h3>选择' + (kind === 'start' ? '上车点' : '目的地') + ' <button class="x" onclick="paxCloseSheet()">✕</button></h3>';
  h += '<div style="display:flex;gap:6px;margin-bottom:8px;">' +
    `<input id="poiKw" placeholder="搜索地点（如：张江）" value="${esc(S.sheet.kw || '')}" style="flex:1">` +
    '<button class="btn blue" style="flex:none;padding:8px 14px;" onclick="paxSearchPoi()">搜索</button></div>';
  for (const p of list) {
    h += `<div class="poirow" onclick="paxChoosePoi('${kind}','${esc(p.id)}')"><span class="dot" style="background:${kind === 'start' ? '#67c23a' : '#f56c6c'}"></span>` +
      `<span class="txt"><b>${esc(p.name)}</b><span>${esc(p.addr || '')}</span></span></div>`;
  }
  return h;
}

function loginSheet() {
  let h = '<h3>登录花小猪 <button class="x" onclick="paxCloseSheet()">✕</button></h3>';
  h += '<div class="note" style="margin-bottom:8px;">本地环境验证码会直接随响应返回并回填。</div>';
  h += '<div style="display:flex;gap:6px;margin-bottom:8px;">' +
    `<input id="loginPhone" type="tel" placeholder="手机号" value="${esc(S.sheet.phone || '13800000000')}" style="flex:1">` +
    '<button class="btn blue" style="flex:none;padding:8px 14px;" onclick="paxSendCode()">获取验证码</button></div>';
  h += '<div style="display:flex;gap:6px;">' +
    `<input id="loginCode" placeholder="验证码" value="${esc(S.codeSent)}" style="flex:1">` +
    '<button class="btn dark" style="flex:none;padding:8px 18px;" onclick="paxDoLogin()">登录</button></div>';
  if (S.sheet.error) h += `<div class="note" style="color:#f56c6c;">${esc(S.sheet.error)}</div>`;
  return h;
}

function orderDetailSheet(order) {
  let h = `<h3>订单详情 <button class="x" onclick="paxCloseSheet()">✕</button></h3>`;
  h += `<div style="display:flex;gap:6px;margin-bottom:8px;"><span class="chip st-${esc(order.state)}">${esc(stateName(order.state))}</span></div>`;
  h += '<div class="feelist" style="background:#f7f8fa;border-radius:8px;padding:10px;margin-bottom:8px;">' +
    `<div><span>车型</span><span>${esc(order.typeName)}</span></div>` +
    `<div><span>路线</span><span>${esc(order.startName)} → ${esc(order.endName)}</span></div>` +
    `<div><span>订单号</span><span>${esc(order.no || '-')}</span></div>` +
    `<div><span>下单时间</span><span>${esc(order.createdAtText || '-')}</span></div>` +
    `<div class="total"><span>预估车费</span><span>¥${fy(order.payFee)}</span></div></div>`;
  h += '<div class="tl"><div class="ev"><b>OrderCreated</b><span>订单已创建，等待派单</span></div></div>';
  return h;
}

/** realNameSheet 实名认证表单：提交姓名与身份证号，服务端只回脱敏值。 */
function realNameSheet() {
  let h = '<h3>实名认证 <button class="x" onclick="paxCloseSheet()">✕</button></h3>';
  h += '<div class="note" style="margin-bottom:8px;">按监管要求，叫车前需完成实名认证。信息仅用于身份校验，服务端只保存脱敏结果。</div>';
  h += '<input id="rnName" placeholder="真实姓名" style="width:100%;margin-bottom:8px;box-sizing:border-box;">';
  h += '<input id="rnIdNo" placeholder="身份证号（15/18 位）" style="width:100%;box-sizing:border-box;">';
  if (S.sheet.error) h += `<div class="note" style="color:#f56c6c;">${esc(S.sheet.error)}</div>`;
  h += '<div class="btnrow" style="margin-top:10px;"><button class="btn yellow" onclick="paxSubmitRealName()">提交认证</button></div>';
  return h;
}

/** claimCouponSheet 领券中心：券由管理端营销中心发放，乘客领取后落入本人券包。 */
function claimCouponSheet() {
  let h = '<h3>领券中心 <button class="x" onclick="paxCloseSheet()">✕</button></h3>';
  // 平台券落入本人券包后的 ID 形如 plat-<券码>-<账号>，用于判断是否已领取。
  if (!S.claimable.length) {
    h += '<div class="note">暂无可领取的券，可先在管理端「营销中心」发放。</div>';
  }
  h += '<div class="couponlist">';
  for (const c of S.claimable) {
    // 已领取以后端标记为准；本地记录兜底（领取成功到列表刷新之间即时置灰）。
    const got = c.claimed || S.claimedNos.has(c.coupon_id);
    h += `<div class="crow ${got ? 'dis' : ''}">` +
      `<div class="face"><b>¥${fy(c.face_value)}</b>${c.threshold ? `<small>满${fy(c.threshold)}可用</small>` : '<small>无门槛</small>'}</div>` +
      `<div class="meta"><b>${esc(c.name || '打车券')}</b><small>到期 ${esc(expireText(c.expires_at || c.expire_at))}</small></div>` +
      (got ? '<button class="btn got" disabled>已领取</button>'
        : `<button class="btn yellow" style="padding:5px 14px;font-size:11px;" onclick="paxClaimCoupon('${esc(c.coupon_id)}')">领取</button>`) +
      '</div>';
  }
  h += '</div>';
  if (S.sheet.error) h += `<div class="note" style="color:#f56c6c;">${esc(S.sheet.error)}</div>`;
  return h;
}

/** serviceFullView 客服中心全屏页：热线 + 常见问题（手风琴）+ 我的工单。 */
function serviceFullView() {
  const svc = S.service || {};
  const faq = svc.faq && svc.faq.length ? svc.faq : FALLBACK_FAQ;
  let h = '<div class="fullpage">';
  h += '<div class="fphead"><span class="back" onclick="paxCloseService()">‹ 返回</span>' +
    '<div class="fpheadtxt"><b>客服中心</b><small>FAQ · 热线 · 工单进度</small></div></div>';
  h += `<div class="hotline"><span>7×24 小时客服热线</span><b>${esc(svc.hotline || '400-810-5335')}</b>` +
    `<small>${esc(svc.service_time || '每日 07:00 - 23:00（节假日不休）')}</small></div>`;
  h += '<div class="card"><h4>常见问题</h4>';
  for (let i = 0; i < faq.length; i++) {
    const item = faq[i];
    const open = S.faqOpen === i;
    h += `<div class="faqrow" onclick="paxToggleFaq(${i})">` +
      `<div class="q"><em>Q${i + 1}</em>${esc(item.q || item.question || '')}<span class="arrow">${open ? '−' : '+'}</span></div>` +
      (open ? `<div class="a">${esc(item.a || item.answer || '')}</div>` : '') +
      '</div>';
  }
  h += '</div>';
  h += '<div class="card"><h4>我的工单</h4>';
  const tickets = svc.my_tickets || [];
  if (!tickets.length) h += '<div class="note">暂无进行中的工单，有问题可拨打上方热线反馈。</div>';
  for (const t of tickets) {
    h += `<div class="faqrow" style="cursor:default;"><div class="q"><em>#</em>${esc(t.ticket_no || t.ticket_id || '工单')}</div>` +
      `<div class="a">${esc(t.progress || t.state || '')}</div></div>`;
  }
  h += '</div></div>';
  return h;
}

/** toggleFaq 展开/收起一条常见问题。 */
function toggleFaq(index) {
  S.faqOpen = S.faqOpen === index ? null : index;
  renderAll();
}

// ---------------------------------------------------------------- 动作

function closeSheet() { S.sheet = null; renderAll(); }

function openPoi(kind) { S.sheet = { type: 'poi', kind }; renderAll(); }

async function searchPoi() {
  const input = document.getElementById('poiKw');
  const kw = input ? input.value.trim() : '';
  S.sheet.kw = kw;
  if (!kw) { S.sheet.results = null; renderAll(); return; }
  try {
    const query = new URLSearchParams({
      keyword: kw, hasCoord: 'true', lng: '121.601', lat: '31.2054', limit: '8',
    });
    if (S.accountId) query.set('accountId', S.accountId);
    const data = await api(`/passenger/poi/search?${query.toString()}`, { accountId: S.accountId, token: S.token });
    const list = (data.list || []).map((p, i) => ({
      id: `s_${i}_${p.lng}_${p.lat}`,
      name: p.name || p.title || '未命名地点',
      addr: p.addr || p.address || p.city || '',
      lng: p.lng, lat: p.lat,
    }));
    S.sheet.results = list;
    if (!list.length) toast('没有匹配的地点');
  } catch (error) {
    toast(`搜索失败：${error.message}`);
  }
  renderAll();
}

async function choosePoi(kind, id) {
  const pool = (S.sheet && S.sheet.results) || POIS;
  const p = pool.find((x) => x.id === id) || POIS.find((x) => x.id === id);
  if (!p) return;
  if (kind === 'start') S.start = p;
  else S.end = p;
  S.sheet = null;
  renderAll();
  await refreshEstimate();
}

function setType(carType) { S.carType = carType; renderAll(); refreshEstimate(); }

// ---------------------------------------------------------------- 实名 / 优惠券 / 客服

async function loadRealname() {
  if (!loggedIn()) { S.realname = null; return; }
  try {
    S.realname = await api('/passenger/realname', { accountId: S.accountId, token: S.token });
  } catch { S.realname = null; }
}

function openRealName() {
  if (!loggedIn()) { toast('请先登录'); openLogin(); return; }
  S.sheet = { type: 'realname' };
  renderAll();
}

async function submitRealName() {
  const name = (document.getElementById('rnName') || {}).value || '';
  const idNo = (document.getElementById('rnIdNo') || {}).value || '';
  if (!name.trim()) { S.sheet.error = '请填写真实姓名'; renderAll(); return; }
  if (!/^\d{15}$|^\d{17}[0-9Xx]$/.test(idNo.trim())) {
    S.sheet.error = '身份证号格式不正确（15 位或 18 位）';
    renderAll();
    return;
  }
  try {
    // 幂等键由页面生成：重复提交返回同一结果，不会重复写入。
    S.realname = await api('/passenger/realname', {
      method: 'POST', accountId: S.accountId, token: S.token,
      body: { name: name.trim(), idNo: idNo.trim(), idempotencyKey: randomKey('rn') },
    });
    S.sheet = null;
    toast('实名认证已通过');
    await refreshEstimate();
  } catch (error) {
    S.sheet.error = error.message;
  }
  renderAll();
}

async function loadCoupons() {
  if (!loggedIn()) { S.coupons = []; return; }
  try {
    const data = await api(`/passenger/coupons?accountId=${encodeURIComponent(S.accountId)}`, {
      accountId: S.accountId, token: S.token,
    });
    S.coupons = data.list || [];
    // 选中的券已不再可用（用完/过期/被锁定）时自动回到「不用券」。
    if (S.couponId && !S.coupons.some((c) => c.coupon_id === S.couponId)) S.couponId = '';
  } catch { S.coupons = []; }
}

async function openClaim() {
  if (!loggedIn()) { toast('请先登录'); openLogin(); return; }
  S.sheet = { type: 'claim' };
  renderAll();
  await loadClaimable();
}

async function loadClaimable() {
  try {
    const data = await api(`/passenger/coupons/claimable?accountId=${encodeURIComponent(S.accountId)}&limit=20`, {
      accountId: S.accountId, token: S.token,
    });
    S.claimable = data.list || [];
  } catch (error) {
    S.claimable = [];
    if (S.sheet && S.sheet.type === 'claim') S.sheet.error = `加载失败：${error.message}`;
  }
  renderAll();
}

async function claimCoupon(couponNo) {
  try {
    const data = await api('/passenger/coupons/claim', {
      method: 'POST', accountId: S.accountId, token: S.token,
      body: { couponNo },
    });
    // 本地记录已领券码（服务端对重复领取幂等返回既有券）。
    // 置灰判断不能只依赖可用券列表：已领的券可能被订单锁定而不可用但确实已领取。
    S.claimedNos.add(couponNo);
    try { localStorage.setItem('huazhu.claimedNos', JSON.stringify([...S.claimedNos])); } catch { /* 隐私模式下忽略 */ }
    toast('领取成功，可在「我的券」中查看');
    await loadCoupons();
    await loadClaimable();
    await refreshEstimate();
  } catch (error) {
    if (S.sheet) S.sheet.error = error.message;
    toast(`领取失败：${error.message}`);
    renderAll();
  }
}

async function selectCoupon(couponId) {
  S.couponId = couponId === S.couponId ? '' : couponId;
  renderAll();
  await refreshEstimate();
}

/** pickCoupon 在「我的」页点券：选中并回到首页准备叫车。 */
function pickCoupon(couponId) {
  S.couponId = S.couponId === couponId ? '' : couponId;
  S.tab = 'home';
  toast(S.couponId ? '已选择该券，叫车时自动抵扣' : '已取消选择该券');
  renderAll();
  refreshEstimate();
}

async function loadService() {
  if (!loggedIn()) { S.service = null; return; }
  try {
    S.service = await api('/passenger/service', { accountId: S.accountId, token: S.token });
  } catch { S.service = null; }
}

function openService() {
  S.fullpage = 'service';
  S.faqOpen = null;
  renderAll();
  loadService();
}

function closeService() {
  S.fullpage = null;
  S.faqOpen = null;
  renderAll();
}

/** refreshEstimate 重新估价：登录用户走正式估价（有单号），游客走看价。 */
async function refreshEstimate() {
  const start = S.start;
  const end = S.end;
  if (!start || !end) { S.estimate = null; renderAll(); return; }
  const body = {
    carType: S.carType,
    startLng: start.lng, startLat: start.lat,
    endLng: end.lng, endLat: end.lat,
  };
  // 选中的券参与估价试算：服务端校验门槛与有效期后返回抵扣后的应付金额。
  if (loggedIn() && S.couponId) body.couponId = S.couponId;
  try {
    if (loggedIn()) {
      body.accountId = S.accountId;
      S.estimate = await api('/passenger/estimate', { method: 'POST', body, accountId: S.accountId, token: S.token });
    } else {
      S.estimate = await api('/passenger/estimate/quote', { method: 'POST', body });
    }
  } catch (error) {
    S.estimate = null;
    toast(`估价失败：${error.message}`);
  }
  renderAll();
}

function openLogin() { S.sheet = { type: 'login' }; renderAll(); }

async function sendCode() {
  const input = document.getElementById('loginPhone');
  const phone = input ? input.value.trim() : '';
  if (!phone) { S.sheet.error = '请输入手机号'; renderAll(); return; }
  try {
    const data = await api('/passenger/auth/sms-code', { method: 'POST', body: { phone } });
    S.codeSent = data.debug_code || '';
    S.sheet.phone = phone;
    toast(`验证码已发送：${S.codeSent}`);
  } catch (error) {
    S.sheet.error = error.message;
  }
  renderAll();
}

async function doLogin() {
  const phone = (document.getElementById('loginPhone') || {}).value || S.sheet.phone || '';
  const code = (document.getElementById('loginCode') || {}).value || S.codeSent || '';
  if (!phone.trim() || !code.trim()) { S.sheet.error = '手机号与验证码都要填'; renderAll(); return; }
  try {
    const data = await api('/passenger/auth/login', {
      method: 'POST',
      body: { channel: 'sms', phone: phone.trim(), code: code.trim(), role: 'passenger' },
    });
    S.accountId = data.account_id;
    S.token = data.access_token;
    S.phone = phone.trim();
    S.sheet = null;
    toast('登录成功');
    await loadHome();
    // 登录后补齐实名、券包与客服数据，供首页用车与「我的」页使用。
    await Promise.all([loadRealname(), loadCoupons(), loadService()]);
    // 登录后恢复进行中订单：页面刷新或换设备后仍能看到行程状态并继续轮询。
    await restoreCurrentOrder();
    await refreshEstimate();
  } catch (error) {
    S.sheet.error = error.message;
    renderAll();
  }
}

async function loadHome() {
  if (!loggedIn()) return;
  try {
    const data = await api(`/passenger/home?accountId=${encodeURIComponent(S.accountId)}`, {
      accountId: S.accountId, token: S.token,
    });
    if (Array.isArray(data.car_types) && data.car_types.length) {
      S.cars = data.car_types.map((c) => ({ ...c, desc: CAR_DESC[c.car_type] || '' }));
    }
  } catch (error) {
    toast(`拉取车型失败：${error.message}`);
  }
}

async function callCar() {
  const start = S.start;
  const end = S.end;
  if (!start || !end) { toast('请先选择上车点与目的地'); return; }
  if (!loggedIn()) { openLogin(); return; }
  if (!S.estimate || !S.estimate.estimate_no) { toast('请先完成估价'); return; }
  // 实名是后端强制前置条件，本地先拦一次，避免必然失败的请求。
  if (S.realname && S.realname.state !== 'VERIFIED') {
    if (!S.realname) await loadRealname();
    if (!S.realname || S.realname.state !== 'VERIFIED') {
      S.orderError = '未完成实名认证（forbidden）：请先在「我的」完成实名认证';
      openRealName();
      renderAll();
      return;
    }
  }
  try {
    const data = await api('/passenger/orders', {
      method: 'POST',
      accountId: S.accountId,
      token: S.token,
      body: {
        accountId: S.accountId,
        estimateNo: S.estimate.estimate_no,
        carType: S.carType,
        startName: start.name, startAddr: start.addr,
        startLng: start.lng, startLat: start.lat,
        endName: end.name, endAddr: end.addr,
        endLng: end.lng, endLat: end.lat,
        idempotencyKey: randomKey('pax'),
      },
    });
    S.orderError = '';
    // 网关把订单包在 order 字段里（OrderResponse.order）。
    const payload = data.order || data;
    const order = {
      no: payload.order_no || payload.order_id || payload.id || '-',
      orderId: payload.order_id || payload.id || '',
      state: payload.state || payload.status || 'PENDING',
      typeName: carName(S.carType),
      payFee: payload.total_fee != null ? payload.total_fee : S.estimate.pay_fee,
      startId: S.startId, endId: S.endId,
      driverName: payload.driver_name || '',
      driverPlate: payload.driver_plate || '',
      startName: start.name, endName: end.name, startAddr: start.addr, endAddr: end.addr,
      createdAtText: new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }),
    };
    S.orders.unshift(order);
    S.currentOrder = order;
    toast(`叫车成功：${order.no}`);
    S.estimate = null;
    startFlowPoll();
  } catch (error) {
    // 同账号已有进行中订单是最常见失败：自动恢复行程视图而不是甩报错。
    if (/conflict/i.test(error.message || '')) {
      const restored = await restoreCurrentOrder();
      S.orderError = restored ? '' : '您已有进行中的行程，请先在「订单」页查看或等待行程结束';
      if (restored) {
        // 未支付订单同样占用行程：必须提示先付清，而不是只说"有进行中行程"。
        const unpaid = S.currentOrder && S.currentOrder.state === 'COMPLETED_UNPAID';
        toast(unpaid ? '您有未支付订单，请先完成支付后再叫车' : '已为您恢复进行中的行程');
        if (unpaid) S.tab = 'orders';
        renderAll();
        return;
      }
    } else {
      S.orderError = error.message;
    }
    toast(`叫车未成功：${S.orderError}`);
  }
  renderAll();
}

/**
 * restoreCurrentOrder 拉取当前进行中订单并恢复行程视图与轮询。
 * 返回是否恢复成功；无进行中订单或已终态返回 false。
 */
async function restoreCurrentOrder() {
  if (!loggedIn()) return false;
  try {
    const data = await api(`/passenger/orders/current?accountId=${encodeURIComponent(S.accountId)}`, {
      accountId: S.accountId, token: S.token,
    });
    const payload = data && (data.order || data);
    const no = payload && (payload.orderNo || payload.order_no);
    const finalizeLocal = () => {
      // 订单已终结（支付完成/取消）：清掉行程卡并停轮询，行程卡不能一直挂着。
      if (S.currentOrder) {
        const row = S.orders.find((o) => o.no === S.currentOrder.no);
        if (row && !['COMPLETED', 'CANCELLED'].includes(row.state)) {
          row.state = S.currentOrder.state === 'COMPLETED_UNPAID' ? 'COMPLETED' : row.state;
        }
      }
      S.currentOrder = null;
      stopFlowPoll();
    };
    if (!payload || !no) { finalizeLocal(); return false; }
    // 待支付订单同样恢复行程卡，让乘客继续完成支付。
    if (['COMPLETED', 'CANCELLED', 'ENDED'].includes(payload.state)) { finalizeLocal(); return false; }
    if (S.currentOrder && S.currentOrder.no === no) {
      S.currentOrder.state = payload.state || S.currentOrder.state;
      startFlowPoll();
      return true;
    }
    S.currentOrder = {
      no,
      orderId: payload.orderId || payload.order_id || '',
      state: payload.state || 'PENDING',
      typeName: carName(payload.car_type),
      payFee: payload.total_fee != null ? payload.total_fee : 0,
      driverName: payload.driver_name || '',
      driverPlate: payload.driver_plate || '',
      startName: payload.start_name || '-', endName: payload.end_name || '-',
      startAddr: payload.start_addr || '', endAddr: payload.end_addr || '',
      createdAtText: payload.created_at
        ? new Date(payload.created_at * 1000).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) : '',
    };
    startFlowPoll();
    renderAll();
    return true;
  } catch { return false; }
}

function orderDetail(no) {
  const order = S.orders.find((o) => o.no === no);
  if (!order) return;
  S.sheet = { type: 'detail', order };
  renderAll();
}

async function logout() {
  try {
    await api('/passenger/auth/logout', {
      method: 'POST', accountId: S.accountId, token: S.token, body: {},
    });
  } catch { /* 退出失败不阻塞本地清理 */ }
  S.accountId = ''; S.token = ''; S.phone = '';
  S.currentOrder = null; S.flowNotified = false; S.fullpage = null;
  stopFlowPoll();
  toast('已退出登录');
  renderAll();
}

// ---------------------------------------------------------------- 启动

window.paxOpenPoi = openPoi;
window.paxCloseSheet = closeSheet;
window.paxCloseService = closeService;
window.paxSearchPoi = searchPoi;
window.paxChoosePoi = choosePoi;
window.paxSetType = setType;
window.paxCallCar = callCar;
window.paxCancelRide = cancelRide;
// 联调临时调试把手：验证完成后移除。
window.paxSendCode = sendCode;
window.paxDoLogin = doLogin;
window.paxOrderDetail = orderDetail;
window.paxLogout = logout;
window.paxOpenRealName = openRealName;
window.paxSubmitRealName = submitRealName;
window.paxOpenClaim = openClaim;
window.paxClaimCoupon = claimCoupon;
window.paxSelectCoupon = selectCoupon;
window.paxPickCoupon = pickCoupon;
window.paxOpenService = openService;
window.paxToggleFaq = toggleFaq;

$nav.addEventListener('click', (event) => {
  const tab = event.target.dataset && event.target.dataset.tab;
  if (tab) {
    S.tab = tab;
    if (tab === 'orders') loadOrderList();
    renderAll();
  }
});

// 手机状态栏时钟。
function tickClock() {
  const el = document.getElementById('paxClock');
  if (el) el.textContent = new Date().toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' });
}
tickClock();
setInterval(tickClock, 30000);

renderAll();
