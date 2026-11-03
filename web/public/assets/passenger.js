// 乘客端页面：登录 → 选起终点 → 估价 → 选车型 → 呼叫。
// 视觉与交互流对齐 Figma 原型（花小猪打车平台 · 三端联调交互原型），
// 数据全部来自真实网关接口；订单列表为会话内状态（网关暂无订单查询接口）。
import { api, fy, esc, toast, randomKey, stateName } from './api.js';

// 全局错误上报：任何未捕获异常/Promise 拒都用 toast 冒泡到界面，
// 避免「页面看着正常但点什么都不响应」时用户无从得知原因；
// 详细堆栈同时落在 console 与 window.__paxLastError，便于 F12 排查。
// 限流：同类错误 6 秒内只提示一次，防止轮询场景刷屏。
let lastErrToastAt = 0;
let lastErrToastText = '';
function surfaceError(kind, detail) {
  const text = String(detail);
  console.error(`[pax] ${kind}:`, detail);
  window.__paxLastError = `${kind}: ${text}`;
  const now = Date.now();
  if (text === lastErrToastText && now - lastErrToastAt < 6000) return;
  lastErrToastAt = now;
  lastErrToastText = text;
  try { toast(`${kind}：${text.slice(0, 120)}`); } catch (e) { void e; }
}
window.addEventListener('error', (e) => { surfaceError('脚本错误', (e.error && e.error.message) || e.message); });
window.addEventListener('unhandledrejection', (e) => {
  surfaceError('异步错误', (e.reason && (e.reason.message || e.reason)) || 'unknown');
});
window.paxLastError = () => window.__paxLastError || '(none)';

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
  // 个人资料（含注册引导完成状态）与引导表单草稿。
  profile: null,
  profileDraft: { nickname: '', avatar: '', emergencyName: '', emergencyPhone: '' },
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
  // 渲染任何一步抛错都不能把页面打死：历史上弹层会停在 display:flex 的空壳上
  // 盖住整屏，表现为「页面看着正常但点什么都无反应」，下次刷新又时好时坏。
  // 这里兜底记录错误、给出可见提示，保证导航与后续渲染始终可用。
  try {
    renderAllInner();
  } catch (error) {
    console.error('[pax] renderAll failed:', error);
    window.__paxLastError = String((error && error.stack) || error);
    try { $overlay.style.display = 'none'; $overlay.innerHTML = ''; } catch (e) { void e; }
    try {
      $screen.innerHTML =
        '<div class="card" style="margin:40px 16px;text-align:center;color:#909399;">页面渲染出错，请刷新重试' +
        '<div class="note" style="margin-top:8px;">' + esc(String(error && error.message || error)) + '</div></div>';
    } catch (e) { void e; }
  }
}

function renderAllInner() {
  // 全屏页（客服中心）覆盖主屏。
  if (S.fullpage === 'service') {
    detachLiveMap();
    $screen.innerHTML = serviceFullView();
    $nav.innerHTML = '';
    renderOverlay();
    return;
  }
  // 关键：innerHTML 整体重写会销毁 Leaflet 容器，导致每次渲染都重建地图实例、
  // 瓦片重新下载（地图反复闪灰），并让异步回调（定位/瓦片/司机位置）打在死
  // 实例上抛错。这里把活着的容器先摘下，渲染完再插回原位，瓦片与实例都保留。
  const keptMap = detachLiveMap();
  if (S.tab === 'orders') $screen.innerHTML = paxOrdersView();
  else if (S.tab === 'me') $screen.innerHTML = paxMeView();
  else if (activeOrder()) $screen.innerHTML = paxFlowView(activeOrder());
  else $screen.innerHTML = paxHomeView();
  attachLiveMap(keptMap);

  $nav.innerHTML =
    `<div class="${S.tab === 'home' ? 'on' : ''}" data-tab="home">首页</div>` +
    `<div class="${S.tab === 'orders' ? 'on' : ''}" data-tab="orders">订单</div>` +
    `<div class="${S.tab === 'me' ? 'on' : ''}" data-tab="me">我的</div>`;
  renderOverlay();
  // 首页渲染完成后初始化真实地图（容器必须已在 DOM 中）。
  // 无行程时容器是 paxMap（选点地图）；行程中容器是 tripMap（行程地图）。
  if (S.tab === 'home') renderMap();
}

// parkedMapEl 暂存被摘下的地图容器：视图临时没有同名占位符（如行程页、
// 订单页）时先停在这里，等视图切回来再插回，实例与瓦片始终不丢。
var parkedMapEl = null;

// resetMapAnim 中止地图上的异步动画（飞行、平移、缩放过渡）并复位缩放动画状态。
//
// 容器被移出文档期间 CSS transition 不会触发 transitionend，Leaflet 收不到收尾
// 回调会一直停在 _animatingZoom=true，之后 setView 被误判为「动画进行中」直接
// 短路，动画状态与真实视图脱节。
function resetMapAnim(map) {
  if (!map) return;
  try { map.stop(); } catch (e) { void e; }
  if (map._animatingZoom) {
    map._animatingZoom = false;
    if (map._mapPane) map._mapPane.classList.remove('leaflet-zoom-anim');
  }
}

// detachLiveMap 摘下当前活着的 Leaflet 容器（连同瓦片、标记、实例）并返回。
// 仅对已初始化过地图的元素生效，避免把模板里的空占位符误当成可复用容器。
function detachLiveMap() {
  const el = $screen.querySelector('#paxMap, #tripMap');
  if (el && el.__paxMap) {
    // 先停动画再摘：容器离文档后尺寸为 0，飞行帧会按 Math.max(w,h)=0 做除零
    // 算出 NaN 经纬度，后续所有 setView/flyTo 都会抛 Invalid LatLng。
    resetMapAnim(el.__paxMap);
    if (el.parentNode) el.parentNode.removeChild(el);
    parkedMapEl = el;
    return el;
  }
  return parkedMapEl && parkedMapEl.__paxMap ? parkedMapEl : null;
}

// attachLiveMap 把先前摘下的容器插回新渲染出的占位符位置。
// 同名占位符直接替换；若视图换成了另一种地图容器（选点地图 paxMap ↔ 行程地图
// tripMap），也复用同一实例，只把 id 与内联样式迁过去，避免重建地图、瓦片重载。
// 当前视图完全没有地图容器时保持游离，等下一次渲染再插回。
function attachLiveMap(el) {
  if (!el) return;
  const sameSlot = $screen.querySelector('#' + el.id);
  let inserted = false;
  if (sameSlot && sameSlot !== el && sameSlot.parentNode) {
    sameSlot.parentNode.replaceChild(el, sameSlot);
    inserted = true;
  } else {
    const otherSlot = $screen.querySelector('#paxMap, #tripMap');
    if (otherSlot && otherSlot !== el && otherSlot.parentNode) {
      el.id = otherSlot.id;
      el.setAttribute('style', otherSlot.getAttribute('style') || '');
      otherSlot.parentNode.replaceChild(el, otherSlot);
      inserted = true;
    }
  }
  if (!inserted) return;
  parkedMapEl = null;
  // 插回文档后立刻按真实尺寸重算：容器离文档期间 getSize() 会被算成 0x0 并缓存，
  // 不刷新的话 flyTo/fitBounds 全按 0 尺寸算，直接产生 NaN 坐标。
  const map = el.__paxMap;
  if (map) {
    resetMapAnim(map);
    try { map.invalidateSize({ pan: false }); } catch (e) { void e; }
  }
}

function renderOverlay() {
  if (!S.sheet) { $overlay.style.display = 'none'; $overlay.innerHTML = ''; return; }
  $overlay.style.display = 'flex';
  let body = '';
  // 某类弹层在异常数据下抛错时，display:flex 的空壳会盖住整屏拦截所有点击，
  // 表现为整页「点什么都无反应」；这里兜底关闭弹层并回显错误。
  try {
    if (S.sheet.type === 'poi') body = poiSheet(S.sheet.kind);
    else if (S.sheet.type === 'login') body = loginSheet();
    else if (S.sheet.type === 'register') body = registerSheet();
    else if (S.sheet.type === 'detail') body = orderDetailSheet(S.sheet.order);
    else if (S.sheet.type === 'realname') body = realNameSheet();
    else if (S.sheet.type === 'claim') body = claimCouponSheet();
    else if (S.sheet.type === 'pay') body = paySheet();
    else if (S.sheet.type === 'review') body = reviewSheet();
  } catch (error) {
    console.error('[pax] renderOverlay failed:', error);
    window.__paxLastError = String((error && error.stack) || error);
    S.sheet = null;
    $overlay.style.display = 'none';
    $overlay.innerHTML = '';
    toast('弹层打开失败：' + String(error && error.message || error));
    return;
  }
  $overlay.innerHTML = `<div class="sheet">${body}</div>`;
}

/**
 * mapMock 渲染地图容器：真实全国地图由 renderMap() 用 Leaflet 初始化。
 */

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
      // 同步更改下车点裁决状态（司机同意后新终点立即生效）。
      const dcNow = payload.dest_change || payload.destChange;
      if (dcNow) S.currentOrder.destChange = { state: dcNow.state, newEndName: dcNow.new_end_name || dcNow.newEndName || '' };
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
      destChange: o.dest_change ? { state: o.dest_change.state, newEndName: o.dest_change.new_end_name || '' } : null,
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

  // 顶部常驻登录/注册入口：未登录时不用等触发叫车才看到登录入口。
  h += '<div style="background:#ffc300;padding:10px 14px 4px;display:flex;justify-content:space-between;align-items:center;">' +
    '<div><b style="font-size:15px;">花小猪打车</b>' +
    `<div style="font-size:10px;color:#6b5900;">${S.phone ? esc(S.phone) + ' · 已登录' : '未登录 · 叫车前需登录'}</div></div>` +
    (S.phone ? ''
      : '<button class="btn blue" style="flex:none;padding:6px 14px;" onclick="paxOpenLogin()">登录 / 注册</button>') +
    '</div>';

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
  // 行程地图：实时显示乘客上车点与司机当前位置（接单后每 5 秒刷新）。
  h += '<div class="hubmap" id="tripMap" style="height:220px;"></div>';
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
  // 行程中可发起更改下车点：提交后待司机裁决，拒绝则维持原终点。
  const liveStates = ['ACCEPTED', 'ARRIVED', 'IN_SERVICE'];
  if (liveStates.includes(order.state)) {
    const dc = order.destChange;
    if (dc && dc.state === 'pending') {
      h += '<div class="card" style="background:#f0f9eb;border:1px solid #c2e7b0;">' +
        '<b style="color:#67c23a;">更改下车点请求已提交</b>' +
        `<div class="note">新终点：${esc(dc.newEndName || '-')} · 等待司机确认中</div></div>`;
    } else {
      if (dc && dc.state === 'rejected') {
        h += '<div class="note" style="color:#e6a23c;">司机未同意上次的下车点更改，可重新选择</div>';
      }
      h += '<button class="btn" style="width:100%;margin-top:10px;background:#fff;color:#2f6fed;border:1px solid #b3d8ff;" onclick="paxOpenDestChange()">更改下车点</button>';
    }
  }
  // 行程结束即待支付：进入收银台选微信/支付宝完成支付。
  if (order.state === 'COMPLETED_UNPAID') {
    h += `<div class="card" style="text-align:center;"><h4>行程结束 · 待支付</h4>` +
      `<div style="font-size:26px;color:#303133;margin:6px 0;">¥${fy(order.payFee)}</div>` +
      `<div class="note">支付完成后订单才会计入司机收入</div>` +
      `<button class="btn yellow" style="width:100%;margin-top:8px;" onclick="paxPaySheet('${esc(order.no)}')">去支付 ¥${fy(order.payFee)}</button></div>`;
  }
  return h;
}

/** flowChipState 把乘客域状态映射到样式芯片（复用 WAITING 系配色）。 */
function flowChipState(state) {
  return { PENDING: 'WAITING', IN_SERVICE: 'IN_TRIP', COMPLETED_UNPAID: 'ENDED', COMPLETED: 'ENDED' }[state] || state;
}

/**
 * mapMock 渲染首页地图容器：真实全国地图由 renderMap() 用 Leaflet 初始化。
 * 起终点标记、路线与定位都在地图实例上绘制。
 */
function mapMock(start, end) {
  void start; void end;
  return '<div class="hubmap" id="paxMap" style="min-height:220px;"></div>';
}

function paxOrdersView() {
  // 已评价订单集合：来自本地留痕（每单仅可评价一次）。
  let reviewedSet = new Set();
  try { reviewedSet = new Set(JSON.parse(localStorage.getItem('huazhu.reviewed') || '[]')); } catch (e) { void e; }
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
      (unpaid ? `<button class="btn yellow" style="width:100%;margin-top:8px;" onclick="event.stopPropagation();paxPaySheet('${esc(o.no)}')">去支付 ¥${fy(o.payFee)}</button>` : '') +
      (o.state === 'COMPLETED'
        ? '<div class="btnrow" style="margin-top:8px;">' +
          (reviewedSet.has(o.no)
            ? '<span class="note" style="display:inline;">已评价 · 感谢您的反馈</span>'
            : `<button class="btn blue" style="flex:1;" onclick="event.stopPropagation();paxOpenReview('${esc(o.no)}')">评价司机</button>` +
              `<button class="btn ghost" style="flex:1;" onclick="event.stopPropagation();paxOpenReview('${esc(o.no)}');setTimeout(paxToggleReviewComplaint,0)">投诉</button>`) +
          '</div>'
        : '') +
      `</div>`;
  }
  return h;
}

function paxMeView() {
  const profile = S.profile || {};
  const displayName = profile.nickname || '乘客';
  const avatar = profile.avatar || '乘';
  let h = '<div style="background:#ffc300;padding:14px;"><div class="drvcard">' +
    `<div class="avatar" style="background:#fff;">${esc(avatar)}</div><div><b>${esc(displayName)}</b>` +
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
    `<div><span>我的优惠券</span><span>${S.coupons.length} 张可用</span></div></div>` +
    (loggedIn()
      ? '<div class="btnrow" style="margin-top:8px;"><button class="btn ghost" onclick="paxEditProfile()">编辑个人资料</button></div>'
      : '') + '</div>';
  // 未登录时「我的」页给出登录/注册入口，避免用户卡在空页面。
  if (!loggedIn()) {
    h += '<div class="card" style="text-align:center;padding:20px 0;">' +
      '<div class="note" style="margin-bottom:10px;">登录后可使用叫车、实名、优惠券等功能</div>' +
      '<div class="btnrow">' +
      '<button class="btn blue" onclick="paxOpenLogin()">登录</button>' +
      '<button class="btn ghost" onclick="paxOpenRegisterEntry()">注册</button></div></div>';
  }
  // 紧急联系人：仅本人可见，注册引导补全后在这里回显脱敏号码。
  if (loggedIn()) {
    h += '<div class="card"><h4>紧急联系人</h4><div class="feelist">' +
      `<div><span>姓名</span><span>${profile.emergency_name ? esc(profile.emergency_name) : '未填写'}</span></div>` +
      `<div><span>电话</span><span>${profile.emergency_phone_mask ? esc(profile.emergency_phone_mask) : '未填写'}</span></div></div>` +
      (profile.completed ? '' : '<div class="note" style="color:#e6a23c;">资料未填完，补全后可在行程异常时联系紧急联系人。</div>') +
      '</div>';
  }
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
  const title = kind === 'start' ? '选择上车点' : (kind === 'dest' ? '更改下车点' : '选择目的地');
  let h = `<h3>${title} <button class="x" onclick="paxCloseSheet()">✕</button></h3>`;
  h += '<div style="display:flex;gap:6px;margin-bottom:8px;">' +
    `<input id="poiKw" placeholder="搜索全国地点（如：北京西站）" value="${esc(S.sheet.kw || '')}" style="flex:1" onkeydown="if(event.key==='Enter')paxSearchPoi()">` +
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
  // 注册入口：验证码登录即自动建号，这里引导未注册用户补齐昵称与紧急联系人。
  h += '<div style="display:flex;justify-content:space-between;align-items:center;margin-top:10px;font-size:11px;color:#909399;">' +
    '<span>还没有账号？用手机号登录即自动注册</span>' +
    '<button class="btn blue" style="flex:none;padding:5px 12px;font-size:11px;" onclick="paxOpenRegister()">注册</button></div>';
  return h;
}

// ---------------------------------------------------------------- 注册引导

// AVATAR_CHOICES 头像候选：预置 emoji，避免图片上传与审核成本。
const AVATAR_CHOICES = ['🚕', '🐷', '🐰', '🐼', '🦊', '🐯', '🦁', '🐨'];

// paxOpenRegister 从未登录状态直接打开注册引导：先走验证码登录，登录成功后自动填资料。
function paxOpenRegister() {
  S.sheet = { type: 'login', phone: S.sheet ? S.sheet.phone : '', error: '', registerMode: true };
  renderAll();
}

// registerSheet 注册引导表单：昵称、头像与紧急联系人。
function registerSheet() {
  const draft = S.profileDraft;
  let h = '<h3>完善资料 <button class="x" onclick="paxSkipRegister()">跳过</button></h3>';
  h += '<div class="note" style="margin-bottom:8px;">首次登录请补充昵称与紧急联系人，紧急联系人仅用于行程安全保障。</div>';
  h += `<input id="rgNickname" placeholder="昵称（如：小王）" value="${esc(draft.nickname)}" oninput="paxRegInput()" style="width:100%;box-sizing:border-box;margin-bottom:8px;">`;
  h += '<div style="font-size:11px;color:#606266;margin-bottom:4px;">选择头像</div>';
  h += '<div style="display:flex;gap:6px;flex-wrap:wrap;margin-bottom:8px;">' +
    AVATAR_CHOICES.map((emoji) => `<button class="btn ${draft.avatar === emoji ? 'blue' : 'ghost'}" ` +
      `style="flex:none;padding:6px 10px;font-size:16px;" onclick="paxPickAvatar('${emoji}')">${emoji}</button>`).join('') +
    '</div>';
  h += '<div style="display:flex;gap:6px;margin-bottom:8px;">' +
    `<input id="rgEmergencyName" placeholder="紧急联系人姓名" value="${esc(draft.emergencyName)}" oninput="paxRegInput()" style="flex:1">` +
    `<input id="rgEmergencyPhone" type="tel" maxlength="11" placeholder="紧急联系电话" value="${esc(draft.emergencyPhone)}" oninput="paxRegInput()" style="flex:1"></div>`;
  h += '<button class="btn dark" style="width:100%;" onclick="paxSubmitRegister()">完成注册</button>';
  if (S.sheet.error) h += `<div class="note" style="color:#f56c6c;margin-top:6px;">${esc(S.sheet.error)}</div>`;
  return h;
}

// paxRegInput 实时把输入同步到草稿：弹层会因选头像等操作重绘，
// 不实时同步就会把用户已填的内容冲掉。
function paxRegInput() {
  const draft = S.profileDraft;
  const nickname = document.getElementById('rgNickname');
  const emergencyName = document.getElementById('rgEmergencyName');
  const emergencyPhone = document.getElementById('rgEmergencyPhone');
  if (nickname) draft.nickname = nickname.value;
  if (emergencyName) draft.emergencyName = emergencyName.value;
  if (emergencyPhone) draft.emergencyPhone = emergencyPhone.value;
}

function paxPickAvatar(emoji) {
  // 重绘前先把已填内容收进草稿，避免选头像时冲掉输入。
  paxRegInput();
  S.profileDraft.avatar = emoji;
  renderAll();
}

// paxSkipRegister 跳过引导：资料可稍后在「我的 → 个人资料」补齐。
function paxSkipRegister() {
  S.sheet = null;
  renderAll();
}

async function paxSubmitRegister() {
  const draft = S.profileDraft;
  draft.nickname = (document.getElementById('rgNickname') || {}).value || '';
  draft.emergencyName = (document.getElementById('rgEmergencyName') || {}).value || '';
  draft.emergencyPhone = (document.getElementById('rgEmergencyPhone') || {}).value || '';
  S.sheet.error = '';
  if (!draft.nickname.trim()) { S.sheet.error = '请填写昵称'; renderAll(); return; }
  if (!draft.emergencyName.trim() || !draft.emergencyPhone.trim()) {
    S.sheet.error = '紧急联系人姓名与电话需同时填写';
    renderAll();
    return;
  }
  try {
    const data = await api('/passenger/profile', {
      method: 'POST',
      accountId: S.accountId,
      token: S.token,
      body: {
        nickname: draft.nickname.trim(),
        avatar: draft.avatar,
        emergencyName: draft.emergencyName.trim(),
        emergencyPhone: draft.emergencyPhone.trim(),
      },
    });
    S.profile = data;
    S.sheet = null;
    toast('资料已保存，注册完成');
    renderAll();
  } catch (error) {
    S.sheet.error = error.message;
    renderAll();
  }
}

// loadProfile 拉取个人资料：判断是否已填完注册引导。
async function loadProfile() {
  if (!loggedIn()) return;
  try {
    S.profile = await api(`/passenger/profile?accountId=${encodeURIComponent(S.accountId)}`, {
      accountId: S.accountId, token: S.token,
    });
  } catch (error) {
    S.profile = null;
  }
}

// paxEditProfile 从「我的」页再次编辑资料。
function paxEditProfile() {
  const profile = S.profile || {};
  S.profileDraft = {
    nickname: profile.nickname || '',
    avatar: profile.avatar || AVATAR_CHOICES[0],
    emergencyName: profile.emergency_name || '',
    emergencyPhone: '',
  };
  S.sheet = { type: 'register', error: '' };
  renderAll();
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
    // 全国地点搜索：走后端代理（服务端调 Nominatim，覆盖省市区县/乡镇/村庄/道路）。
    const query = new URLSearchParams({ keyword: kw, limit: '8' });
    if (S.accountId) query.set('accountId', S.accountId);
    // 带上当前定位，让后端按就近偏好排序（无定位时不传，避免虚假距离）。
    if (paxLastLoc) {
      query.set('lng', paxLastLoc.lng);
      query.set('lat', paxLastLoc.lat);
      query.set('hasCoord', 'true');
    }
    const data = await api(`/passenger/poi/search?${query.toString()}`, { accountId: S.accountId, token: S.token });
    S.sheet.results = (data.list || []).map((p, i) => ({
      id: `s_${i}_${p.lng}_${p.lat}`,
      name: p.name || p.title || '未命名地点',
      addr: p.addr || p.address || p.city || '',
      lng: p.lng, lat: p.lat,
    })).filter((p) => Number.isFinite(p.lng) && Number.isFinite(p.lat));
    if (!S.sheet.results.length) toast('没有匹配的地点');
  } catch (e) {
    // 后端检索不可达（断网/超时）：回退到内置常用地点，保证可继续下单演示。
    S.sheet.results = POIS.filter((p) => p.name.includes(kw) || (p.addr || '').includes(kw));
    if (!S.sheet.results.length) S.sheet.results = POIS;
    toast('在线搜索暂不可用，已显示常用地点');
  }
  renderAll();
}

async function choosePoi(kind, id) {
  const pool = (S.sheet && S.sheet.results) || POIS;
  const p = pool.find((x) => x.id === id) || POIS.find((x) => x.id === id);
  if (!p) return;
  if (kind === 'dest') { await submitDestChange(p); return; }
  if (kind === 'start') S.start = p;
  else S.end = p;
  S.sheet = null;
  savePaxSession(); // 记下起终点，刷新页面后行程地图仍可还原标记。
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

// paxOpenRegisterEntry 未登录状态下从首页/我的页进入注册引导。
function paxOpenRegisterEntry() {
  S.sheet = { type: 'login', registerMode: true };
  renderAll();
}

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
    const registerMode = Boolean(S.sheet && S.sheet.registerMode);
    S.accountId = data.account_id;
    S.token = data.access_token;
    S.phone = phone.trim();
    // 登录态写进 sessionStorage：刷新页面后 token 不会丢，
    // 否则页面显示已登录但所有带鉴权的请求都会 401（估价会静默失败）。
    // 同时存起终点坐标，刷新后行程地图能还原上车点/目的地标记。
    savePaxSession();
    await loadHome();
    // 登录后补齐实名、券包、客服与个人资料，供首页用车与「我的」页使用。
    await Promise.all([loadRealname(), loadCoupons(), loadService(), loadProfile()]);
    // 新用户或从注册入口进来：直接弹出资料引导，填完即可进入登录态。
    const needGuide = registerMode || data.is_new || !(S.profile && S.profile.completed);
    if (needGuide) {
      S.profileDraft = {
        nickname: (S.profile && S.profile.nickname) || '',
        avatar: (S.profile && S.profile.avatar) || AVATAR_CHOICES[0],
        emergencyName: (S.profile && S.profile.emergency_name) || '',
        emergencyPhone: '',
      };
      S.sheet = { type: 'register', error: '' };
      renderAll();
    } else {
      S.sheet = null;
      toast('登录成功');
    }
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
    S.currentOrder = buildCurrentOrder(payload);
    startFlowPoll();
    renderAll();
    return true;
  } catch { return false; }
}

// buildCurrentOrder 把后端订单响应映射成前端行程对象（行程卡与行程地图共用）。
function buildCurrentOrder(payload) {
  const no = payload.orderNo || payload.order_no || payload.order_id || payload.id || '-';
  return {
    no,
    orderId: payload.orderId || payload.order_id || payload.id || '',
    state: payload.state || 'PENDING',
    typeName: carName(payload.car_type),
    payFee: payload.total_fee != null ? payload.total_fee : 0,
    driverName: payload.driver_name || '',
    driverPlate: payload.driver_plate || '',
    startName: payload.start_name || '-', endName: payload.end_name || '-',
    startAddr: payload.start_addr || '', endAddr: payload.end_addr || '',
    // 订单自带起终点坐标：刷新页面后 S.start/S.end 会丢，行程地图靠这四个值还原标记。
    startLat: payload.start_lat, startLng: payload.start_lng,
    endLat: payload.end_lat, endLng: payload.end_lng,
    destChange: (() => { const d = payload.dest_change || payload.destChange; return d ? { state: d.state, newEndName: d.new_end_name || d.newEndName || '' } : null; })(),
    createdAtText: payload.created_at
      ? new Date(payload.created_at * 1000).toLocaleTimeString('zh-CN', { hour: '2-digit', minute: '2-digit' }) : '',
  };
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
  // 清掉本地会话，避免刷新后又"自动登录"。
  try { sessionStorage.removeItem('huazhu.paxSession'); } catch (e) { void e; }
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
window.paxOpenLogin = openLogin;
window.paxOpenRegisterEntry = paxOpenRegisterEntry;
window.paxOpenRegister = paxOpenRegister;
window.paxPickAvatar = paxPickAvatar;
window.paxRegInput = paxRegInput;
window.paxSkipRegister = paxSkipRegister;
window.paxSubmitRegister = paxSubmitRegister;
window.paxEditProfile = paxEditProfile;
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

// savePaxSession 把登录态与起终点坐标写入 sessionStorage，刷新后自动还原。
function savePaxSession() {
  try {
    sessionStorage.setItem('huazhu.paxSession', JSON.stringify({
      accountId: S.accountId, token: S.token, phone: S.phone,
      start: S.start || null, end: S.end || null,
    }));
  } catch (e) { void e; }
}
window.savePaxSession = savePaxSession;

// 会话恢复：登录态存 sessionStorage（本次标签页有效），刷新后自动还原，
// 避免「显示已登录但估价/下单全部 401」。
(function restorePaxSession() {
  try {
    const raw = sessionStorage.getItem('huazhu.paxSession');
    if (!raw) return;
    const saved = JSON.parse(raw);
    if (saved && saved.accountId && saved.token) {
      S.accountId = saved.accountId;
      S.token = saved.token;
      S.phone = saved.phone || '';
      // 还原起终点坐标：行程地图刷新后能继续显示上车点/目的地标记。
      if (saved.start && Number.isFinite(saved.start.lat) && Number.isFinite(saved.start.lng)) S.start = saved.start;
      if (saved.end && Number.isFinite(saved.end.lat) && Number.isFinite(saved.end.lng)) S.end = saved.end;
      S.sessionRestored = true;
    }
  } catch (e) { void e; }
})();

// PAX_TILE_SOURCES 候选底图源，按序尝试。
// 部分浏览器（Edge 跟踪防护、系统代理）会拦截高德瓦片域名，导致地图只剩灰底；
// 首个源连续报错时自动切到下一个候选源重挂，保证底图可用。
// 必须在首次 renderAll() 之前赋值：初始渲染就会走到 addTileLayer 读它。
var PAX_TILE_SOURCES = [
  { url: 'https://webrd0{s}.is.autonavi.com/appmaptile?x={x}&y={y}&z={z}&lang=zh_cn&size=1&scale=1&style=8', opts: { subdomains: '1234', minZoom: 3, maxZoom: 18 } },
  { url: 'https://server.arcgisonline.com/ArcGIS/rest/services/World_Street_Map/MapServer/tile/{z}/{y}/{x}', opts: { minZoom: 3, maxZoom: 18 } },
  { url: 'https://tile.openstreetmap.org/{z}/{x}/{y}.png', opts: { minZoom: 3, maxZoom: 18 } },
];
// paxTileSrcIdx 当前生效的底图源下标：一旦切换，后续新建地图直接用可用的源。
var paxTileSrcIdx = 0;

renderAll();

// 还原登录态后补齐首页与行程数据。
if (S.sessionRestored) {
  loadHome();
  loadOrderList();
  restoreCurrentOrder();
}

// ============ 真实地图 / 收银台 / 更改下车点 / 评价投诉 ============

// paxMapInstance 首页 Leaflet 地图实例（renderAll 重渲染时按容器复用）。
// 用 var 声明以获得模块级提升：初始 renderAll 发生在本块之前执行。
var paxMapInstance = null;

// dotIcon 用 div 圆点做标记图标，避免依赖图片资源。
function dotIcon(color) {
  return L.divIcon({
    className: '',
    html: '<span style="display:inline-block;width:14px;height:14px;border-radius:50%;' +
      `background:${color};border:3px solid #fff;box-shadow:0 1px 5px rgba(0,0,0,.4);"></span>`,
    iconSize: [14, 14], iconAnchor: [7, 7],
  });
}

// ============ 坐标系转换：WGS84 → GCJ02（火星坐标） ============
// 业务数据（POI 检索、GPS 定位、订单坐标）统一存 WGS84；
// 高德底图是 GCJ02，渲染前转换，标记才不会偏移几百米。

function mapTransformLat(x, y) {
  let r = -100 + 2 * x + 3 * y + 0.2 * y * y + 0.1 * x * y + 0.2 * Math.sqrt(Math.abs(x));
  r += (20 * Math.sin(6 * x * Math.PI) + 20 * Math.sin(2 * x * Math.PI)) * 2 / 3;
  r += (20 * Math.sin(y * Math.PI) + 40 * Math.sin(y / 3 * Math.PI)) * 2 / 3;
  r += (160 * Math.sin(y / 12 * Math.PI) + 320 * Math.sin(y * Math.PI / 30)) * 2 / 3;
  return r;
}

function mapTransformLng(x, y) {
  let r = 300 + x + 2 * y + 0.1 * x * x + 0.1 * x * y + 0.1 * Math.sqrt(Math.abs(x));
  r += (20 * Math.sin(6 * x * Math.PI) + 20 * Math.sin(2 * x * Math.PI)) * 2 / 3;
  r += (20 * Math.sin(x * Math.PI) + 40 * Math.sin(x / 3 * Math.PI)) * 2 / 3;
  r += (150 * Math.sin(x / 12 * Math.PI) + 300 * Math.sin(x / 30 * Math.PI)) * 2 / 3;
  return r;
}

// gcj 把 WGS84 经纬度转成 GCJ02 数组 [lat, lng]；境外坐标原样返回。
function gcj(lat, lng) {
  const a = 6378245.0, ee = 0.00669342162296594323;
  if (lng < 72.004 || lng > 137.8347 || lat < 0.8293 || lat > 55.8271) return [lat, lng];
  let dLat = mapTransformLat(lng - 105, lat - 35);
  let dLng = mapTransformLng(lng - 105, lat - 35);
  const radLat = lat / 180 * Math.PI;
  let magic = Math.sin(radLat); magic = 1 - ee * magic * magic;
  const sqrtMagic = Math.sqrt(magic);
  dLat = (dLat * 180) / ((a * (1 - ee)) / (magic * sqrtMagic) * Math.PI);
  dLng = (dLng * 180) / (a / sqrtMagic * Math.cos(radLat) * Math.PI);
  return [lat + dLat, lng + dLng];
}

// paxMarkGroup 起终点标记/路线图层：复用地图像例时先清空再重绘。
var paxMarkGroup = null;
// paxMapSig 上一次上图的点位签名：只有起终点真的变了才调整视野，
// 否则每次 renderAll 都会把用户手动拖到的位置拽回去。
var paxMapSig = '';

// PaxLocateCtl 地图右上角的「定位到我的位置」按钮，首次建图时惰性定义，
// 避免 leaflet 未加载时在模块顶层直接抛错把整个页面打挂。
var PaxLocateCtl = null;

// addTileLayer 按当前候选源挂瓦片层，判定源不可达后切换下一源。
// 两个触发条件：连续 3 块瓦片报错（快速失败）；或 5 秒宽限内一块都没加载成功
// （可见区域可能只有 1~2 块瓦片，光靠错误计数达不到阈值）。
// 所有候选源都失败时在地图上挂出可见提示，不再无声灰屏。
function addTileLayer(map) {
  const src = PAX_TILE_SOURCES[paxTileSrcIdx];
  const isLastSource = paxTileSrcIdx >= PAX_TILE_SOURCES.length - 1;
  const layer = L.tileLayer(src.url, src.opts);
  let errors = 0;
  let loaded = 0;
  let switched = false;
  const el = mapContainer(map);
  const trySwitch = () => {
    // 整体容错：切换发生在 5 秒后的定时器里，期间地图可能已被重渲染销毁；
    // Leaflet 的图层操作还可能走延迟队列，死实例上的报错不应打断页面。
    try {
      // 已切换过 / 有成功瓦片 / 地图已随旧渲染销毁：都不再切。
      if (switched || loaded > 0) return;
      const liveEl = mapContainer(map);
      if (!liveEl || !liveEl.isConnected) return;
      if (isLastSource) {
        // 最后一个源也失败：灰底已成定局，挂提示让用户知道是网络受限而非页面坏了。
        if (!liveEl.querySelector('.map-tile-hint')) {
          const tip = document.createElement('div');
          tip.className = 'map-tile-hint';
          tip.style.cssText = 'position:absolute;left:50%;top:10px;transform:translateX(-50%);z-index:600;' +
            'background:rgba(0,0,0,.45);color:#fff;font-size:11px;padding:2px 10px;border-radius:10px;pointer-events:none;';
          tip.textContent = '底图加载失败（网络受限），不影响选点与叫车';
          liveEl.appendChild(tip);
        }
        return;
      }
      switched = true;
      paxTileSrcIdx += 1;
      try { map.removeLayer(layer); } catch (e) { void e; }
      addTileLayer(map);
      toast('默认底图加载失败，已切换备用底图');
    } catch (e) { console.warn('[pax] tile switch skipped:', e && e.message); }
  };
  layer.on('tileerror', () => { errors += 1; if (errors >= 3) trySwitch(); });
  layer.on('tileload', () => { loaded += 1; });
  setTimeout(trySwitch, 5000);
  layer.addTo(map);
}

// mapContainer 安全取实例容器：实例被 remove 后 getContainer 可能抛错。
function mapContainer(map) {
  try { return map ? map.getContainer() : null; } catch (e) { void e; return null; }
}

// paxMapRef 取当前活着的图实例：优先模块变量，其次容器上缓存的实例。
//
// renderAll 触发很频繁，模块变量有可能在重渲染中被清空，而地图本身还活着；
// 只认模块变量的话，定位按钮会误报「当前浏览器不支持定位」、自动定位也静默失效。
function paxMapRef() {
  const el = document.getElementById('paxMap') || document.getElementById('tripMap');
  if (!el) return null;
  if (mapContainer(paxMapInstance) === el) return paxMapInstance;
  if (mapContainer(el.__paxMap) === el) return el.__paxMap;
  return null;
}

// renderMap 初始化/刷新乘客端 Leaflet 地图：高德底图，可拖动缩放看全国。
// 首页容器 paxMap（选点地图）；行程中容器 tripMap（行程地图：起终点+司机实时位置）。
//
// 按容器复用地图像例：每次 renderAll 都 remove + 重建既会闪屏，
// 也会在建图失败时把实例置空，导致地图发灰、定位按钮和交互全部失效。
function renderMap() {
  const el = document.getElementById('paxMap') || document.getElementById('tripMap');
  stopDriverPoll();
  // 没有容器（切到订单/我的/客服页时地图被暂存）时直接返回：不能把实例引用
  // 清空，否则切回首页时只能重建地图，白丢已加载的瓦片。
  if (!el || typeof L === 'undefined') return;
  let map = mapContainer(paxMapInstance) === el ? paxMapInstance : null;
  if (!map && mapContainer(el.__paxMap) === el) map = el.__paxMap;
  const fresh = !map;
  if (fresh) {
    if (paxMapInstance) { try { paxMapInstance.remove(); } catch (e) { void e; } }
    paxMapInstance = null;
    paxMarkGroup = null;
    // 容器可能残留上一次的 leaflet 初始化标记，不清掉 L.map 会抛
    // "Map container is already initialized"。
    if (el._leaflet_id) { delete el._leaflet_id; el.innerHTML = ''; }
    // 动画全部用 Leaflet 默认（缩放、瓦片淡入、标记缩放、拖动惯性）。此前为了
    // 压制 _leaflet_pos 才关闭，根因是每次 renderAll 都重建地图实例、动画回调
    // 落在死实例上；现在实例跨渲染复用（见 detachLiveMap/attachLiveMap），回调
    // 始终作用在同一个活实例上，动画可以安全恢复。
    map = L.map(el, { zoomControl: false, attributionControl: false });
    // 底图源见 PAX_TILE_SOURCES：默认高德栅格（国内可达），被拦时自动切备用。
    addTileLayer(map);
    paxMarkGroup = L.layerGroup().addTo(map);
    if (!PaxLocateCtl) {
      PaxLocateCtl = L.Control.extend({
        onAdd() {
          const btn = L.DomUtil.create('button', 'map-locate-btn');
          btn.innerHTML = '◎';
          btn.title = '定位到我的位置';
          btn.type = 'button';
          L.DomEvent.on(btn, 'click', (e) => { L.DomEvent.stop(e); locatePaxMap(true); });
          return btn;
        },
      });
    }
    map.addControl(new PaxLocateCtl({ position: 'topright' }));
    paxMapSig = '';
  }
  // 容器上留一份实例引用，模块变量丢了也能恢复。
  el.__paxMap = map;
  paxMapInstance = map;
  // 图层组必须存在且绑在当前实例上：复用地图像例、或上次建图在中途失败时，
  // paxMarkGroup 可能为 null 或仍挂在已销毁的旧实例上，直接 addTo 会抛
  // "Cannot read properties of null (reading 'addLayer')" 并把整页渲染打断。
  if (!paxMarkGroup || paxMarkGroup._map !== map) paxMarkGroup = L.layerGroup().addTo(map);
  paxMarkGroup.clearLayers();
  const trip = activeOrder();
  // 标记点（转 GCJ02 后再上图）：行程中优先用订单自带的起终点坐标
  // （刷新后 S.start/S.end 为空），空闲时用当前选点。
  const tripStart = trip && trip.startLat ? { lat: trip.startLat, lng: trip.startLng } : null;
  const tripEnd = trip && trip.endLat ? { lat: trip.endLat, lng: trip.endLng } : null;
  const startPoint = tripStart || S.start;
  const endPoint = tripEnd || S.end;
  // 坐标必须是有限值才上图：历史上定位回调把 undefined 写进过 S.start，
  // 一旦 gcj(undefined) 产出 NaN，L.marker 会抛 Invalid LatLng 并中断整个
  // renderAll，表现为地图发灰且页面点不动，因此在入口处过滤坏点。
  const pointOn = (p) => (p && Number.isFinite(p.lat) && Number.isFinite(p.lng) ? gcj(p.lat, p.lng) : null);
  const pts = [];
  const startPt = pointOn(startPoint);
  const endPt = pointOn(endPoint);
  if (startPt) pts.push(startPt);
  if (endPt) pts.push(endPt);
  pts.forEach((p, i) => {
    L.marker(p, { icon: dotIcon(i === 0 ? '#19be6b' : '#f56c6c') })
      .addTo(paxMarkGroup)
      .bindTooltip(i === 0 ? '上车点' : (trip ? '下车点' : '目的地'));
  });
  if (pts.length === 2) {
    L.polyline(pts, { color: '#2f7cf6', weight: 3, dashArray: '6,8' }).addTo(paxMarkGroup);
  }
  const sig = pts.map((p) => `${p[0].toFixed(5)},${p[1].toFixed(5)}`).join('|');
  if (fresh || sig !== paxMapSig) {
    // 尺寸为 0 时 fitBounds 会算出无效缩放（NaN），先按真实尺寸校正一次，
    // 仍为 0 就跳过视野设置，留到下一次渲染再定（标记不受影响）。
    let size = map.getSize();
    if (!size.x || !size.y) {
      try { map.invalidateSize({ pan: false }); } catch (e) { void e; }
      size = map.getSize();
    }
    if (size.x > 0 && size.y > 0) {
      if (!pts.length) {
        // 无选点：先给全国视野，随后自动定位飞到当前位置。
        map.setView([35.8617, 104.1954], 4);
      } else if (pts.length === 1) {
        map.setView(pts[0], 14);
      } else {
        map.fitBounds(L.latLngBounds(pts).pad(0.35));
      }
      paxMapSig = sig;
    }
  }
  // 新建地图时自动定位一次（静默，失败不打扰）；复用时不再重复请求定位。
  if (fresh) locatePaxMap(false);
  // 行程中（已接单）启动司机实时位置轮询，在地图上画司机车辆标记。
  // IN_TRIP/IN_SERVICE 都是行程进行中的状态，必须一并覆盖，否则司机标记不会出现。
  if (trip && ['ACCEPTED', 'ARRIVED', 'IN_TRIP', 'IN_SERVICE'].includes(trip.state)) startDriverPoll();
  // 容器刚插入 DOM 时尺寸可能还是 0，下一帧再让 leaflet 重算一次；尺寸没变时
  // 是空操作。必须同时校验实例与容器：容器被视图切换摘下时若仍重算，会把
  // getSize() 缓存成 0x0，插回后 flyTo 按 0 尺寸除零算出 NaN 坐标。
  setTimeout(() => {
    if (map !== paxMapInstance) return;
    const host = mapContainer(map);
    if (!host || !host.isConnected) return;
    try { map.invalidateSize(); } catch (e) { void e; }
  }, 80);
}

// drvMarker / drvPollTimer 行程地图上的司机标记与轮询句柄。
// 用 var 声明以获得模块级提升：初始 renderAll 发生在本块之前执行。
var drvMarker = null;
var drvPollTimer = null;

// startDriverPoll 每 5 秒拉取司机实时位置并更新地图标记。
function startDriverPoll() {
  stopDriverPoll();
  fetchDriverLocation();
  drvPollTimer = setInterval(fetchDriverLocation, 5000);
}

// stopDriverPoll 停止司机位置轮询并摘除标记。
function stopDriverPoll() {
  if (drvPollTimer) { clearInterval(drvPollTimer); drvPollTimer = null; }
  if (drvMarker) { try { drvMarker.remove(); } catch (e) { void e; } drvMarker = null; }
}

// fetchDriverLocation 拉取一次司机位置；失败静默（下一轮重试）。
async function fetchDriverLocation() {
  const trip = activeOrder();
  if (!trip || !trip.orderId) return;
  try {
    const d = await api(`/passenger/orders/driver-position?orderId=${encodeURIComponent(trip.orderId)}`,
      { accountId: S.accountId, token: S.token });
    // await 期间 renderAll 可能重建过地图：上图前重新解析活实例。
    const map = paxMapRef();
    const mapEl = mapContainer(map);
    if (!map || !mapEl || !mapEl.isConnected) return;
    const lng = d.lng != null ? d.lng : d.driver_lng;
    const lat = lat0(d);
    if (!Number.isFinite(lng) || !Number.isFinite(lat) || (lng === 0 && lat === 0)) return;
    if (drvMarker) drvMarker.remove();
    // 司机坐标是 WGS84，上图前转 GCJ02。
    drvMarker = L.marker(gcj(lat, lng), {
      icon: L.divIcon({
        className: '',
        html: '<span style="display:inline-flex;width:26px;height:26px;border-radius:8px;background:#2f6fed;' +
          'color:#fff;align-items:center;justify-content:center;font-size:15px;box-shadow:0 2px 6px rgba(0,0,0,.35);">🚗</span>',
        iconSize: [26, 26], iconAnchor: [13, 13],
      }),
    }).addTo(map)
      .bindTooltip(`司机 ${trip.driverName || ''} ${trip.driverPlate || ''}`.trim());
  } catch (e) { void e; }
}

// lat0 兼容驼峰/蛇形的纬度字段。
function lat0(d) {
  if (d.lat != null) return d.lat;
  if (d.driver_lat != null) return d.driver_lat;
  return NaN;
}

// paxLocateMarker 我的定位蓝点（重复定位时先移除旧点）。
var paxLocateMarker = null;
// paxLastLoc 最近一次成功定位的坐标，供地点搜索做就近偏好。
var paxLastLoc = null;

// locatePaxMap 浏览器地理定位：成功后飞到当前位置并画蓝点。
// manual=true 时失败会给 toast 提示，自动定位静默失败。
function locatePaxMap(manual) {
  const map = paxMapRef();
  if (!map || !navigator.geolocation) {
    if (manual) toast(!navigator.geolocation ? '当前浏览器不支持定位' : '地图未就绪，请稍后重试');
    return;
  }
  navigator.geolocation.getCurrentPosition((pos) => {
    // GPS 是 WGS84，上图前转 GCJ02。gcj 返回 [lat, lng] 数组，
    // 这里解构成对象再写 S.start / paxLastLoc：直接读数组的 .lat/.lng 会得到
    // undefined，下一次渲染就会用 NaN 建 marker 并抛异常。
    const [lat, lng] = gcj(pos.coords.latitude, pos.coords.longitude);
    if (!Number.isFinite(lat) || !Number.isFinite(lng)) return;
    // 记录最近一次定位坐标，供地点搜索做就近偏好。
    paxLastLoc = { lng, lat };
    // 默认上车点取当前定位：尚未手动选点且不在行程中时自动填充。
    if (!S.start && !S.currentOrder) {
      S.start = { id: 'gps', name: '当前位置', addr: '我的位置', lng, lat };
      savePaxSession();
    }
    renderAll();
    // 数据更新不依赖地图实例，先于上图完成；地图操作可能撞上 Leaflet 的
    // 延迟队列（addTo/setView 在地图未就绪时走 whenReady/动画帧，若期间
    // renderAll 重建地图，回调就在死实例上跑并抛 _leaflet_pos/appendChild），
    // 因此整体 try/catch 降级：定位失败只影响蓝点，不影响数据与交互。
    try {
      // 定位是异步的：入口拿到的地图实例可能已被某次 renderAll 重建或销毁，
      // 必须重新解析当前活着的实例，且容器必须仍挂在 DOM 上。
      const live = paxMapRef();
      const liveEl = mapContainer(live);
      if (live && liveEl && liveEl.isConnected) {
        const latlng = L.latLng(lat, lng);
        if (paxLocateMarker) paxLocateMarker.remove();
        paxLocateMarker = L.marker(latlng, {
          icon: L.divIcon({
            className: '',
            html: '<span style="position:relative;display:inline-block;width:22px;height:22px;">' +
              '<span style="position:absolute;inset:0;border-radius:50%;background:#2f7cf6;opacity:.25;"></span>' +
              '<span style="position:absolute;inset:6px;border-radius:50%;background:#2f7cf6;border:2.5px solid #fff;box-shadow:0 1px 6px rgba(0,0,0,.4);"></span></span>',
            iconSize: [22, 22], iconAnchor: [11, 11],
          }),
        }).addTo(live).bindTooltip('我的位置');
        // 定位飞行动画。尺寸为 0 时 flyTo 内部会除零算出 NaN 坐标（容器刚插回、
        // 还没完成布局的瞬间），此时退化为无动画定位。
        const size = live.getSize();
        if (size.x > 0 && size.y > 0) live.flyTo(latlng, 15, { duration: 0.6 });
        else live.setView(latlng, 15, { animate: false });
      }
    } catch (e) { console.warn('[pax] locate map update skipped:', e && e.message); }
    if (manual) toast('已定位到当前位置');
  }, (err) => {
    if (manual) {
      if (err.code === err.PERMISSION_DENIED) toast('定位被拒绝：请在浏览器地址栏允许位置权限');
      else toast('定位失败，请稍后重试');
    }
  }, { enableHighAccuracy: true, timeout: 8000, maximumAge: 30000 });
}
// submitDestChange 乘客行程中请求更改下车点，由司机端裁决。
async function submitDestChange(poi) {
  const order = S.currentOrder;
  if (!order || !order.no) { toast('当前没有进行中的行程'); return; }
  try {
    const resp = await api('/passenger/orders/destination-change', {
      method: 'POST', accountId: S.accountId, token: S.token,
      body: {
        accountId: S.accountId, orderNo: order.no,
        newEndName: poi.name, newEndAddr: poi.addr || poi.name,
        newEndLng: poi.lng, newEndLat: poi.lat,
      },
    });
    order.destChange = {
      state: (resp && resp.state) || 'pending',
      newEndName: poi.name, newEndAddr: poi.addr || poi.name,
    };
    S.sheet = null;
    toast('已提交更改请求，等待司机确认');
  } catch (error) {
    toast(`提交失败：${error.message}`);
  }
  renderAll();
}

// paxOpenDestChange 行程中打开更改下车点选点面板。
function paxOpenDestChange() {
  if (!S.currentOrder) { toast('当前没有进行中的行程'); return; }
  S.sheet = { type: 'poi', kind: 'dest', kw: '', results: null };
  renderAll();
}

// pickRating / toggleReviewComplaint / toggleReviewTag 评价面板交互。
function pickRating(n) { if (S.sheet && S.sheet.type === 'review') { S.sheet.rating = n; renderAll(); } }
function toggleReviewComplaint() {
  if (S.sheet && S.sheet.type === 'review') { S.sheet.complaint = !S.sheet.complaint; renderAll(); }
}
function toggleReviewTag(tag) {
  const st = S.sheet;
  if (!st || st.type !== 'review') return;
  st.tags = st.tags || [];
  const i = st.tags.indexOf(tag);
  if (i >= 0) st.tags.splice(i, 1);
  else st.tags.push(tag);
  renderAll();
}
// submitReview 提交评价或投诉（每单仅一条，提交后不可改）。
async function submitReview() {
  const st = S.sheet;
  if (!st || st.type !== 'review') return;
  const comment = ((document.getElementById('rvComment') || {}).value || '').trim();
  const complaintDesc = ((document.getElementById('cmpDesc') || {}).value || '').trim();
  const complaintType = ((document.getElementById('cmpType') || {}).value || '').trim();
  if (st.complaint && (!complaintType || !complaintDesc)) {
    st.error = '投诉需选择类型并填写问题描述';
    renderAll();
    return;
  }
  if (!st.complaint && !st.rating) {
    st.error = '请先选择星级';
    renderAll();
    return;
  }
  try {
    await api('/passenger/orders/review', {
      method: 'POST', accountId: S.accountId, token: S.token,
      body: {
        accountId: S.accountId, orderNo: st.orderNo,
        rating: st.complaint ? st.rating || 0 : st.rating,
        tags: st.tags || [], comment,
        isComplaint: st.complaint, complaintType, complaintDesc,
      },
    });
    try {
      const reviewed = new Set(JSON.parse(localStorage.getItem('huazhu.reviewed') || '[]'));
      reviewed.add(st.orderNo);
      localStorage.setItem('huazhu.reviewed', JSON.stringify([...reviewed]));
    } catch (e) { void e; }
    S.sheet = null;
    toast(st.complaint ? '投诉已提交，客服会尽快跟进' : '感谢您的评价');
  } catch (error) {
    st.error = error.message;
  }
  renderAll();
}

// paxOpenReview 打开评价/投诉面板。
function paxOpenReview(orderNo) {
  S.sheet = { type: 'review', orderNo, rating: 5, tags: [], complaint: false, error: '' };
  renderAll();
}
// PAY_CHANNELS 收银台可选支付渠道。
const PAY_CHANNELS = [
  { id: 'wechat', name: '微信支付', color: '#09bb07', mark: '微' },
  { id: 'alipay', name: '支付宝', color: '#1677ff', mark: '支' },
];

// paxPaySheet 打开收银台：先选渠道再确认支付。
function paxPaySheet(orderNo) {
  if (!loggedIn()) { toast('请先登录'); openLogin(); return; }
  S.sheet = { type: 'pay', orderNo, channel: '', error: '' };
  renderAll();
}

// paxPickChannel 收银台选择支付渠道。
function paxPickChannel(channelId) {
  if (S.sheet && S.sheet.type === 'pay') { S.sheet.channel = channelId; renderAll(); }
}

// paySheet 渲染收银台：金额 + 微信/支付宝渠道选择 + 确认按钮。
function paySheet() {
  const st = S.sheet;
  const order = S.orders.find((o) => o.no === st.orderNo) ||
    (S.currentOrder && S.currentOrder.no === st.orderNo ? S.currentOrder : null);
  const fee = order && order.payFee != null ? order.payFee : 0;
  let h = '<h3>收银台 <button class="x" onclick="paxCloseSheet()">✕</button></h3>';
  h += `<div style="text-align:center;margin:8px 0 12px;"><div class="note">订单 ${esc(st.orderNo)}</div>` +
    `<div style="font-size:30px;font-weight:700;color:#303133;">¥${fy(fee)}</div></div>`;
  h += '<div class="couponlist">';
  for (const ch of PAY_CHANNELS) {
    const on = st.channel === ch.id;
    h += `<div class="crow ${on ? 'on' : ''}" onclick="paxPickChannel('${ch.id}')">` +
      `<div class="face"><b style="width:30px;height:30px;border-radius:8px;background:${ch.color};color:#fff;display:inline-flex;align-items:center;justify-content:center;font-size:14px;">${ch.mark}</b></div>` +
      `<div class="meta"><b>${esc(ch.name)}</b><small>跳转${esc(ch.name)}完成付款（演示环境模拟）</small></div>` +
      '<span class="pick"></span></div>';
  }
  h += '</div>';
  h += `<button class="btn yellow" style="width:100%;margin-top:10px;${st.channel ? '' : 'opacity:.5;pointer-events:none;'}" onclick="paxConfirmPay()">确认支付 ¥${fy(fee)}</button>`;
  if (st.error) h += `<div class="note" style="color:#f56c6c;">${esc(st.error)}</div>`;
  return h;
}
// paxConfirmPay 确认支付：创建支付单 → 渠道回调（演示环境模拟支付成功）。
// 回调成功后网关联动下游：订单推进到已完成、司机域记账结算；
// 完成后引导乘客对本次行程评价。
async function paxConfirmPay() {
  const st = S.sheet;
  if (!st || st.type !== 'pay' || !st.channel) return;
  const orderNo = st.orderNo;
  const order = S.orders.find((o) => o.no === orderNo) ||
    (S.currentOrder && S.currentOrder.no === orderNo ? S.currentOrder : null);
  const fee = order && order.payFee != null ? order.payFee : 0;
  try {
    st.error = '';
    await api('/payments', {
      method: 'POST', accountId: S.accountId, token: S.token,
      body: {
        order_no: orderNo, subject: '花小猪打车·车费', amount: fee,
        channel: st.channel, user_id: S.accountId,
        idempotent_key: `pay-${orderNo}`,
      },
    });
    const tradeNo = `demo-${Date.now()}`;
    await api('/payments/notify', {
      method: 'POST', accountId: S.accountId, token: S.token,
      body: { order_no: orderNo, trade_no: tradeNo, channel: st.channel, paid_amount: fee, paid_at_unix: Math.floor(Date.now() / 1000) },
    });
    S.flowNotified = false;
    stopFlowPoll();
    if (S.currentOrder) {
      S.currentOrder.state = 'COMPLETED';
      const row = S.orders.find((o) => o.no === S.currentOrder.no);
      if (row) row.state = 'COMPLETED';
    }
    S.currentOrder = null;
    S.tab = 'orders';
    await loadOrderList();
    toast('支付成功，订单已完成');
    // 支付完成立即引导评价（可跳过）。
    S.sheet = { type: 'review', orderNo, rating: 5, tags: [], complaint: false, error: '' };
  } catch (error) {
    if (S.sheet && S.sheet.type === 'pay') { S.sheet.error = error.message; }
    toast(`支付失败：${error.message}`);
  }
  renderAll();
}
// reviewSheet 评价/投诉面板：星级 + 标签 + 评语；投诉模式追加类型与描述。
function reviewSheet() {
  const st = S.sheet;
  const TAGS = ['服务好', '开车稳', '车内整洁', '准时到达', '路线合理'];
  let h = '<h3>' + (st.complaint ? '投诉司机' : '评价本次行程') + ' <button class="x" onclick="paxCloseSheet()">✕</button></h3>';
  h += '<div style="text-align:center;font-size:26px;letter-spacing:6px;margin:6px 0;">';
  for (let i = 1; i <= 5; i++) {
    h += `<span style="cursor:pointer;color:${i <= (st.rating || 0) ? '#ffc300' : '#dcdfe6'};" onclick="paxPickRating(${i})">★</span>`;
  }
  h += '</div>';
  if (!st.complaint) {
    h += '<div style="display:flex;gap:6px;flex-wrap:wrap;margin:8px 0;">' +
      TAGS.map((t) => `<button class="btn ${(st.tags || []).includes(t) ? 'blue' : 'ghost'}" style="flex:none;padding:4px 10px;font-size:11px;" onclick="paxToggleReviewTag('${esc(t)}')">${esc(t)}</button>`).join('') +
      '</div>';
  }
  h += '<textarea id="rvComment" placeholder="' + (st.complaint ? '补充说明（选填）' : '写下您的评价（选填）') + '" style="width:100%;box-sizing:border-box;height:60px;margin-bottom:8px;"></textarea>';
  h += '<div style="display:flex;align-items:center;gap:6px;margin-bottom:8px;">' +
    `<input type="checkbox" id="cmpSwitch" ${st.complaint ? 'checked' : ''} onchange="paxToggleReviewComplaint()" style="flex:none;">` +
    '<label for="cmpSwitch" style="font-size:12px;color:#606266;">我要投诉司机</label></div>';
  if (st.complaint) {
    h += '<select id="cmpType" style="width:100%;box-sizing:border-box;margin-bottom:8px;font-size:12px;">' +
      '<option value="">请选择投诉类型</option>' +
      '<option value="service_attitude">服务态度差</option>' +
      '<option value="route_detour">故意绕路</option>' +
      '<option value="unsafe_driving">危险驾驶</option>' +
      '<option value="overcharge">乱收费</option>' +
      '<option value="other">其他问题</option></select>';
    h += '<textarea id="cmpDesc" placeholder="请描述问题经过（必填，500 字内）" style="width:100%;box-sizing:border-box;height:60px;margin-bottom:8px;"></textarea>';
  }
  if (st.error) h += `<div class="note" style="color:#f56c6c;">${esc(st.error)}</div>`;
  h += '<button class="btn dark" style="width:100%;" onclick="paxSubmitReview()">' + (st.complaint ? '提交投诉' : '提交评价') + '</button>';
  return h;
}

window.paxPaySheet = paxPaySheet;
window.paxPickChannel = paxPickChannel;
window.paxConfirmPay = paxConfirmPay;
window.paxOpenDestChange = paxOpenDestChange;
window.paxPickRating = pickRating;
window.paxToggleReviewTag = toggleReviewTag;
window.paxToggleReviewComplaint = toggleReviewComplaint;
window.paxSubmitReview = submitReview;
window.paxOpenReview = paxOpenReview;