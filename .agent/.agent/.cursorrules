# AI 约束入口 · 花小猪打车（规则库）

> 你在 `bug-free-octo-potato`（bw-cli 脚手架）仓库里干活；**本目录就是规则库本体**（`C:\Users\91526\Desktop\ai开发规则`）。
> 本文件是约束的**地图**，不是约束本身——读完它你只知道「该去哪读」：索引在 `00-index.md`，具体条款在 `microservice/`、`frontend/`、`management/` 三个目录下。
> 本文件存了**三个同内容副本**：`AGENTS.md`（通用标准，Codex / 多数新工具认）、`CLAUDE.md`（Claude Code 认）、`.cursorrules`（Cursor 认）。三者必须**字节一致**，改一处立刻同步另外两个（同步命令见 §8）。
> 铁律：**读不懂、对不上、需求含糊 → 停下来问人，MUST NOT 凭经验脑补一个实现糊过去。**
> **规则已迁移（v4）**：唯一有效副本在本目录；旧 `花小猪打车/代码内置约束/Agent约束规则/` **降级为只读归档，不再更新**，冲突一律以本目录为准。规则按职责域分为微服务 / 前端 / 管理三类，详见 `00-index.md`。

---

## 0. 开工前的三个动作（MUST，按顺序）

1. **读完本文件**（约 3 分钟，不要跳）。
2. **读台账**：打开 `management/02-ai-dev-ledger.md`，看 §0「当前状态摘要」——你在接谁的手、做到第几步、下一步是什么、有没有待确认问题。有「待确认问题」就先停下确认，**不要脑补继续写**。
3. **按 §1 路由表按需加载**：只加载本次任务真正需要的约束文件，MUST NOT 一次性把十六份全灌进上下文（上下文越长，早期约束越容易被稀释掉）。

---

## 1. 路由表：遇到什么 → 去读哪个文件

| 你正在做 / 正要问的事 | 去读这个文件 |
|---|---|
| 刚接手、不确定技术栈、状态 / 枚举该叫什么、该 import 哪个 `pkg/` | `microservice/01-core-agent-rules.md` |
| 定义模型、建表建集合、加字段加索引 | `microservice/01-data-model-rules.md` |
| 写 SQL / Mongo 查询、GORM、事务边界 | `microservice/02-database-orm-sql-rules.md` |
| 动订单状态机、计价、账单、支付、钱包、派单、优惠券 | `microservice/01-business-core-rules.md` |
| 写 Go 代码：分层、命名、错误码、日志、测试、注释 | `microservice/02-go-code-style-rules.md` |
| 不确定这段代码 AI 能不能碰、高危域边界在哪 | `microservice/01-ai-dev-code-rules.md` |
| 定义 / 修改接口、路由、出入参、错误码、响应体 | `microservice/01-api-contract-rules.md` |
| 服务分层与依赖方向、gRPC 调用链 | `microservice/01-architecture-layer-rules.md` |
| 写任何前端代码（页面 / 组件 / 样式 / 状态 / 请求） | `frontend/04-frontend-code-style-rules.md`（行为边界见 `frontend/03-frontend-ai-dev-rules.md`） |
| 前端依赖 / 构建 / 环境变量 / 发布 | `frontend/05-frontend-build-publish-rules.md` |
| 准备 commit / 发 PR / 处理冲突 / 写提交信息 | `management/01-git-commit-rules.md` |
| 交付前自检、不确定某件事 AI 到底能不能做 | `management/01-ai-behavior-rules.md` |
| 开工前扫坑、事故复盘 | `management/01-lessons-learned-rules.md` |
| 开工前恢复上下文、每个改动点收尾 | `management/02-ai-dev-ledger.md` |

**规则冲突优先级**：`microservice/01-business-core-rules.md 业务不变量` > `microservice/01-data-model-rules.md 数据模型硬约束` > `microservice/02-go-code-style-rules.md Go 工程规范` > `microservice/01-core-agent-rules.md 技术栈底座`。最后一份是 SSOT 底座，其余文件不得与它矛盾；发现矛盾 → 报告负责人，MUST NOT 自行取舍。

**字段级定义**（某个字段叫什么、什么类型、要不要索引）一律以**仓库内实际的数据字典文件**为唯一依据，不以代码现状为准。定位方式：`git ls-files | grep -i 数据字典`；找不到就停下问人，MUST NOT 照现有 model 反推。

---

## 2. 七条不可协商红线（任何情况都适用）

1. **金额全链路用「分」为单位的整数**（字段名以 `Fee / Amount / Price / Income` 结尾），MUST NOT 用 float/double 表示钱。计价 / 优惠 / 分摊一律**向下取整到分**（floor），尾差记入平台侧，MUST NOT 四舍五入。
2. **只动自己域的代码与集合**；要别人的数据走对方的 service / gRPC，MUST NOT 直连别人的表、MUST NOT 改别人的集合。
3. **schema 以数据字典为唯一依据**；要新增集合、改字段名 / 类型 / 索引，必须先提变更说明（写明业务理由）→ 人工确认并更新字典 → 才允许改代码。代码先行的 schema 变更一律打回。
4. **状态机与资金守恒是事故级红线**：订单状态变更走**乐观锁 + `order_status_logs` 留痕**，MUST NOT 读-改-写；每一单必须满足 `乘客实付 = 平台佣金 + 司机收入 + 平台补贴`，改任何涉钱代码都要说明怎么维持这个等式。
5. **AI 不碰仓库钥匙**：MUST NOT 自行 `git push` / 强推 / `reset --hard` / 改写历史 / `--no-verify` 绕 CI，MUST NOT 为了过检查去改 `.gitignore`。是否 commit / merge / rebase 永远由人决定。
6. **AI 不改约束本身**：MUST NOT 修改本文件与三个目录（`microservice/`、`frontend/`、`management/`）下任何规则文件。发现问题 → 报告负责人，人工裁决。
7. **不许假实现**：MUST NOT 空 TODO、假数据、写死的 mock 冒充"调通了三方"；三方未配置密钥时走 Mock Adapter 且必须标 `// ADAPTER: real|mock`，接口契约与真实实现一致。**「本期不做」的业务**（顺风车 / 自动驾驶 / 境外 / 自营车队）只返回明确"暂不支持"，MUST NOT 写任何实现。

---

## 3. 台账：每完成一个可验证的改动点，立刻写一条

这是本项目最容易漏、也最容易让你"上下文一丢就断片"的一条。

- 位置：`management/02-ai-dev-ledger.md` §2，**最新记录放最上面**，模板见该文件 §1。
- 一条记录必须写清：类型 / 改动文件（带路径）/ 动的集合 / **测试方式与结果（怎么跑 + 期望看到什么）** / 是否提交 / 遗留与下一步。
- 时机：**每个接口或可独立验证的改动点做完就写**，不要等整个功能收尾才补。
- 上下文变长、切换会话、收工前：更新该文件 §0「当前状态摘要」。
- 判断标准：**只看台账就能接着干活**。写不出"怎么跑、期望什么结果"，说明这个改动点你根本没验证过。

---

## 4. 交付前自检（逐条回答，答不出 = 没做完）

- [ ] 涉及金额的字段全部是分（整数）？全链路没有 float？舍入是 floor 吗？
- [ ] 改状态机了吗？走乐观锁 + 状态日志了吗？流转关系合法吗？
- [ ] 动钱包 / 支付 / 券库存了吗？是原子操作吗？幂等键（`{bizType}:{orderId}:{action}` 唯一索引）在吗？
- [ ] 手机号 / 身份证 / 银行卡：加密（`Enc`）+ 哈希（`Hash`）+ 脱敏（`Mask`）三件套齐了吗？日志和响应里只有 Mask 吗？
- [ ] 复用 `pkg/` 既有能力了吗（errors / httpx / logger / redisx / kafkax / filex …）？引入新依赖说明理由并经确认了吗？
- [ ] 错误走 `pkg/errors`、响应走 `pkg/httpx`、日志走 `pkg/logger` 吗？错误码是按域段分配的新号吗？
- [ ] 单元测试补了吗？`make test` 真的跑过并贴出结果了吗？（没跑过 MUST NOT 说"完成了"）
- [ ] 改动处写了**中文"为什么"**注释吗？proto 改过的话跑过 `make proto` 且没手改 `api/gen/` 吗？
- [ ] 台账 §2 追加了对应记录吗？
- [ ] 交付三件套齐了吗：**可复现的验证方式 + 本清单逐条答案 +（涉及提交时）按模板填好的 PR 描述**？

---

## 5. 卡住了怎么问（四要素，缺一不可）

> 问题点 / 影响范围 / 候选方案（≥ 2 个）/ 我的倾向

MUST NOT 只抛一句"这里不清楚"。同一时刻**只改一个点**，改完可验证再动下一个；MUST NOT 多文件大换血式重构混在一次交付里（出问题无法定位是哪个改动生效）。

---

## 6. Git 摘要（细则见 `management/01-git-commit-rules.md`）

- `main` 受保护，走「域分支 → 功能分支 → PR」；功能分支名 `<type>/<功能编号>-<简述>`。
- 提交信息：`<type>(<scope>): <中文简述>`，MUST NOT 写"更新代码""修改 bug"这类无信息量的 message；一个提交只说一件事。
- 提交前：`make test` 过 + 自检清单逐条答完 + 密钥扫描（`gitleaks detect`）。`.env`、真密钥、真手机号 MUST NOT 入库。
- 改代码前先确认自己的域分支没有他人新提交；发现别人已改你要改的文件 → 停下报告，MUST NOT 强行覆盖。

---

## 7. 规则文件清单（十六份 · 三类目录）

```
ai开发规则/
├── 00-index.md                        # 索引：场景 → 必读清单（先读它）
├── microservice/                      # 微服务（后端服务端）规则
│   ├── 01-core-agent-rules.md         # 项目定位、SSOT 索引、技术栈基线、pkg/ 清单（人人必读底座）
│   ├── 01-ai-dev-code-rules.md        # AI 能做什么 / 禁止碰哪些高危域
│   ├── 02-go-code-style-rules.md      # 分层与依赖方向、命名、错误码、日志、测试、注释
│   ├── 01-api-contract-rules.md       # 路由、出入参、错误码、统一响应体
│   ├── 01-architecture-layer-rules.md # 网关 / gRPC 服务分层与依赖方向
│   ├── 01-data-model-rules.md         # 金额/敏感三件套/GeoJSON/UTC/枚举/软删除/审计字段/乐观锁
│   ├── 02-database-orm-sql-rules.md   # GORM 用法、SQL 红线、事务边界
│   ├── 01-business-core-rules.md      # 订单状态机、计价快照与舍入、资金守恒、支付幂等
│   └── 01-backend-dev-plan.md         # 72 接口 / 37 表 / 18 模块分步施工任务书
├── frontend/                          # 前端（Vue3+TS）规则
│   ├── 03-frontend-ai-dev-rules.md    # AI 前端行为约束：高危域边界 / 禁止行为 / 自查
│   ├── 04-frontend-code-style-rules.md # Vue3+TS 工程规范：目录/命名/API/样式/Pinia/性能
│   └── 05-frontend-build-publish-rules.md # 依赖/Vite/环境变量/质量门禁/发布
└── management/                        # 管理（协作与过程）规则
    ├── 01-git-commit-rules.md         # 分支、提交信息、PR 模板、AI 权限边界
    ├── 02-ai-dev-ledger.md            # 台账：当前状态摘要 + 改动记录 + 上下文恢复
    ├── 01-lessons-learned-rules.md    # 经验教训沉淀：开工前扫坑、事故复盘
    └── 01-ai-behavior-rules.md        # AI MUST / MUST NOT + 交付前自检清单
```

---

## 8. 三份入口怎么同步 / 怎么验证真的加载了

**同步**（改完一份立刻执行，保持字节一致）：

```bash
cd "C:/Users/91526/Desktop/ai开发规则"
cp AGENTS.md CLAUDE.md && cp AGENTS.md .cursorrules
```

**你（人）验证 AI 是否真读到**——不要相信"我已了解约束"，开工前让它回答：

> 1. 这个项目的技术栈和统一基建包分别是哪些？
> 2. 七条红线分别是什么？
> 3. 现在我要给订单表加一个字段，你应该怎么做？
> 4. 台账文件在哪，什么时候写？

答不上来或答错 = 入口没被加载，先解决加载再开工，否则后面全是返工。

---

*本目录是团队共享 SSOT。规则文件 MUST NOT 被 `.gitignore` 排除；发现被排除 → 报告负责人，AI 不得自行改 `.gitignore`。*
