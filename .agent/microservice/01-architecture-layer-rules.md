# 规则 4：架构 & 依赖分层约束

> 适用范围：`bw-cli` 项目的目录结构、包依赖方向、代码生成落位。
> 目标：**禁止私自修改框架结构，代码必须严格按本项目 DDD 分层生成。**

---

## 1. 项目既有分层（AI 必须严格遵循）

### 1.1 顶层目录职责
| 目录 | 职责 | 禁止事项 |
|------|------|----------|
| `api/proto` | gRPC 契约（`.proto`） | 禁止 AI 修改 |
| `api/gen` | protoc 生成的 `.pb.go` | 禁止手工编辑 |
| `cmd/*` | 各服务入口（main 启动装配） | 禁止写业务逻辑 |
| `configs/` | 配置（Viper） | 密钥/地址变更需人工 |
| `internal/{服务名}` | 各微服务内部实现 | 按 1.2 分层 |
| `internal/gateway` | HTTP 网关（router/handler/request/client） | 按 1.3 |
| `pkg/*` | 可复用公共组件 | 禁止放业务逻辑 |
| `docs/` | 文档 | 非代码 |

### 1.2 微服务内部六层（`internal/{服务名}`）
以 `internal/order` 为基准，每个服务必须保持一致：

```
internal/{服务名}/
├── entity/      # 领域层：聚合根、领域规则、仓储接口、领域错误
├── model/       # 持久化模型：Gorm 模型 + Mongo 文档模型
├── dto/         # 用例入参 Command / 出参 DTO / 转换函数
├── service/     # 用例编排：依赖 entity 接口，不依赖具体实现
├── repo/        # 仓储实现：Gorm / Mongo，实现 entity.Repository 接口
└── handler/     # 适配器：gRPC Server，组装 Command，映射错误
```

各层职责边界（AI 不得越界）：
- `entity`：只放领域对象、业务规则、仓储接口（如 `entity.Repository`）、领域错误（`ErrXxx`）。**禁止**依赖 `gorm`/`bson`/`proto`。
- `model`：只放持久化结构（`TableName()` / `MongoCollectionName()`），**禁止**放业务逻辑。
- `dto`：只放入参 Command、出参 DTO、转换函数（`FromXxx`），**禁止**放业务规则与协议标签。
- `service`：只做用例编排，依赖 `entity.Repository` 接口（依赖倒置），**禁止**直接依赖 `repo` 实现或 `*gorm.DB`。
- `repo`：实现 `entity.Repository`，编译期断言 `var _ entity.Repository = (*XxxRepository)(nil)`，负责 entity↔model 转换。
- `handler`：只做协议适配（`req → Command`、`DTO → proto`、错误映射），**禁止**写业务逻辑。

### 1.3 网关层（`internal/gateway`）
```
internal/gateway/
├── router/    # 路由注册（每个服务一个 *_routes.go）
├── request/   # REST 入参结构（json/form 标签 + binding 校验）
├── handler/   # HTTP handler（参数解析、调用 client、写响应）
└── client/    # 各下游 gRPC 客户端封装
```
- `router` 只注册路由，`request` 只定义入参，`handler` 只做适配，`client` 只封装 gRPC 调用。
- 禁止在网关层写业务逻辑、直接操作数据库。

---

## 2. 依赖方向约束（核心红线）

### 2.1 合法依赖方向（只允许向下/向内）
```
handler ──> service ──> entity
                │
                └──> dto（service 消费 Command、产出 DTO）
repo ──> entity（实现接口）+ model（持久化结构）
gateway.handler ──> gateway.client ──> 各服务 gRPC
```

### 2.2 禁止的依赖方向
- ❌ 循环 import（任何两个包相互 import）。
- ❌ 上层包被下层依赖：`entity` 不得依赖 `service`/`repo`/`handler`；`model` 不得依赖 `entity`；`dto` 不得依赖 `service`。
- ❌ `service` 直接依赖 `repo` 具体实现（必须依赖 `entity.Repository` 接口）。
- ❌ `entity` 依赖 `gorm` / `mongox` / `proto`（保持领域纯净）。
- ❌ 任意服务 `internal/a` 直接 import `internal/b` 的私有实现（跨服务只能通过 gRPC / 事件）。

### 2.3 跨服务/跨模块调用约束
- 服务间调用必须走 gRPC（`api/gen` + `client` 封装），禁止直接读对方库表、调对方私有方法。
- 跨模块事件协作必须走 `pkg/contracts` 定义的事件契约（可靠事件表 + 幂等）。
- 调用第三方/其它服务**必须走统一封装客户端**（`pkg/*x`：`grpcx`/`httpx`/`redisx`/`mongox`/`mysqlx`/`postgresx`/`kafkax`/`rocketmqx`/`rabbitmqx`/`esx`/`nacosx`/`filex` 等）。
- **禁止裸写 HTTP 请求**（禁止 `net/http` 直接 `http.Get/Post` 调第三方）；必须用 `pkg/httpx` 或对应客户端。

---

## 3. 代码生成落位约束（AI 生成代码必须放在正确位置）

AI 生成代码时，按以下规则决定文件位置，禁止自创目录：

| 要生成的代码 | 必须放在 |
|--------------|----------|
| 领域实体/规则 | `internal/{服务名}/entity/` |
| Gorm/Mongo 模型 | `internal/{服务名}/model/` |
| 入参 Command / 出参 DTO | `internal/{服务名}/dto/` |
| 用例编排 | `internal/{服务名}/service/` |
| 仓储实现 | `internal/{服务名}/repo/` |
| gRPC 适配 | `internal/{服务名}/handler/` |
| REST 路由 | `internal/gateway/router/` |
| REST 入参 | `internal/gateway/request/` |
| HTTP handler | `internal/gateway/handler/` |
| 下游客户端封装 | `internal/gateway/client/` |
| 跨模块事件契约 | `pkg/contracts/`（需人工评审） |
| 可复用公共组件 | `pkg/`（对应子包） |

- 新增服务必须复用 `bw-cli service` / 现有服务目录模板，保持六层结构一致。
- 禁止把 `repo` 实现塞进 `service`、把 `handler` 逻辑塞进 `router` 等越位行为。

---

## 4. 禁止行为

- ❌ 私自修改框架目录结构、新增非标准目录、重命名既有分层目录。
- ❌ 循环 import、上层依赖下层、跨服务直接 import 私有实现。
- ❌ `service` 依赖 `repo` 具体类型而非接口。
- ❌ 裸写 `net/http` 调用第三方/其它服务，绕过统一封装客户端。
- ❌ 在 `entity` 引入数据库/协议依赖，污染领域层。
- ❌ 在 `handler` 写业务、在 `router` 写业务、在 `cmd` 写业务。
- ❌ 跨模块直接读对方数据库表（应走 gRPC / 事件契约）。

---

## 5. 交付自查清单

- [ ] 代码是否落在正确的分层目录
- [ ] 依赖方向是否符合 `handler → service → entity`、`repo → entity/model`
- [ ] 是否引入了循环 import 或上层依赖下层
- [ ] 跨服务/第三方调用是否走统一封装客户端
- [ ] 是否破坏了既有目录结构或自创了目录
- [ ] 领域层（entity）是否保持纯净（无 gorm/bson/proto 依赖）
