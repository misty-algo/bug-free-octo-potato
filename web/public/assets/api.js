// 页面与 Gin 网关之间的唯一出入口：页面部署在同源的静态服务上，
// 请求打到 /api/v1，由 web/server.mjs 反向代理到网关，所以不需要处理跨域。
const BASE = '/api/v1';

/**
 * api 调用乘客/司机端接口，并统一抛出「code：message」形式的错误。
 * 账号标识走 X-Account-Id，登录态走 Authorization，与后端约定一致。
 */
export async function api(path, { method = 'GET', body, accountId, token } = {}) {
  const headers = {};
  if (accountId) headers['X-Account-Id'] = accountId;
  if (token) headers['Authorization'] = `Bearer ${token}`;
  if (body !== undefined) headers['Content-Type'] = 'application/json';

  const response = await fetch(BASE + path, {
    method,
    headers,
    body: body === undefined ? undefined : JSON.stringify(body),
  });

  const raw = await response.text();
  let payload = null;
  try {
    payload = raw ? JSON.parse(raw) : null;
  } catch {
    payload = null;
  }

  if (!response.ok) {
    const error = payload && payload.error;
    const detail = error ? `${error.code || response.status}：${error.message}` : `HTTP ${response.status}`;
    throw new Error(detail);
  }
  return payload && payload.data !== undefined ? payload.data : payload;
}

/** fy 把「分」格式化成「元」金额字符串（保留两位小数），与原型 fy() 语义一致。 */
export function fy(fen) {
  return ((Number(fen) || 0) / 100).toFixed(2);
}

/** esc 防 XSS：所有来自接口的文本进 innerHTML 前先转义。 */
export function esc(text) {
  return String(text == null ? '' : text)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
    .replace(/"/g, '&quot;').replace(/'/g, '&#39;');
}

/** toast 复刻原型右上返回的轻提示，2.6 秒自动消失。 */
let toastTimer = null;
export function toast(message) {
  const el = document.getElementById('toast');
  el.textContent = message;
  el.style.display = 'block';
  if (toastTimer) clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { el.style.display = 'none'; }, 2600);
}

/** randomKey 生成幂等键：下单、抢单、提现都要求同键重复提交返回同一结果。 */
export function randomKey(prefix) {
  return `${prefix}-${Date.now()}-${Math.random().toString(36).slice(2, 8)}`;
}

/** stateName 订单状态中文名，与原型一致。 */
export function stateName(state) {
  return {
    PENDING: '等待接单', WAITING: '待接单', ACCEPTED: '已接单', ARRIVED: '已到达',
    IN_TRIP: '行程中', IN_SERVICE: '行程中', COMPLETED_UNPAID: '待支付',
    COMPLETED: '已完成', ENDED: '已结束', CANCELLED: '已取消',
  }[state] || state || '-';
}
