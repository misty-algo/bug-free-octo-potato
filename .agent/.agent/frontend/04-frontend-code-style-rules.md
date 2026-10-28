# 规则 4：前端代码工程规范（Vue3 + TypeScript）

> 前置：先加载 `microservice/01-core-agent-rules.md`；不确定 AI 能否碰某段代码时读 `frontend/03-frontend-ai-dev-rules.md`。
> 适用场景：写任何前端代码（页面 / 组件 / 样式 / 状态 / 路由 / 请求）。
> 来源：项目真实代码约定（技术栈基线见 microservice/01-core-agent-rules.md §2），不是泛泛最佳实践。

---

## 1. 技术栈基线（以真实仓库为准）

- **框架**：Vue 3 组合式 API + `<script setup lang="ts">`，TypeScript 开启 strict。
- **构建 / 包管理**：Vite + pnpm（锁文件 `pnpm-lock.yaml`）。
- **组件库**：乘客端 / 司机端 H5 用 Vant，管理端用 Element Plus，按需引入。
- **状态**：Pinia；**请求**：axios 统一封装在 `src/api`（拦截器、错误码映射）。
- **样式**：SCSS + CSS 变量主题 token。

## 2. 目录与分层

```
src/
├── api/          # 接口定义与请求封装（按域分文件，出入参类型与后端契约一致）
├── assets/       # 静态资源（图片 / 字体），MUST NOT 放业务代码
├── components/   # 通用组件（跨页面可复用；页面私有组件不放在这里）
├── composables/  # 组合式函数（hooks），承载可复用逻辑
├── layouts/      # 布局组件
├── router/       # 路由表 + 路由守卫
├── stores/       # Pinia 状态（按域拆分）
├── styles/       # 全局样式、CSS 变量、mixin
├── types/        # 全局类型定义（业务类型尽量与 api 层就近定义）
└── views/        # 页面级组件（一页一目录：index.vue + 页面私有组件）
```

- 依赖方向：`views → components / composables → api / stores / types`；页面 MUST NOT 直接裸发请求（统一走 `api/` 封装）、MUST NOT 绕过 action 直改全局 store 状态。
- 页面私有组件放 `views/<页>/components/`，可复用组件提升到 `components/`。

## 3. 命名规范

| 对象 | 约定 | 示例 |
|---|---|---|
| 文件 / 目录 | kebab-case | `order-list/`、`user-card.vue` |
| 组件名 | PascalCase，多词组成（禁止单字母） | `UserCard.vue` |
| 变量 / 函数 | camelCase | `getUserList` |
| 顶层常量 | UPPER_SNAKE_CASE | `MAX_PAGE_SIZE` |
| 类型 / 接口 | PascalCase，前缀风格全仓统一一种 | `OrderInfo` / `IOrderInfo` |
| props / emit | camelCase；事件用 `update:xxx` / 语义化动词 | `@submit`、`update:modelValue` |
| store | `useXxxStore` | `useUserStore` |
| CSS 类名 | BEM：`block__element--modifier` | `order-card__title--active` |

## 4. Vue3 组合式 API 约束

- 新代码一律 `<script setup lang="ts">`，MUST NOT 用 Options API 写新代码。
- `defineProps` / `defineEmits` 必须带类型；props 派生值用 `computed` / `watch`，MUST NOT 直接改 props。
- 单文件职责单一：模板超长（约 > 200 行）应拆子组件；逻辑复杂拆 composables。
- `v-for` 必须带稳定 key（业务 id 字段，禁止用 index 作 key）。
- 数据请求：按路由进入需求放 `onMounted` 或路由守卫按需，MUST NOT 在 setup 顶层直接 `await` 阻塞渲染（Suspense 场景除外）。
- 组件对外事件语义化（`@submit` / `@change` / `@select`），禁止把业务操作裸绑在 `@click` 上透传。

## 5. TypeScript 约束

- tsconfig 开启 strict；新代码禁止 `any`（确需时加豁免注释并交人工评审）。
- 接口出入参类型与后端契约一一对应，禁止用宽泛 `object` / 双重断言糊弄。
- 后端枚举值在前端建映射表（中文文案 / 颜色 / 状态标签统一出口），禁止散落魔法值。
- 金额字段统一 `number`（单位分），展示统一走 `formatFen` 转元，MUST NOT 在页面手写转换逻辑。
- 请求响应字段命名遵循后端契约（`camelCase` 映射由 api 层统一处理，页面不感知原始字段差异）。

## 6. 样式规范

- 组件 / 页面样式一律 `<style scoped lang="scss">`；全局样式只进 `styles/`，MUST NOT 在组件里写全局选择器。
- 颜色、字号、间距、圆角一律走主题 token（CSS 变量），禁止组件内硬编码色值 / 像素。
- 布局用 flex / grid，禁止滥用绝对定位；H5 适配按项目约定（rem / viewport），禁止写死宽度。
- 禁止行内 `style`（动态样式用 CSS 变量绑定）。
- `z-index` 统一登记管理，禁止随手写 `9999`。
- 深色模式 / 主题切换一律走 token 切换，禁止各组件各自判断。

## 7. 状态管理（Pinia）

- store 按域拆分，MUST NOT 建一个巨型 root store。
- 只存跨页共享、需要响应式的状态；页面局部状态用 `ref` / `reactive` 即可。
- 异步请求放 action（统一入口）；请求竞态用 AbortController / 请求序号防止旧响应覆盖新数据。
- 持久化（登录态 / 偏好）显式声明 storage 策略；MUST NOT 把敏感信息明文持久化到 localStorage。

## 8. 性能约束

- 列表 / 长页用虚拟滚动或分页；图片懒加载 + 尺寸占位，避免布局抖动。
- 高频事件（滚动 / 输入 / resize）防抖节流；组件卸载时清理定时器与订阅。
- 路由按需加载（动态 import）；大体积组件 / 图表库分包。
- 减少重复渲染：`v-if` / `v-show` 按场景选用、`computed` 缓存派生值、模板内避免复杂函数调用。

## 9. 注释

- 代码修改处附**中文注释说明"为什么"**（业务原因 / 规避什么坑），MUST NOT 只写"修改了 xx"。
- 导出组件 / 工具函数写 JSDoc 简注（用途、参数、返回）。
