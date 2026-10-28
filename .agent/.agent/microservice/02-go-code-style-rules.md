# code-agent.md — Go 代码工程约束

> 前置：先加载 `core-agent.md`；落库代码同时遵守 `data-agent.md`，核心链路同时遵守 `business-agent.md`。
> 适用场景：写任何 Go 代码（实体 / 模型 / 服务 / 仓储 / handler / 测试）。
> 来源：bw-cli 脚手架（`bug-free-octo-potato`）真实代码约定，不是泛泛最佳实践。

---

## 1. 目录与分层

每个业务域一个微服务（`cmd/<域>` + `internal/<域>`），域内固定四层：

```
internal/<域>/
├── entity/     # 领域实体 + Repository 接口定义（不依赖任何框架）
├── model/      # 落库模型（GORM XxxModel / Mongo XxxDocument）
├── dto/        # 入参 command.go、出参 <域>.go
├── service/    # 只写业务流程编排
├── repo/       # Repository 接口的 Gorm/Mongo 实现
└── handler/    # gRPC server（网关侧在 internal/gateway/handler）
```

- 依赖方向：`handler → service → entity`，`repo → entity/model`。**entity 包 MUST NOT import Gin / gRPC / GORM / MongoDB SDK / 日志框架**。
- 新增服务用 `bw-cli service <name>` 生成，MUST NOT 手搓目录结构。
- 请求链路：`gateway/router → gateway/handler → gRPC → 域/handler → 域/service → entity.Repository → 域/repo → DB`。

## 2. 命名

| 对象 | 约定 | 示例 |
|---|---|---|
| 文件 / 包名 | 单数、小写 | `user.go`、包 `model` |
| 落库模型 | `XxxModel` / `XxxDocument` | `OrderModel`、`OrderDocument` |
| 领域实体 | 名词本身 | `Order` |
| 仓储接口 | 定义在 entity 包，叫 `Repository` | `entity.Repository` |
| 错误值 | `ErrXxx` 定义在 entity | `ErrUserNotFound` |
| 表 / 集合名 | 表带项目前缀（如 `xls_user`），集合按数据字典名 | — |

## 3. 错误处理

- MUST 使用 `pkg/errors`：`errors.New(kind, code, msg)` / `errors.Wrap(err, ...)`，按 `KindInvalidArgument / KindUnauthorized / KindNotFound / KindConflict / KindInternal` 分类，附带错误码。
- MUST NOT 把裸 `err` 直接返回给前端；MUST NOT 用 `panic` 处理业务错误。
- 对外响应统一走 `pkg/httpx`。

**错误码段分配**：错误码 4 位数字，格式 `域码 + 序号`；前 2 位为域码（`00` 通用 / `01` passenger / `02` driver / `03` pricing / `04` order / `05` settlement / `06` risk），后 2 位为域内序号（01-99）；同一域内错误码必须唯一，新错误码在对应域代码中登记，MUST NOT 复用已废弃错误码。

## 4. 日志

统一 `pkg/logger`（zap）。关键节点必打结构化日志，约定字段：`request_id`、`use_case`、`aggregate_id`（订单号/司机号等）、`latency_ms`。**日志里 MUST NOT 出现未脱敏的敏感字段**。

## 5. 接口契约

- 服务间调用走 gRPC：proto 改动后必须重新生成（`make proto`），MUST NOT 手改 `api/gen/` 下的生成代码。
- 对外 HTTP 路由统一在 `internal/gateway/router/` 注册，版本化（`/v1/...`）。

## 6. 测试

- service 层写单元测试：`testify` + 手写 `fakeRepository`，并断言 `var _ entity.Repository = (*fakeRepository)(nil)`。
- 涉及钱、状态机、派单并发的改动 MUST 附测试。
- 提交前跑 `make test`。

## 7. 注释

代码修改处附**中文注释说明"为什么"**（业务原因 / 规避什么坑），MUST NOT 只写"修改了 xx"这类无信息量注释。公共包导出符号写 GoDoc 注释。
