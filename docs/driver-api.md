# 司机端接口文档（driver-service）

> 对应方案：陈春博负责的司机端模块（D01–D04 接单大厅、D09–D11 首页收益对比、D12–D13 收入结算）。
> 本版本已对齐《花小猪打车平台_各模块技术点与实行方案》的统一技术底座，所有跨模块字段按本文约定与其他模块对接。

## 一、通用约定

| 约定 | 说明 |
| --- | --- |
| 基础路径 | `http://<host>:8080/api/v1`（gateway RPC 转发到 driver-service gRPC `:9101`） |
| 响应信封 | `{ "request_id": "...", "data": {...} }`；错误为 `{ "request_id": "...", "error": { "code": "...", "message": "..." } }` |
| 金额 | **一律整数分**（`int64`，字段名以 `_fen` 结尾），1 元 = 100 分，禁止浮点 |
| 坐标 | GCJ02，纬经度为 `double`；距离单位为**整数米** |
| 时间 | 业务日按 **Asia/Shanghai** 自然日切分，时间戳为 RFC3339（带时区） |
| 主键 | UUID 字符串（分布式生成，多实例不冲突） |
| 幂等键 | 写接口支持 `idempotency_key`，同键重复提交返回同一业务结果，不会重复扣款/记账 |
| 乐观锁 | 状态类写接口支持 `version`，不传或传 0 表示不校验；冲突返回 `version_conflict` |
| 分页 | `page`（从 1 开始）+ `page_size`（上限 100） |
| 零值字段 | 值为 0 或缺省的字段不返回（protobuf 语义），前端按「缺失即 0」处理 |

## 二、状态机

### 2.1 订单状态（与订单域共用一套六态）

```text
WAITING ──抢单/邀请确认──> ACCEPTED ──开始行程──> IN_TRIP ──送达──> ENDED
   │            │                                  │
   │            └──── 退单/超时回收 ──> WAITING      └── 平台/乘客取消 ──> CANCELLED
   └──── 派单超时（平台责任） ──> CANCELLED
```

### 2.2 司机状态（四态）

```text
offline ──出车──> idle ──抢单/邀请──> pre_occupied ──确认接单──> busy
   ^               │                     │                      │
   └──── 收车 ─────┘                     └──── 邀请超时释放 ─────┘
                                                         完单/退单 → idle
```

- `pre_occupied`（接单预占）是抢单/邀请后的临时态，15 秒内未确认自动释放。
- 忙碌掉线保留任务；空闲连续 60 秒无心跳自动收车。
- 「本单后收车」置位后，完单瞬间自动转 `offline`。

### 2.3 提现单状态

```text
reviewing ──审批通过──> approved ──打款──> paying ──回调──> success
    │                                      
    └──驳回──> rejected（额度原路返还可用桶）
```

### 2.4 资金五桶

| 桶 | 含义 |
| --- | --- |
| U（pending） | 待结算：乘客支付已到账、财务域尚未确认结算 |
| A（available） | 可用：可提现金额 |
| Fz（frozen） | 冻结：风控/争议冻结 |
| W（withdraw） | 提现预占：申请后到打款前占用 |
| Debt（debt） | 待追偿：退款/处罚产生的欠款 |

流转规则：外部资金 → U（支付成功）；U → A（财务域结算确认）；A → W（申请提现预占）；W → 出账（打款成功）或 W → A（驳回/打款失败）。

## 三、司机档案与状态

### 1. 注册司机

`POST /driver/profile`

```json
{ "name": "张三", "phone": "13800000001" }
```

响应要点：`id`、`driver_no`、`status=idle`、`mode=grab`、`qualified=true`、`funds` 五桶、`version=1`。

### 2. 查询档案

`GET /driver/profile/{id}`

返回档案、接单区域、心跳时间、资金五桶、当月累计收入、`active_task_id`。

### 3. 收车 / 出车

`POST /driver/profile/{id}/status`

```json
{ "status": "offline", "version": 3 }
```

`status` 仅接受 `offline`（收车）/ `idle`（出车）。忙碌或接单预占状态下调用会被拒绝（`busy_cannot_offline` / `pre_occupied_busy` / `status_not_allowed`）。

### 4. 听单 / 抢单模式切换

`POST /driver/profile/{id}/mode`

```json
{ "mode": "listen", "version": 2 }
```

- 仅空闲可切换（`mode_switch_denied`）。
- 切换会递增 `mode_version`，旧邀请立即失效，并释放该司机名下待确认推送。

### 5. 接单区域偏好

`PUT /driver/profile/{id}/area`

```json
{
  "area_limit_switch": true,
  "areas": [
    { "name": "成都春熙路", "min_lat": 30.60, "min_lng": 104.00, "max_lat": 30.75, "max_lng": 104.15 }
  ],
  "version": 4
}
```

开启限制后，只有起点落在片区内的订单可见/可派。片区必须 `min < max` 且经纬度合法，否则 `invalid_input`。

### 6. 心跳上报

`POST /driver/profile/{id}/heartbeat`

```json
{ "lat": 30.657, "lng": 104.081 }
```

建议 5 秒一次；坐标用于派单距离与在线时长计分。空闲连续 60 秒无上报由调度器自动收车。

### 7. 本单后收车

`POST /driver/profile/{id}/offline-after-trip`

```json
{ "on": true, "version": 5 }
```

## 四、接单大厅（D01–D04）

### 8. 创建派单任务

`POST /driver/hall/orders`

```json
{
  "passenger_name": "乘客甲",
  "origin": { "name": "春熙路", "lat": 30.657, "lng": 104.081 },
  "destination": { "name": "天府广场", "lat": 30.657, "lng": 104.065 },
  "passenger_paid_fen": 3000,
  "orig_fare_fen": 3300,
  "platform_coupon_fen": 300,
  "driver_coupon_fen": 0,
  "subsidy_fen": 0,
  "passenger_count": 1,
  "city_code": "510100",
  "order_type": "normal",
  "require_seats": 0,
  "require_car_type": "",
  "order_id": "订单域订单号（可选）"
}
```

金额口径：

```text
司机应得 = 乘客实付 - 平台佣金 - 司机承担券 + 平台补贴
平台佣金 = 乘客实付 × commission_rate（默认 20%）
平台承担券不计入司机收入，由平台补足给乘客
```

分发流程（单通道，避免重复派单）：

1. 先进入**听单通道**：按候选评分邀请司机，`dispatch_channel=listening`、`visible_in_pool=false`；
2. 听单窗口（默认 60 秒）结束或确认无人听单时转**抢单池**：`dispatch_channel=grab_pool`、`visible_in_pool=true`；
3. 总派单时限（默认 120 秒）内无人接单 → 平台责任取消（`CANCELLED`）。

### 9. 大厅列表

`GET /driver/hall/orders?driver_id={id}&limit=20&lat=30.657&lng=104.081`

- 只返回抢单池中、未被他人占用、且符合车型/座位/接单区域的订单；
- `pickup_distance_meters` 为司机最近心跳位置到起点的整数米。

### 10. 抢单

`POST /driver/hall/orders/grab`

```json
{ "driver_id": "...", "order_id": "...", "idempotency_key": "grab-1" }
```

原子预占订单并绑定司机（`pre_occupied`），返回 15 秒 `accept_deadline`。并发抢同一单只有一个成功，其余返回 `order_not_available`。

### 11. 确认接单

`POST /driver/hall/orders/accept`

```json
{ "driver_id": "...", "order_id": "...", "version": 2 }
```

成功后订单 `ACCEPTED`、司机 `busy`；超时未确认由调度器回收并更换下一候选。

### 12. 退单

`POST /driver/hall/orders/release`

```json
{ "driver_id": "...", "order_id": "...", "reason": "临时有事", "responsible_party": "driver" }
```

- 仅「未开始行程」可退，行程中返回 `trip_already_started`；
- **司机责任**计入每日上限（默认 3 次），乘客/平台责任不计入；
- 退单后订单按原通道重派，并继承剩余派单预算（不重置计时）。

### 13. 开始行程

`POST /driver/hall/orders/start`

```json
{ "driver_id": "...", "order_id": "..." }
```

通常由行程域（文钦）在乘客上车后调用，司机端与行程域共用同一状态口径。

### 14. 送达结束

`POST /driver/hall/orders/complete`

```json
{ "driver_id": "...", "order_id": "...", "distance_meters": 5200, "duration_sec": 900 }
```

- 订单 `ENDED` 并**立即释放运力**（与付款无关）；
- 返回 `order` 与自动生成的收益流水 `flow`（课程模拟：`auto_settle_on_trip_end=true` 时支付域/财务域未接入前自动记账并结算到可用桶；接入后关闭该开关，改由支付成功事件与结算接口驱动）。

### 15. 取消订单（乘客/平台责任）

`POST /driver/hall/orders/cancel`

```json
{ "order_id": "...", "responsible_party": "passenger", "reason": "乘客取消" }
```

释放司机运力，不计入司机退单次数。

## 五、首页收益对比（D09–D11）

### 16. 今日 / 昨日收益对比

`GET /driver/income/compare?driver_id={id}`

```json
{
  "today": { "date": "2026-10-22", "order_count": 3, "income_fen": 8400, "refund_fen": 0 },
  "yesterday": { "date": "2026-10-21", "order_count": 2, "income_fen": 5600, "refund_fen": 0 },
  "order_diff": 1,
  "income_diff_fen": 2800,
  "percent_text": "较昨日 +50.0%",
  "summary": "今日 3 单，收入 84.00 元，较昨日 +50.0%",
  "pending_income_fen": 0,
  "last_updated_at": "2026-10-22T16:00:00+08:00"
}
```

双口径说明（必须与订单域、支付域保持一致）：

- **接单量**：按行程结束日（`ended_at`）统计，拼车按乘客数计（2 乘客算 2 单）；
- **收入**：按记账日（`accounting_date`，Asia/Shanghai）统计，跨日退款记入退款当日；
- 昨日为 0 时不展示百分比（`percent_text` 为空），只展示绝对差值。

## 六、收入结算（D12–D13）

### 17. 收入流水

`GET /driver/income/flows?driver_id={id}&page=1&page_size=20`

返回正向（`income`/`subsidy`）与反向（`refund`/`punishment`）流水；反向流水通过 `reverse_of` 指向原凭证，**绝不修改原凭证**。

### 18. 收入概览

`GET /driver/income/summary?driver_id={id}`

```json
{
  "pending_fen": 0,
  "available_fen": 16000,
  "frozen_fen": 0,
  "withdraw_fen": 0,
  "debt_fen": 0,
  "monthly_income_fen": 16000,
  "min_withdraw_fen": 1000,
  "max_withdraw_fen": 200000,
  "daily_limit_fen": 500000,
  "daily_max_count": 3,
  "dual_approval_fen": 50000,
  "withdrawn_today_fen": 0,
  "withdraw_count_today": 0
}
```

提现规则（由后台 A06 配置下发，前端直接展示，不要硬编码）：

| 规则 | 默认值 | 错误码 |
| --- | --- | --- |
| 最低提现门槛 | 10 元（1000 分） | `below_min_withdraw` |
| 单笔上限 | 2000 元（200000 分） | `above_max_withdraw` |
| 单日累计上限 | 5000 元（500000 分） | `daily_withdraw_limit` |
| 单日受理次数 | 3 次 | `daily_withdraw_count` |
| 双人审批阈值 | 500 元（50000 分） | `dual_approval_required` |
| 可提现余额 | 可用桶 A | `balance_insufficient` |

### 19. 申请提现

`POST /driver/income/withdraw`

```json
{ "driver_id": "...", "amount_fen": 1000, "idempotency_key": "wd-20261022-1" }
```

流程：额度校验 → 预占 `A → W` → 落提现单（`reviewing`）→ 后台审批 → 打款回调。同幂等键重复提交返回原单且不重复预占。

### 20. 提现记录

`GET /driver/income/withdrawals?driver_id={id}&page=1&page_size=20`

## 七、跨模块接口（供其他模块调用）

### 21. 支付成功记账（支付域 → 司机端）

`POST /driver/payments`

```json
{
  "order_id": "...",
  "driver_id": "...",
  "accounting_date": "2026-10-22",
  "event_id": "evt-pay-xxx",
  "idempotency_key": "pay-key-1",
  "amount": {
    "orig_fare_fen": 3300,
    "passenger_paid_fen": 3000,
    "platform_coupon_fen": 300,
    "driver_coupon_fen": 0,
    "commission_fen": 600,
    "subsidy_fen": 0,
    "driver_income_fen": 2400
  }
}
```

- 资金进入**待结算桶 U**，此时不可提现；
- 同 `event_id` 重复投递返回原凭证，不重复记账。

### 22. 退款回调（支付域 → 司机端）

`POST /driver/refunds`

```json
{
  "order_id": "...",
  "refund_fen": 1000,
  "responsible_party": "passenger",
  "accounting_date": "2026-10-22",
  "reason": "乘客退款",
  "event_id": "evt-refund-xxx"
}
```

- 生成**反向流水**（`refund` 或司机责任的 `punishment`），不改原凭证；
- 跨日退款计入退款当日，之前日期的收益对比保持不变；
- 幂等由 `event_id` 唯一索引保证，失败可安全重试。

### 23. 结算确认（财务域 → 司机端）

`POST /driver/settlements`

```json
{ "driver_id": "...", "flow_ids": ["flow-1", "flow-2"] }
```

不传 `flow_ids` 表示结算该司机全部待结算流水（不传 `driver_id` 表示全量批处理）：**U → A**，同时写资金分录。

### 24. 提现审批与打款（后台 / 财务域 → 司机端）

| 接口 | 请求体 | 说明 |
| --- | --- | --- |
| `POST /driver/withdrawals/approve` | `{ "withdrawal_id": "...", "approver_id": "admin01", "comment": "ok" }` | 单人审批；达到阈值需两位**不同**审批人（`duplicate_approver` 拦截重复审批） |
| `POST /driver/withdrawals/reject` | `{ "withdrawal_id": "...", "approver_id": "admin01", "reason": "信息不全" }` | 驳回并返还 `W → A` |
| `POST /driver/withdrawals/complete` | `{ "withdrawal_id": "...", "success": true }` | 打款结果回调；失败则返还额度。双人审批未满足返回 `dual_approval_required` |

## 八、领域事件

事件先写入可靠事件表（outbox，与业务同库同事务），由投递器发布到 RabbitMQ；消费端按 `event_id` 去重。

| 事件 | 触发 | 主要消费方 |
| --- | --- | --- |
| `order.created`（消费） | 订单域下单 | 司机端创建派单任务 |
| `driver.task.assigned` | 邀请/抢单成功 | 订单域（占用）、行程域（预热） |
| `driver.task.accepted` | 确认接单 | 订单域、通知 |
| `driver.task.released` | 退单/超时回收 | 订单域（回到待派） |
| `driver.trip.ended` | 送达结束 | 行程域、支付域（发起收款） |
| `driver.status.changed` | 状态流转 | 后台监控、风控 |
| `driver.income.recorded` | 记账完成 | 财务域（对账） |
| `driver.withdrawal.requested` | 提现申请 | 财务域、后台审批中心 |

## 九、错误码

| 错误码 | HTTP | 场景 |
| --- | --- | --- |
| `invalid_request` | 400 | 网关格式/必填/类型校验失败 |
| `invalid_input` | 400 | 业务校验失败（姓名全空格、片区非法、坐标非法、金额为 0 等） |
| `driver_not_found` | 404 | 司机不存在 |
| `order_not_found` | 404 | 订单不存在（先查后改的接口） |
| `withdraw_not_found` | 404 | 提现单不存在 |
| `order_not_visible` | 409 | 订单尚未进入抢单池（听单窗口内/已超时） |
| `order_not_available` | 409 | 订单已被占用或状态不允许（原子条件更新未命中） |
| `order_not_mine` | 409 | 订单不属于该司机 |
| `order_state_denied` | 409 | 订单状态不允许该操作 |
| `trip_already_started` | 409 | 行程已开始，不能退单 |
| `driver_not_idle` | 409 | 司机非空闲/预占，不能接单 |
| `driver_unqualified` | 409 | 资质无效 |
| `driver_forbidden` | 409 | 被后台禁止接单 |
| `busy_cannot_offline` | 409 | 忙碌中不能直接收车 |
| `pre_occupied_busy` | 409 | 有待确认邀请，不能收车 |
| `status_not_allowed` | 409 | 状态流转不被允许 |
| `mode_switch_denied` | 409 | 非空闲不能切换听抢模式 |
| `area_change_denied` | 409 | 非空闲不能修改接单区域 |
| `version_conflict` | 409 | 乐观锁版本冲突 |
| `release_limit_exceeded` | 409 | 当日退单次数达上限 |
| `below_min_withdraw` / `above_max_withdraw` | 409 | 提现金额越界 |
| `balance_insufficient` | 409 | 可提现余额不足 |
| `daily_withdraw_limit` / `daily_withdraw_count` | 409 | 单日提现累计/次数超限 |
| `dual_approval_required` | 409 | 大额提现需双人审批 |
| `duplicate_approver` | 409 | 同一审批人重复审批 |
| `withdraw_state_denied` | 409 | 提现单状态不允许该操作 |
| `duplicate_event` | 409 | 事件已处理（内部幂等兜底） |

## 十、与其他模块的协作要点

| 协作方 | 模块 | 约定 |
| --- | --- | --- |
| 贺敬毅 | 订单域 | 共用六态；下单事件 `order.created` 由司机端消费建派单任务；接单/退单/完单结果回传订单域 |
| 文钦 | 行程域 | `start`/`complete` 与行程域共用状态口径，行程结束事件驱动支付域收款 |
| 孙源 | 支付域 | 支付成功调 `/driver/payments` 记账，退款调 `/driver/refunds` 生成反向流水；金额拆分字段与本文一致 |
| 寅杰 | 财务域 | 结算确认调 `/driver/settlements`（U→A）；打款结果回调 `/driver/withdrawals/complete` |
| 李江 | 后台管理 | 抽成比例、派单权重、提现规则由后台下发（本服务从配置读取，字段名一致）；审批走 `/driver/withdrawals/approve|reject` |

## 十一、配置项（configs/config.yaml `driver` 段）

```yaml
driver:
  commission_rate: 0.2              # 平台抽成比例
  min_withdraw_fen: 1000
  max_withdraw_fen: 200000
  daily_withdraw_limit_fen: 500000
  daily_withdraw_count: 3
  dual_approval_fen: 50000
  daily_release_limit: 3
  accept_timeout_seconds: 15        # 邀请确认超时
  listen_window_seconds: 60         # 听单窗口，之后转抢单池
  dispatch_window_seconds: 120      # 总派单时限
  heartbeat_timeout_seconds: 60     # 空闲无心跳收车
  hall_list_limit: 50
  auto_settle_on_trip_end: true     # 课程模拟：接入支付/财务域后置 false
  dispatch_weights:                 # 四项权重之和必须为 10000
    distance: 4000
    rating: 2500
    online: 1500
    success: 2000
```

## 十二、数据存储

| 集合 | 说明 | 关键约束 |
| --- | --- | --- |
| `driver_profiles` | 司机档案 + 资金五桶 + 退单记录 | `active_task_id` 唯一（部分索引，非空才约束） |
| `hall_orders` | 派单任务 | `order_id` 唯一、`task_id` 唯一 |
| `income_flows` | 收益凭证（含反向流水） | `event_id` 唯一、`(driver_id,idempotency_key)` 唯一 |
| `withdraw_requests` | 提现单 | `(driver_id,idempotency_key)` 唯一 |
| `ledger_entries` | 资金分录（五桶流水） | 与凭证一一对应 |
| `outbox_events` | 可靠事件表 | `event_id` 唯一 |
| `inbound_events` | 消费去重表 | `(consumer,event_id)` 唯一 |
