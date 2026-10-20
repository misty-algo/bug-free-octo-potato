# git-agent.md — Git 与 GitHub 提交规范

> 前置：先加载 `core-agent.md`；提交前同时过一遍 `ai-agent.md` 的自检清单。
> 适用场景：commit / 分支 / push / PR / 处理冲突的一切时刻。

---

## 1. 分支约定

- `main` 是受保护分支，**MUST NOT 直接 push**，所有改动走「分支 + PR」。
- 每人一条长期域分支：`dev/p1-passenger`、`dev/p2-driver`、`dev/p3-pricing`、`dev/p4-order-dispatch`、`dev/p5-settlement`、`dev/p6-risk`。
- 功能分支从自己的域分支切出，命名：`<type>/<功能编号>-<简述>`，示例：`feat/C02-dispatch-ranking`、`fix/E01-payment-idempotent`。
- 功能分支完成后合回自己的域分支；由负责人统一把域分支合入 `main`。**MUST NOT 直接把别人的域分支合进自己的分支**（口径冲突只应由人裁决）。

## 2. 提交信息规范

格式（Conventional Commits + 域 scope）：

```
<type>(<scope>): <中文简述>

[可选正文] 对应功能编号 / 改了哪些集合 / 为什么这么改
```

- `type`：`feat` / `fix` / `refactor` / `test` / `docs` / `chore` / `perf`
- `scope`：自己的域或集合名，如 `order`、`pricing`、`payment`、`dispatch`、`risk`
- 示例：`feat(order): 实现 C02 派单候选司机召回与排序`

MUST NOT 使用 `更新代码`、`fix`、`提交`、`修改 bug` 这类无信息量的 message。一次提交说不清一件事，就拆成两次提交。

## 3. 提交粒度

- 一个提交 = 一件事：一个功能编号的一个子步骤，或一次独立修复。
- MUST NOT 把「新功能 + 重构 + 格式化 + 依赖升级」混进同一个提交 / PR（出问题无法定位是哪个改动生效）。
- proto 改动产出的 `api/gen/` 生成代码与手写代码分开提交，并在描述里注明"含 proto 重新生成"。

## 4. 提交前必须完成

- `make test` 通过；改了 proto 的必须跑过 `make proto` 且**生成产物一并提交**。
- 跑完自检清单（`ai-agent.md`）并逐条回答（结果写进 PR 描述）。
- **确认没有敏感信息入库**：`.env`、含真实密钥的 `configs/*`、AK/SK、带密码的数据库连接串、测试用的真实手机号/身份证。配置类只提交 `.env.example` / `env.example` 占位，**MUST NOT 提交真值**。
- ⚠️ 一旦误提交密钥：**立刻报告负责人并轮换密钥**——只删文件没用，git 历史里仍然存在。

## 5. AI 的 Git 权限边界（重要）

**AI MUST NOT：**

1. ❌ 自行执行 `git push`（任何分支，尤其 `main`）。是否 commit / push 永远由人决定；AI 最多**生成** commit message 供人使用。
2. ❌ 执行 `git push --force` / `--force-with-lease`、`git reset --hard`、`git checkout .`、删除分支、改写已推送的提交或清理历史。
3. ❌ 绕过 CI 与校验：`--no-verify`、`--skip-worktree`、伪造签名、强行跳过 lint / test。
4. ❌ 为了"省事"修改 `.gitignore` 把不该提交的文件藏起来（例如把 `.env` 加进忽略以通过检查）。发现 `.gitignore` 有问题 → 报告给负责人。
5. ❌ 自行决定 rebase / merge 冲突的取舍。AI 应给出方案 + 每个冲突文件的取舍理由，**由人执行**——只有人知道哪个业务口径是对的。

## 6. PR 描述模板

```markdown
## 功能编号
C02 派单候选司机召回与排序

## 改了什么
- internal/dispatch/service/…（召回分层 3/5/8km）
- internal/dispatch/repo/…（新增候选查询）

## 影响的集合
- dispatches（新增）
- orders（只读，未改结构）

## 自检清单（见 ai-agent.md）
- [x] 只动了自己的域
- [x] 金额全部分（本单不涉及）
- [ ] ……（未通过的写清原因）

## 需要跨域确认的点
与第 2 人确认派单卡片字段口径
```

## 7. 仓库卫生

- **不提交**：`node_modules/`、`dist/`、`logs/`、`*.log`、覆盖率产物、IDE 目录（`.idea/` / `.vscode/` 建议进 `.gitignore`）。
- **必须提交**：`api/gen/` 生成代码（保证 CI 可构建）、数据库迁移 / 初始化脚本、`Agent.md`、`Agent约束规则/` 全部子文件与四份业务文档（它们是团队 SSOT，MUST NOT 被 `.gitignore` 排除；若发现被排除，报告负责人，AI 不得自行改 `.gitignore`）。
