# 规则 3：接口、入参出参、协议约束

> 核心原则：**接口先有契约，AI 只按契约实现，禁止自造接口、自定协议、擅自改库。**
> 适用范围：`bw-cli` 项目中所有对外/对内接口、入参出参、字段、协议（gRPC proto、REST 路由、领域事件）。

---

## 1. 接口契约约束

### 1.1 契约唯一来源
- 本项目 gRPC 契约的唯一来源是 `api/proto/**/*.proto`，生成代码位于 `api/gen/**/*.pb.go`。
- REST 契约来源是 `internal/gateway/router/*_routes.go` + `internal/gateway/request/*_request.go`。
- 跨模块事件契约唯一来源是 `pkg/contracts/events.go`。

### 1.2 AI 的角色：只实现，不定义
- AI **只能**根据已存在的契约生成实现代码。
- AI **禁止**：
  - 新建/修改 `api/proto` 下的 `.proto` 文件（定义接口、增删 RPC、改字段）。
  - 手工编辑 `api/gen` 下的 `.pb.go`（必须由 `protoc` 生成）。
  - 修改 `pkg/contracts/events.go` 中已发布的事件常量、状态机取值、事件载荷字段。
  - 自造 REST 路由、自定 HTTP 方法/路径、自定 JSON 字段名。
  - 自定错误码，必须复用 `pkg/errors` 的 `Kind` 与既有 `code`。

### 1.3 契约版本管理
- 任何接口/字段变更，必须先改 `.proto`（走 MR 评审），再 `make proto` 重新生成 `.pb.go`。
- 禁止"先改 `.pb.go` 再补 proto"的倒序操作。
- 已上线的字段不得删除、不得改类型、不得改语义，只能新增字段做兼容。
- 字段编号（proto field number）一旦使用不得复用、不得调整。

---

## 2. 入参出参约束（分层各司其职）

项目要求四类结构各归其位，AI 不得混用：

| 层 | 结构 | 职责 | 示例 |
|----|------|------|------|
| 协议层 | proto / request | 承载 gRPC / HTTP 协议细节与标签 | `order_request.go`、`api/proto/order/v1` |
| 用例入参 | `dto/*Command` | handler 组装、service 消费的纯业务入参 | `dto.CreateCommand` |
| 用例出参 | `dto/*DTO` | service 返回、handler 转换的纯业务出参 | `dto.OrderDTO` |
| 持久化模型 | `model/*` | Gorm/Mongo 模型 | `model.OrderModel` |
| 领域实体 | `entity/*` | 聚合根与业务规则 | `entity.Order` |

### 2.1 字段约束（禁止私自新增/修改字段）
- 任何**对外接口字段、事件载荷字段、数据库字段**的增删改，都必须先人工确认。
- AI 禁止私自给 `proto`、`request`、`DTO`、`entity`、`model` 新增字段。
- 字段命名遵循契约已有风格：金额用 `Fen` 后缀（`int64`，单位分）、时间用 `RFC3339` 字符串、坐标用 `LatLng`。
- 数据库字段变更约束见规则 5（DDL 单独评审）。

### 2.2 参数校验约束
- 入参校验必须显式：空值、长度、范围、分页上下限、枚举合法性。
- REST 层用 `binding` 标签做基础校验；业务规则校验放在 `entity` 构造/`service` 编排中。
- 禁止在 `handler` 里写业务校验，禁止在 `service` 里读协议标签。

### 2.3 错误码与错误映射
- 只允许使用 `pkg/errors` 的统一错误体系（`Kind` + `code`）。
- `handler` 用 `mapXxxError` 映射领域错误到 `AppError`，禁止自造错误格式、自定 HTTP/gRPC 状态码。

---

## 3. 协议与序列化约束

- gRPC 服务端适配器必须继承 `UnimplementedXxxServer`，保证新增 RPC 时向前兼容。
- 事件载荷（`pkg/contracts`）字段必须带 `json` 标签，命名用 `snake_case`。
- 事件 `event_id`、`idempotency_key` 必须唯一，用于幂等（支付/退款/提现等必须幂等）。
- 禁止在载荷里直接透传数据库模型或含 `gorm`/`bson` 标签的结构，避免协议与存储耦合。

---

## 4. 禁止行为

- ❌ 未先定契约就写实现（先改 proto / 先评审再编码）。
- ❌ AI 私自新建/修改 `.proto`、`.pb.go`、事件契约、状态机常量。
- ❌ 私自给接口、事件、DTO、entity、model 新增/删除/改类型字段。
- ❌ 删除已上线字段、复用 proto field number、改字段语义。
- ❌ 自造错误码、自定状态码、自定 JSON 字段名。
- ❌ 把 `gorm`/`bson` 标签结构直接暴露到协议或事件载荷。
- ❌ 在 `handler` 写业务逻辑、在 `service` 读协议对象（应只消费 `Command`）。
- ❌ 绕过统一错误映射直接返回裸 `error` 或 `fmt.Errorf` 给客户端。

---

## 5. 交付自查清单

- [ ] 本次是否新增/修改了任何契约（proto / 事件 / 路由 / 字段）？若是，是否已人工评审？
- [ ] 入参出参是否使用项目既有的 Command / DTO 结构，未混入协议标签
- [ ] 字段命名是否遵循项目约定（`Fen`、`snake_case`、`RFC3339`）
- [ ] 错误是否走 `pkg/errors` + `mapXxxError`
- [ ] 事件载荷是否带 `json` 标签、是否幂等
- [ ] 是否复用既有错误码与枚举，未自造
