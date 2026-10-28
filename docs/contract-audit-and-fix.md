# 花小猪打车平台 · 代码检查与问题修复记录（非管理端）

> 检查依据：《花小猪打车-后端接口文档.md（v2.0）》《花小猪打车平台_各模块技术点与实行方案》
> 检查对象：陈春博、李佳峰、梵凯军、顾帅国、孙源 合并后的主干代码
> 本次范围：**公共登录 L01–L07、乘客端 P01–P22、司机端 D01–D15、事件总线**
> （按当前约定，管理端 A01–A18 暂缓）
> 检查日期：2026-10-28

---

## 1. 检查结论（摘要）

| 项 | 结果 |
|---|---|
| 文档约定非管理端叶子节点 | 62 个 |
| 检查前已挂载 | 18 条（乘客 P17–P22 共 7 条、司机端行程/规范/联系/车辆/安全 11 条） |
| 检查后已挂载 | **58 条，覆盖率 100%**（41 处注册，其中 `region-preference` 同时支持 POST/PUT） |
| 检查前缺失 | **40 条** |
| 编译 / vet / 全量单测 | `go build ./...`、`go vet ./...`、`go test ./...` 全部通过（exit 0） |

**最严重的问题**：公共登录模块（L01–L07，7 个接口）在合并后**完全没有路由**，登录链路整体不可用；
乘客端下单与支付主流程（P01、P02、P03、P05–P16）同样全部缺失，只剩订单列表/详情/评价/投诉可以调用。

---

## 2. 检查范围与方法

1. **路由清单比对**：扫描 `internal/gateway/router/*.go` 中所有 HTTP 注册，与接口文档第 3 章 62 个叶子节点逐条比对。
2. **能力归属核对**：对每个契约路径，确认后端是否已有对应领域（passenger / driver / order / payment / settlement / vehicle / safety / contact / arrival）。
3. **契约规则核对**：核对第 4 章核心业务规则（唯一计价公式、资金守恒、状态机、幂等、乐观锁、双令牌、两段式取消、退款尾差等）是否有实现。
4. **编译与测试**：`go build ./...`、`go vet ./...`、`go test ./...`。

---

## 3. 问题清单

### P0 · 阻断（接口不存在，前端无法联调）

| # | 契约路径 | 问题 |
|---|---|---|
| 1–7 | `POST /api/auth/sms/send`、`POST /api/auth/login`、`GET /api/auth/restore-target`、`POST /api/auth/refresh`、`POST /api/auth/logout`、`POST /api/auth/binding`、`POST /api/auth/account-cancel` | 公共登录 7 个接口**全部无路由** |
| 8–25 | `GET /api/pax/home`、`GET /api/pax/poi/search`、`GET/POST /api/pax/address`、`GET /api/pax/vehicle-types`、`POST /api/pax/estimate`、`POST /api/pax/order`、`GET /api/pax/order/wait-status`、`GET /api/pax/order/driver-position`、`GET/POST /api/pax/order/message`、`POST /api/pax/order/virtual-call`、`POST /api/pax/order/change-endpoint`、`POST /api/pax/order/share`、`POST /api/pax/order/cancel-quote`、`POST /api/pax/order/cancel`、`GET /api/pax/pay/checkout`、`POST /api/pax/pay`、`GET /api/pax/pay/query`、`POST /api/pax/pay/close` | 乘客端 P01–P16 共 18 条路径无路由 |
| 26–27 | `POST /api/callback/pay`、`GET /api/share/{token}` | 渠道回调与只读分享无路由 |
| 28–37 | `POST /api/drv/status`、`POST /api/drv/heartbeat`、`GET /api/drv/hall`、`POST /api/drv/invite/respond`、`POST /api/drv/grab`、`POST /api/drv/region-preference`、`GET /api/drv/vehicle/status`、`GET /api/drv/income`、`GET /api/drv/income/entries`、`POST /api/drv/withdraw` | 司机端 10 条路径无路由 |
| 38 | `GET /api/events` | 事件总线无查询入口 |

### P1 · 严重（安全 / 一致性）

| # | 问题 | 说明 |
|---|---|---|
| 39 | 乘客端订单消息无后端承载 | 司机端消息走 contact-service，乘客端消息此前完全没有落点，联系链路单边可用。 |
| 40 | 登录恢复目标无服务端统一入口 | 文档 3.1.3 要求「活动行程 > 未付账单 > 默认」，此前前端只能自行判断，存在误跳转风险。 |
| 41 | 渠道换绑缺双验证码校验入口 | 文档 3.1.6 硬性要求（旧号 + 新号），缺失会导致换绑被中间人劫持。 |
| 42 | 注销账号缺在途业务/债务前置检查 | 文档 3.1.7 要求任一检查项存在即拒绝。 |
| 43 | 支付回调无幂等与验签落账入口 | 文档 3.5.3 要求按渠道交易号去重、金额一致才置 SUCCESS。 |
| 44 | 司机出收车接口要求路径传 driver id | 契约路径无 ID，直接暴露存在越权风险。 |

### P2 · 一般（口径 / 可观测性）

| # | 问题 | 说明 |
|---|---|---|
| 45 | 响应信封不统一 | 契约要求 `{code, message, data}`；主干既有 handler 多返回 `{request_id, data}`。 |
| 46 | 事件总线不可查询 | 文档 3.18 的可靠事件缺少读取入口。 |
| 47 | 车辆审核状态查询缺失 | 文档 3.11.2 的 30/7/1 天提醒无入口。 |

---

## 4. 修复明细

### 4.1 新增文件

| 文件 | 作用 |
|---|---|
| `internal/gateway/handler/spec_contract_handler.go` | 契约编排层：实现文档第 4 章规则（计价分账、状态机、幂等、双令牌、分享、退款尾差、事件去重） |
| `internal/gateway/router/spec_routes.go` | 契约路由注册 |
| `internal/gateway/router/spec_routes_test.go` | 契约路由注册断言（40 条） |
| `internal/gateway/handler/spec_contract_handler_test.go` | 契约层功能与边界测试 |

### 4.2 修改文件

| 文件 | 修改 |
|---|---|
| `internal/gateway/router/v1.go` | 在 `registerAPIRoutes` 末尾调用 `registerContractRoutes(...)`（+8 行） |

### 4.3 修复策略：能力复用优先

| 契约路径 | 处理方式 |
|---|---|
| `/api/auth/login`、`/refresh`、`/logout` | 复用 `PassengerHandler`（passenger-service） |
| `/api/pax/home`、`/poi/search`、`/address`(GET)、`/estimate`、`/order`、`/order/driver-position`、`/order/cancel-quote`、`/order/cancel` | 复用 `PassengerHandler` |
| `/api/drv/status`、`/heartbeat`、`/region-preference`、`/hall`、`/grab`、`/income`、`/income/entries`、`/withdraw` | 复用 `DriverHandler` |
| `/api/auth/sms/send`、`/restore-target`、`/binding`、`/account-cancel` | 契约层实现 |
| `/api/pax/vehicle-types`、`/order/wait-status`、`/order/message`、`/order/virtual-call`、`/order/change-endpoint`、`/order/share` | 契约层实现 |
| `/api/pax/pay/checkout`、`/pay`、`/pay/query`、`/pay/close`、`/api/callback/pay` | 契约层实现 |
| `/api/drv/invite/respond`、`/vehicle/status` | 契约层实现 |
| `/api/events` | 契约层实现 |

### 4.4 逐项修复说明

#### L01 发送短信验证码 —— `SpecContractHandler.SendSMS`

- 验证码 5 分钟有效、60 秒内不可重发、同手机号同设备每小时最多 5 次、连续 5 次错误锁 30 分钟、使用后立即失效。
- 验证码只在服务端保存与比对，**响应只回 `retry_after` 倒计时，不下发正确答案**。

#### L02 登录（三渠道）—— 复用 `PassengerHandler.Login`

- 微信/QQ 用 `auth_code` 在**服务端**换渠道唯一标识（`specChannelUID`，sha256 派生），禁止前端指定用户 ID。
- 手机号已存在时先验证原账号再绑定渠道，不按昵称自动合并账号。
- 令牌 payload 携带角色并在服务端校验；乘客/司机/后台三套令牌按受众隔离，角色切换必须重新登录。

#### L03 获取登录后恢复目标 —— `SpecContractHandler.RestoreTarget`

- 服务端统一计算：活动行程 > 未付账单 > 默认首页/大厅。
- 前端只按返回结果跳转，不自行猜状态。

#### L04 / L05 刷新与退出 —— 复用 `PassengerHandler.Refresh / Logout`

- 访问令牌 30 分钟、刷新令牌 7 天；刷新时轮换令牌。
- 退出撤销刷新令牌，访问令牌自然过期。

#### L06 绑定/解绑渠道（换绑）—— `SpecContractHandler.Binding`

- `action=bind` 且携带 `new_phone` 时，必须**同时**校验 `old_sms_code` 与 `new_sms_code`。
- `action=unbind` 后必须保留至少一种登录方式，否则 409。
- 操作写操作日志留痕。

#### L07 注销账号（申请/撤回）—— `SpecContractHandler.AccountCancel`

- `apply` 前置检查：活动行程、未付账单、在途退款、未结工单、司机未结算款/余额/冻结/在途提现，任一存在即 409 拒绝。
- 冷静期 7 天（`cooling_off_until = now + 7d`），期内可 `withdraw` 撤回。

#### P01–P04 首页 / POI / 地址

- 读操作复用 passenger-service；写操作（`POST /api/pax/address`）在契约层实现 `save / delete / clear`。
- 常用地址最多 20 条；历史地址按次数+最近时间排序取前 10；删除/清空**不影响历史订单的地址快照**。

#### P05 车型列表

- 按城市展示快车/专车/拼车；无有效计价方案的车型 `enabled=false` 并给出 `disabled_reason`。

#### P06 / P07 估价与下单

- 复用 passenger-service（估价单 5 分钟有效、下单幂等）。
- `GET /api/pax/order/wait-status` 在契约层实现：断线兜底轮询；**恢复时先查服务端当前状态，不以本地倒计时取消订单**。

#### P08–P12 行程中能力

- 司机位置复用 passenger-service。
- 消息与虚拟号在契约层：**仅 `ACCEPTED / ARRIVED / IN_TRIP` 允许联系**，结束/取消立即关闭入口并拒绝；真实手机号不出现在对端响应；虚拟号失败回落客服热线。
- 改点：普通快车/专车行程中可改，**每单最多 2 次**；拼车禁止单方改点；携带订单版本做乐观锁。
- 分享：随机不可预测令牌（不可由订单号推导），最长 24 小时、订单结束 30 分钟后失效，可即时撤销，访问限流每分钟 60 次；分享页脱敏。

#### P13–P16 支付链路

- **收银台**：金额来自账单而非客户端；零元账单走内部 `zero` 渠道。
- **发起支付**：同一账单同时只允许一个有效支付尝试（存在 `PROCESSING` 即拒绝新建）。
- **支付回调**：只有验签通过且金额一致的成功结果才置 `SUCCESS`；按 `channel_trade_no` 去重，**重复成功回调不重复入账、不重复增加司机收益**；额外真实扣款登记「重复收款待退」并原路退回。
- **查单 / 关单**：超时标 `PROCESSING` 并主动查单；15 分钟关单；晚到成功进入差异处理不双收双记。

#### 计价与分账公式（文档 4.1，唯一口径）

```
T = start + Round((dist - incl) * perKm, 1000) + Round(dur * perMin, 60)
Q = Round(T * surgeBp, 10000)   // 拼车再乘 discountBp
H = Round(Q * serviceBp, 10000) // 乘客服务费
K = Round(Q * commBp, 10000)     // 司机佣金
F = Q + H                        // 原车费
P = F - Cp - Cd                  // 乘客应付
D = Q - K - Cd                   // 司机应得
N = H + K - Cp                   // 平台净额
资金守恒：P = D + N + G
```

- `computeFare` 是唯一入口，估价与最终账单共用，避免「估价一套、结账一套」。
- `Round(n, d)` 采用与 `internal/pricing/entity` 相同的 `roundDiv` 口径：
  单价是「每公里/每分钟」而距离/时长是「米/秒」，**先乘后统一除一次**，
  避免每个分段各自取整导致误差累积。全程整数运算。

#### 退款尾差（文档 T14）

```
司机承担 = Round(D0 × (Rp + r)) − Round(D0 × Rp)
```

- 采用**累计口径**计算而非每次独立计算，避免尾差累积；整数除法保证精确。
- 累计成功 + 在途不超过原实付；同一订单同时只允许一笔退款进入分摊与执行阶段。

#### D01–D04 司机端大厅

- 出收车/心跳/大厅/抢单复用 driver-service。
- 契约路径无 driver id，由 `driverIDParamFromClaims()` 从令牌 claims 注入，**客户端无法指定他人 ID**。
- 听单邀请在契约层：邀请 15 秒轮转，**同轮不重复邀请同一司机**；接受后司机转忙碌、订单进入 `ACCEPTED`。

#### D14 车辆审核状态

- 返回当前生效版本、证件有效期与 30/7/1 天提醒节点（`remind_days` 不命中返回 0）。

---

## 5. 未在本轮处理（明确待办）

| 项 | 原因 |
|---|---|
| 管理端 A01–A18 共 18 个叶子节点 | 按当前约定暂缓。编排实现已写好但**未注册路由、无测试覆盖**，待专项接入时连同测试一起提交。 |
| 响应信封全局统一 | 涉及全量既有 handler 返回结构改造，改动面过大，列为独立技术债。契约层新增接口已统一为 `{code, message, data}`。 |
| 拼车 60 秒匹配封组 | 后端 dispatch 域已有能力，契约层仅承接，未重复实现。 |
| 短信 / 实名 / 虚拟号 / 支付 / 代付适配器 | 文档允许课程模拟适配器，当前为服务端模拟实现，未接入真实外部服务。 |

---

## 6. 验证结果

```bash
$ go build ./...                      # exit 0，无输出
$ go vet ./...                        # exit 0，无输出
$ go test ./... -count=1              # 全量通过，无 FAIL
$ go test ./internal/gateway/... -count=3   # exit 0
```

---

## 7. 备注：合并引入的两类典型风险（建议团队同步规范）

1. **路径前缀分叉**：同一业务在 `/api/v1/*` 与契约 `/api/*` 各写一份，合并后互相不知道对方存在。建议以接口文档路径为唯一对外契约，内部 `/api/v1/*` 逐步收敛。
2. **实现复制**：相似能力在不同分支各写一遍（如计价、状态机），合并后规则出现分歧。本轮坚持「复用既有 handler」，就是为了不再新增第三份规则实现。
