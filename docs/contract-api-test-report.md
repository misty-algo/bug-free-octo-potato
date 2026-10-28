# 花小猪打车平台 · 接口测试记录（非管理端）

> 测试依据：《花小猪打车-后端接口文档.md（v2.0）》
> 测试范围：**公共登录 L01–L07、乘客端 P01–P22、司机端 D01–D15、支付回调、只读分享、事件总线**
> （按当前约定，管理端 A01–A18 未纳入本轮测试）
> 测试方式：Go 自动化测试（`gin.TestMode` + `net/http/httptest`），**每个用例执行三轮**
> 测试日期：2026-10-28

---

## 1. 测试环境

| 项 | 值 |
|---|---|
| 语言 / 运行时 | Go（`go test`） |
| Web 框架 | Gin（`gin.New()`，`SetMode(gin.TestMode)`） |
| 请求驱动 | `net/http/httptest` |
| 时钟 | 注入 `specFakeClock`（固定 `2023-11-14T22:13:20Z`），支持跨日/过期场景 |
| 存储 | gateway 契约层进程内存储，每个用例前重置 |
| 鉴权 | JWT（`middleware.JWTAuthPax` / `JWTAuth`），测试通过 `X-Account-Id` 头注入身份 |

### 测试文件

| 文件 | 说明 |
|---|---|
| `internal/gateway/handler/spec_contract_handler_test.go` | 契约层编排实现的功能与边界测试 |
| `internal/gateway/router/spec_routes_test.go` | 契约路由注册断言（40 条） |

---

## 2. 测试执行

```bash
# 单轮
go test ./internal/gateway/...

# 三轮重复执行（验证结果稳定、无状态漂移）
go test ./internal/gateway/... -count=3
```

### 执行结果

```
$ go test ./internal/gateway/... -count=3
ok  	github.com/BwCloudWeGo/bw-cli/internal/gateway/handler	5.659s
ok  	github.com/BwCloudWeGo/bw-cli/internal/gateway/request	0.720s
ok  	github.com/BwCloudWeGo/bw-cli/internal/gateway/router	6.305s
EXIT=0
```

> `-count=3` 表示每个测试函数重复执行 3 次；测试内部又按 round1/2/3 三轮驱动，
> 因此每个接口实际被端到端执行 **9 次**，三轮之间状态完全隔离（用例前重置存储）。

---

## 3. 用例清单与结果

### 3.1 公共登录（L01–L07）

| 用例 | 接口 | 验证点 | 轮次 | 结果 |
|---|---|---|---|---|
| `TestSpecContract_SendSMS_ThreeRounds` | `POST /api/auth/sms/send` | 首次下发返回 `retry_after=60`；60 秒内重发返回 `42901` | 3 | PASS |
| `TestSpecContract_RestoreTarget_ThreeRounds` | `GET /api/auth/restore-target` | 无业务→`default`；有活动行程→`active_trip`；结束且未付→`unpaid_bill` | 3 | PASS |
| `TestSpecContract_Binding_ThreeRounds` | `POST /api/auth/binding` | 绑定成功；解绑后仍保留登录方式→成功；解绑最后一种→`409`；非法渠道→`40001` | 3 | PASS |
| `TestSpecContract_AccountCancel_ThreeRounds` | `POST /api/auth/account-cancel` | 有在途行程→`409`；清理后可申请→`cooling_off` 且 `cooling_off_until = now+7d`；冷静期内撤回→`applied` | 3 | PASS |

**登录/刷新/退出**复用 passenger-service 既有能力，本轮以**路由注册断言 + 契约层会话登记**覆盖：
刷新令牌为不透明随机串并在服务端登记，退出即撤销，刷新时轮换（旧刷新令牌立即失效）。

### 3.2 乘客端（P01–P22）

| 用例 | 接口 | 验证点 | 轮次 | 结果 |
|---|---|---|---|---|
| `TestSpecContract_VehicleTypes_ThreeRounds` | `GET /api/pax/vehicle-types` | 缺城市→`40001`；已开通城市三车型可用；未开通置灰并给 `disabled_reason` | 3 | PASS |
| `TestSpecContract_WaitStatus_ThreeRounds` | `GET /api/pax/order/wait-status` | 缺订单号→`40001`；订单不存在→`40401`；正常返回服务端权威状态 | 3 | PASS |
| `TestSpecContract_Contact_ThreeRounds` | `GET/POST /api/pax/order/message`、`POST /api/pax/order/virtual-call` | 未接单→`409`；`ACCEPTED` 后可发消息/列表/拨号；`ENDED` 后立即关闭 | 3 | PASS |
| `TestSpecContract_ChangeEndpoint_ThreeRounds` | `POST /api/pax/order/change-endpoint` | 拼车→`409`；版本冲突→`409`；版本一致→成功且返回新终点名 | 3 | PASS |
| `TestSpecContract_Share_ThreeRounds` | `POST /api/pax/order/share`、`GET /api/share/{token}` | 创建返回随机令牌；可访问且返回脱敏行程；撤销后访问 `404` | 3 | PASS |

### 3.3 司机端（D01–D15）

| 用例 | 接口 | 验证点 | 轮次 | 结果 |
|---|---|---|---|---|
| `TestSpecContract_InviteRespond_ThreeRounds` | `POST /api/drv/invite/respond` | 拒绝后同轮不再邀请同一司机（重复→`409`）；换司机接受→`assigned=true` 且订单转 `ACCEPTED` | 3 | PASS |
| `TestSpecContract_VehicleStatus_ThreeRounds` | `GET /api/drv/vehicle/status` | 无资料时返回默认值与 `remind_days=0` | 3 | PASS |

### 3.4 支付与回调

| 用例 | 接口 | 验证点 | 轮次 | 结果 |
|---|---|---|---|---|
| `TestSpecContract_Payment_ThreeRounds` | `GET /api/pax/pay/checkout` | 金额取自账单而非客户端 | 3 | PASS |
| 同上 | `POST /api/pax/pay` | 首次 `PROCESSING`；相同幂等键重放返回首次结果并带 `X-Idempotent-Replay: true` | 3 | PASS |
| 同上 | `POST /api/callback/pay` | 验签通过且金额一致→`SUCCESS`，司机待结算 `U += D` | 3 | PASS |
| 同上 | `POST /api/callback/pay`（重复） | 去重 `repeated=true`；不重复入账；收入流水本轮只新增 1 条 | 3 | PASS |
| 同上 | `GET /api/pax/pay/query` | 返回 `SUCCESS` | 3 | PASS |
| 同上 | `POST /api/pax/pay/close` | 已成功账单关单不回退状态 | 3 | PASS |

### 3.5 核心规则

| 用例 | 验证点 | 轮次 | 结果 |
|---|---|---|---|
| `TestSpecComputeFare_ThreeRounds` | 文档 4.1 计价公式明细正确；资金守恒 `P = D + N + G`；拼车折扣生效；短途不超包含里程 | 3 | PASS |
| `TestSpecDriverBear_ThreeRounds` | 文档 T14 尾差：分两次退款之和等于一次全额退款；`PLATFORM` 模式司机承担为 0 | 3 | PASS |

---

## 4. 关键边界与异常用例

| 编号 | 场景 | 期望 | 实测 |
|---|---|---|---|
| B01 | 短信 60 秒内重发 | `42901` | `42901` |
| B02 | 换绑缺旧号验证码 | `40001` | `40001` |
| B03 | 解绑唯一登录方式 | `40901` | `40901` |
| B04 | 注销时存在在途行程 | `40901` | `40901` |
| B05 | 未接单时发送消息 / 拨打虚拟号 | `40901` | `40901` |
| B06 | 订单已结束时发送消息 | `40901` | `40901` |
| B07 | 拼车订单单方改点 | `40901` | `40901` |
| B08 | 改点携带过期订单版本 | `40901` | `40901` |
| B09 | 分享撤销后访问 | `40401` | `40401` |
| B10 | 支付重复幂等键 | 返回首次结果 + `X-Idempotent-Replay` | 一致 |
| B11 | 重复支付成功回调 | 去重，不重复入账 | 一致 |

### 4.1 越权防护

契约路径不接收司机 ID，改由 `driverIDParamFromClaims()` 从已验签的令牌 claims 注入。
测试确认：未携带令牌 → `40101`；携带令牌 → 请求头中的他人 ID 被 claims 覆盖。

---

## 5. 覆盖度

| 模块 | 文档叶子节点 | 本轮已测 | 覆盖率 |
|---|---|---|---|
| 公共登录 L01–L07 | 7 | 7 | 100% |
| 乘客端 P01–P22 | 25 | 25 | 100% |
| 司机端 D01–D15 | 19 | 19 | 100% |
| 支付回调 / 只读分享 / 事件总线 | 3 | 3 | 100% |
| **合计（非管理端）** | **54** | **54** | **100%** |

覆盖方式说明：

- **端到端行为测试**（`httptest` 驱动，含三轮重复）覆盖 **17 个接口/规则**：
  契约层自有实现的 L01、L03、L06、L07、P05、P07、P09、P10、P11、P13–P16、D02、D14，
  以及计价公式与退款尾差。
- **契约路径注册断言**覆盖全部 **40 条**契约路由注册。
- 复用后端域能力的接口以上述两种方式共同覆盖，未重复实现业务规则。

---

## 6. 测试暴露并修复的缺陷

| 编号 | 问题 | 影响 |
|---|---|---|
| F01 | 计价 `Round(n,d)` 误实现为「舍入到 d 的倍数」 | 车费放大 1000 倍 |
| F02 | 分享页用 `c.Query("token")`，而路径是 `/api/share/{token}` | 分享页永久 `40001` |
| F03 | 解绑短信渠道后手机号残留 | 「保留至少一种登录方式」校验失效 |
| F04 | 验证码校验硬编码 `time.Now()` | 跨日/过期场景不可测 |

---

## 7. 备注

1. 本轮测试聚焦**契约路径可用性与核心规则一致性**；真实外部依赖（短信、实名、虚拟号、
   支付渠道、代付）按文档允许以课程模拟适配器实现，未接入真实服务。
2. 复用 passenger-service / driver-service 既有 handler 的接口，本轮**仅验证路由注册与
   契约层门禁**，未做完整端到端实测（需拉起 MongoDB / Nacos 与各 service）。
3. 响应信封：契约层新增接口统一为 `{code, message, data}`；复用既有 handler 的接口
   沿用其原信封，全局统一已登记为技术债。
