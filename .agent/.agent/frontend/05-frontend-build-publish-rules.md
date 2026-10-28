# 规则 5：前端工程化与发布规范

> 前置：先加载 `microservice/01-core-agent-rules.md`；提交 / PR 流程遵守 `management/01-git-commit-rules.md`。
> 适用场景：依赖管理、构建配置、环境变量、代码质量门禁、测试与发布。

---

## 1. 依赖管理

- 包管理统一 **pnpm**，使用 `pnpm-lock.yaml` 锁定版本；MUST NOT 混用 npm / yarn 改锁文件。
- 新增 / 升级依赖必须说明理由并经人工确认；AI 不擅自加依赖（高危域，见规则 3）。
- 运行时依赖进 `dependencies`，构建 / 开发期进 `devDependencies`；MUST NOT 装错位置。
- 组件库（Vant / Element Plus）按需引入，禁止全量引入拖体积。

## 2. 构建配置（Vite）

- 构建配置只允许人工修改或人工确认后修改：`vite.config.ts`、`tsconfig.json`、`package.json` scripts。
- 多环境文件：`.env.development` / `.env.production` 等；环境变量统一 `VITE_` 前缀。
- **MUST NOT 把任何密钥 / 内网地址放进前端环境变量**（前端变量随产物暴露即泄露）；敏感凭据走后端代理。
- dev 代理只在开发环境配置，MUST NOT 把内网代理地址打进生产构建。
- 构建产物输出路径与 CDN 前缀按项目约定，MUST NOT 手改 `base` 路径。

## 3. 代码质量门禁（提交前 MUST 全过）

- `pnpm lint`（ESLint + 项目规则）
- `pnpm type-check`（`tsc --noEmit`）
- `pnpm test`（Vitest 单测 / 组件测试）
- 提交前格式化（Prettier 或 lint-staged 自动格式化）
- 门禁不过不得声称"完成"；MUST NOT 用 `--no-lint` / 跳过检查的方式交付。

## 4. 测试

- 纯函数 / 工具 / store 逻辑写单测（Vitest）。
- 关键页面（登录、下单、支付、权限）附组件测试或可复现的手工验证步骤。
- 测试文件命名 `xxx.spec.ts`，与源码同目录或 `__tests__`（按仓库现状统一一种）。
- 涉及金额、权限、竞态的改动 MUST 附测试或验证说明。

## 5. 发布流程

- 发布走 CI/CD：MR → lint / type-check / test → build → 部署；AI MUST NOT 手动触发生产发布。
- 版本号遵循语义化版本（`major.minor.patch`），changelog 同步更新。
- 生产产物可回滚：保留上一版本产物 / 镜像；发布后验证关键链路（登录、下单、支付回调）。
- 缓存策略：带 hash 的静态资源长缓存，`index.html` 不缓存；发布后刷新 CDN 缓存需人工确认。

## 6. AI 前端交付自查清单

- [ ] `lint` / `type-check` / `test` 全过
- [ ] 未新增依赖（或已人工确认）
- [ ] 环境变量 / 代理未泄露任何敏感配置
- [ ] 本地 `pnpm build` 通过
- [ ] 关键链路验证说明齐全（怎么跑、期望看到什么）
- [ ] 调试代码、`console.log`、临时文件已清理
