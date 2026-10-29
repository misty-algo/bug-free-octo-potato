# core-agent.md — 项目定位 · SSOT · 技术栈 

> 前置：本文件是**人人必读**的核心约束。任何成员、任何场景，开工前必须先加载本文件。

---

## 0. 最小加载清单（按场景选读，避免全量灌入上下文）

| 场景 | 必读文件 |
|---|---|
| 一切任务开始前 | 本文件（core-agent.md） |
| 写任何 Go 代码 | + code-agent.md |
| 任何落库 / 读库 / 模型定义 | + data-agent.md |
| 订单 / 计价 / 账单 / 支付 / 钱包 / 派单 / 营销核心链路 | + business-agent.md |
| 提交 / PR / 处理冲突 | + git-agent.md |
| 全部 AI 行为底线与自检 | + ai-agent.md |

规则冲突优先级：business-agent > data-agent > code-agent > core-agent；本文件为 SSOT 底座，其余文件不得与它矛盾。

---

## 1. 项目一句话

聚合型网约车平台（对标花小猪）：以「低价 + 社交裂变」为增长手段，连接乘客出行需求与司机运力供给，平台负责定价、调度、支付担保、安全与客服兜底。形态：乘客端 + 司机端 + 运营管理端 + 服务端。

## 1.5 业务文档索引（SSOT）

所有业务口径的最终定义以 `docs/` 目录下编号文档为准，AI 动手前必须先按需查阅：

| 编号 | 文档 | 用途 | 何时必读 |
|---|---|---|---|
| 01 | 项目总体设计 | 产品定位、架构总览 | 开工前 |
| 05 | 业务定义 | 业务编号、功能编号定义 | 实现具体功能前 |
| 06 | MongoDB 数据字典（补齐版） | 集合 / 字段 / 索引唯一 schema | 任何落库 / 读库前 |

路径以仓库实际 `docs/` 目录为准；找不到时先 `git ls-files` 检索文件名，仍找不到则停下提问。


---


## 2. 技术栈基线（以真实仓库代码为准）

服务端基于 **bw-cli 脚手架**（参考仓库：`bug-free-octo-potato`）：

- **形态**：Gin HTTP 网关 + 各业务域 gRPC 微服务，多进程部署。
- **语言**：Go（go.mod 声明为准）。
- **存储**：GORM（MySQL/PostgreSQL，结构化业务数据）+ MongoDB（文档类数据，mongo-driver）+ Redis（缓存 / 派单锁 / 幂等键 / 验证码）。
- **MQ**：Kafka（`kafkax`）/ RocketMQ（`rocketmqx`），事件解耦。
- **配置**：Viper + Nacos（`configs/config.yaml`、`configs/nacos.yaml`）。
- **日志**：zap + lumberjack（`pkg/logger`）。
- **前端**：乘客端/司机端 H5（Vue3 + Vant），管理端（Element Plus），后续单独立项。

**统一基建包（MUST 复用，禁止重复造轮子）**：

| 包 | 用途 |
|---|---|
| `pkg/errors` | 统一错误体系：`Kind/Code/Message/Cause`，含 gRPC 编解码 |
| `pkg/httpx` | 统一响应体（`{"request_id","error":{...}}`） |
| `pkg/logger` | zap 封装，按服务按日切分 |
| `pkg/config` | Viper + Nacos 加载 |
| `pkg/mongox` / `pkg/mysqlx` / `pkg/postgresx` | 数据库连接 |
| `pkg/redisx` | Redis + 分布式锁 |
| `pkg/kafkax` / `pkg/rocketmqx` | MQ 生产/消费 |
| `pkg/filex` | 统一上传（多云存储适配） |

AI 需要某个基础能力时，第一步先查 `pkg/` 下是否已存在；**存在就 import，不存在才允许新建，且新建公共包必须在 PR 描述里说明理由**。

---

## 3. 关键口径速查（详细定义一律以 `06` 为准）

| 口径 | 值 |
|---|---|
| 订单状态 | `PENDING / NO_DRIVER / ACCEPTED / ARRIVED / IN_SERVICE / COMPLETED_UNPAID / UNPAID / COMPLETED / CANCELLED / SUSPENDED` |
| 取消责任方 | `passenger / driver / system / cs` |
| 订单业务类型 | `instant / reserve / airport / proxy / carpool / enterprise`（预留扩展，不实现） |
| 产品线 | `express(快车) / discount(特惠) / premium(优享) / luxury(专车) / carpool(拼车) / taxi(出租车)` |
| 司机生命周期 | `registered → auditing → audited → training → active → suspended/frozen/banned/quit` |
| 司机在线状态 | `offline / online / listening / in_service / resting` |
| 钱包流水类型 | `order_income / reward / penalty / withdraw / withdraw_fail / recharge / refund / adjust / subsidy` |
| 风控决策 | `pass / review / reject` |
| 工单 SLA | 首响 ≤ 15 分钟；普通 ≤ 24h；安全类 ≤ 30 分钟（安全类自动 `urgent`） |
| 性能基线 | 下单 P99 < 300ms；派单决策 < 200ms；查询 < 500ms |
| 派单超时 | 单轮推送 15s，超时自动转派 + 扩圈 |
| 疲劳驾驶 | 连续在线 ≥ 240 分钟强制休息 20 分钟 |
