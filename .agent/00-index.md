# 00-index.md — AI 开发规则 · 唯一入口

> **AI MUST 最先读本文件**，再按 §3「场景 → 必读清单」加载对应规则，禁止跳过索引直接干活。
> **目录编号 = 加载优先级，文件名编号 = 目录内顺序。**
> 本机路径：`C:\Users\91526\Desktop\ai开发规则`

---

## 0. 迁移说明（v4，2026-10-27）

- 规则已从 `花小猪打车/代码内置约束/Agent约束规则/`（七份中文名）**整体迁移**到本目录（十六份）。
- v4 起规则按**职责域**分为三类目录，分开放置：

| 目录 | 定位 | 内容 |
|---|---|---|
| `microservice/` | 微服务（后端服务端）开发规则 | 底座、AI 写码、Go 规范、接口契约、分层架构、数据、业务红线、后端计划 |
| `frontend/` | 前端（Vue3+TS）开发规则 | AI 前端行为约束、前端工程规范、工程化与发布 |
| `management/` | 管理与协作规则 | Git 提交、开发台账、经验教训、AI 行为约束 |

- 三个入口文件（`AGENTS.md` / `CLAUDE.md` / `.cursorrules`）的路由表已指向本文件。
- 对外仍沿用的 agent 名 → 本目录文件对照：

| 旧 agent 名 | 本目录路径 |
|---|---|
| `core-agent.md` | `microservice/01-core-agent-rules.md` |
| `data-agent.md` | `microservice/01-data-model-rules.md` |
| `business-agent.md` | `microservice/01-business-core-rules.md` |
| `code-agent.md` | `microservice/02-go-code-style-rules.md` |
| `git-agent.md` | `management/01-git-commit-rules.md` |
| `ai-agent.md` | `management/01-ai-behavior-rules.md` |

---

## 1. 目录结构

```
ai开发规则/
├── 00-index.md                      ← 本文件（唯一入口）
├── microservice/                    微服务（后端服务端）规则
│   ├── 01-core-agent-rules.md              项目定位 · SSOT · 技术栈（人人必读底座）
│   ├── 01-ai-dev-code-rules.md             AI 写码行为约束
│   ├── 02-go-code-style-rules.md           Go 工程规范
│   ├── 01-api-contract-rules.md            接口契约
│   ├── 01-architecture-layer-rules.md      分层架构
│   ├── 01-data-model-rules.md              数据模型硬约束
│   ├── 02-database-orm-sql-rules.md        数据库 / ORM / SQL
│   ├── 01-business-core-rules.md           核心业务不变量（事故级红线）
│   └── 01-backend-dev-plan.md              后端开发计划 v4.0
├── frontend/                       前端（Vue3+TS）规则
│   ├── 03-frontend-ai-dev-rules.md         AI 前端行为约束
│   ├── 04-frontend-code-style-rules.md     前端工程规范（Vue3+TS）
│   └── 05-frontend-build-publish-rules.md  前端工程化与发布
└── management/                     管理（协作与过程）规则
    ├── 01-git-commit-rules.md               提交 / 协作规范
    ├── 02-ai-dev-ledger.md                  AI 开发台账（外部记忆）
    ├── 01-lessons-learned-rules.md          经验教训沉淀
    └── 01-ai-behavior-rules.md              AI 行为约束 + 交付前自检
```

## 2. 文件清单

### microservice/（微服务 · 9 份）

| 目录 | 路径 | 主题 | 一句话说明 |
|---|---|---|---|
| microservice | `microservice/01-core-agent-rules.md` | 项目定位 · SSOT · 技术栈 | 人人必读底座，含最小加载清单与统一基建包 |
| microservice | `microservice/01-ai-dev-code-rules.md` | AI 写码行为约束 | AI 能做什么 / 禁止碰哪些高危域、交付前自检 |
| microservice | `microservice/02-go-code-style-rules.md` | Go 工程规范 | 目录与分层、命名、错误码、日志、测试、注释 |
| microservice | `microservice/01-api-contract-rules.md` | 接口契约规范 | 路由、出入参、错误码、统一响应体 |
| microservice | `microservice/01-architecture-layer-rules.md` | 分层架构规范 | 网关 / gRPC 服务分层与依赖方向 |
| microservice | `microservice/01-data-model-rules.md` | 数据模型硬约束 | 金额分整数、敏感三件套、GeoJSON、软删除、乐观锁 |
| microservice | `microservice/02-database-orm-sql-rules.md` | 数据库 / ORM / SQL 规范 | GORM 用法、SQL 红线、事务边界 |
| microservice | `microservice/01-business-core-rules.md` | 核心业务不变量 | 订单状态机、计价快照与舍入、资金守恒、支付幂等、派单唯一性 |
| microservice | `microservice/01-backend-dev-plan.md` | 后端开发计划 v4.0 | 72 接口 / 37 表 / 18 模块的分步施工任务书 |

### frontend/（前端 · 3 份）

| 目录 | 路径 | 主题 | 一句话说明 |
|---|---|---|---|
| frontend | `frontend/03-frontend-ai-dev-rules.md` | AI 前端行为约束 | 前端高危域边界、禁止行为、交付自查 |
| frontend | `frontend/04-frontend-code-style-rules.md` | 前端工程规范（Vue3+TS） | 目录分层、命名、组合式 API、TS、样式、Pinia、性能 |
| frontend | `frontend/05-frontend-build-publish-rules.md` | 前端工程化与发布 | 依赖、Vite、环境变量、质量门禁、测试、发布 |

### management/（管理 · 4 份）

| 目录 | 路径 | 主题 | 一句话说明 |
|---|---|---|---|
| management | `management/01-git-commit-rules.md` | 提交与协作规范 | 分支、commit message、PR、AI 权限边界 |
| management | `management/02-ai-dev-ledger.md` | AI 开发台账（外部记忆） | 上下文丢失后的恢复点；每完成一个改动点追加一条 |
| management | `management/01-lessons-learned-rules.md` | 经验教训沉淀 | 已踩过的坑，开工前扫一眼 |
| management | `management/01-ai-behavior-rules.md` | AI 行为约束 | MUST / MUST NOT + 交付前自检清单 |

## 3. 场景 → 必读清单（最小加载，避免灌满上下文）

| 场景 | 必读文件 |
|---|---|
| 一切任务开始前 | `microservice/01-core-agent-rules.md`（SSOT 底座） |
| 写任何 Go 代码 | `microservice/01-core` + `microservice/01-ai-dev-code-rules` + `microservice/02-go-code-style-rules` + `microservice/01-architecture-layer-rules` |
| 写任何前端代码（页面/组件/样式/状态/请求） | `microservice/01-core` + `frontend/03` + `frontend/04` + `management/01-ai-behavior` |
| 前端依赖 / 构建 / 环境变量 / 发布 | `microservice/01-core` + `frontend/03` + `frontend/05` + `management/01-git` |
| 落库 / 读库 / 定义模型 / 写 SQL | `microservice/01-core` + `microservice/01-data-model` + `microservice/02-database-orm-sql` |
| 定义 / 修改接口 | `microservice/01-core` + `microservice/01-api-contract` |
| 订单 / 计价 / 账单 / 支付 / 钱包 / 派单 / 营销 | `microservice/01-core` + `microservice/01-business-core` + `microservice/01-data-model` |
| 提交 / PR / 处理冲突 | `microservice/01-core` + `management/01-git` |
| 交付前自检、不确定 AI 能否做某事 | `management/01-ai-behavior` |
| 开工前恢复上下文、每个改动点收尾 | `management/02-ai-dev-ledger` |
| 接手新会话 / 上下文被截断 | 先读 `management/02-ai-dev-ledger` §0，再按场景加载 |

## 4. 冲突优先级

1. `microservice/01-business-core-rules.md`（业务不变量）> `microservice/01-data-model-rules.md`（数据硬约束）> `microservice/02-go-code-style-rules.md`（Go 工程规范）> `microservice/01-core-agent-rules.md`（底座）。最后一份是 SSOT 底座，其余文件不得与它矛盾。
2. 本目录规则 > 仓库外任何接口文档原话；冲突时以 `microservice/01-business-core-rules.md` / `microservice/01-data-model-rules.md` 为准，并在 `management/02-ai-dev-ledger.md` 记一条待裁决。
3. 发现规则之间矛盾：停下提问，MUST NOT 自行取舍后继续写码。

## 5. AI 行为底线（速记）

- 动手前先读规则，规则没授权的事不做；一次只改一个点，验证通过再动下一个。
- 碰钱、碰状态、碰并发的改动必须带测试；金额一律「分」整数，舍入一律 floor。
- schema 变更必须先报批，代码先行一律打回；敏感信息只输出 Mask 版。
- AI 不碰仓库钥匙（不自行 push / 强推 / 绕 CI），不改约束本身。
- 每完成一个可独立验证的改动点，立即更新 `management/02-ai-dev-ledger.md`。
