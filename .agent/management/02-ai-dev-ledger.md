---
AIGC:
    Label: "1"
    ContentProducer: 001191440300708461136T1XGW3
    ProduceID: 679f6ec560e3134b325d0d1a8502c6f1_d5c0e6dfd10f11f18f85525400de85a5
    ReservedCode1: FTcoSdw8bKQ1I0/9PCnO60a3dX21stl77HKmbc2+fCcRqo8J8HcA9auxtz+XWgIi1/zLhOn0n5vDGrghuKHXVaVO51DDG6WDbVV3QQmQdFcJGScHfyeD1tnVeQtIDPhLomTayxbjKrgqhpzRBzin9Q6Wv63Bptzmo2rfjTvuyJAv8nB0dCjK8myT4cM=
    ContentPropagator: 001191440300708461136T1XGW3
    PropagateID: 679f6ec560e3134b325d0d1a8502c6f1_d5c0e6dfd10f11f18f85525400de85a5
    ReservedCode2: FTcoSdw8bKQ1I0/9PCnO60a3dX21stl77HKmbc2+fCcRqo8J8HcA9auxtz+XWgIi1/zLhOn0n5vDGrghuKHXVaVO51DDG6WDbVV3QQmQdFcJGScHfyeD1tnVeQtIDPhLomTayxbjKrgqhpzRBzin9Q6Wv63Bptzmo2rfjTvuyJAv8nB0dCjK8myT4cM=
---

# ai开发台账.md — AI 开发过程外部记忆（防上下文丢失）

> 本文件是 AI 的**外部记忆**：AI 上下文过长被截断 / 丢失后，读本文件即可恢复"进行到哪、下一步做什么、最近改了什么"。
> 使用规则：**每完成一个可独立验证的改动点，立即追加一条记录**；开工前先读 §0，收工前更新 §0。

---

## 0. 当前状态摘要（开工前读这里，收工前更新这里）

- **进行中的功能**：（示例）C02 派单候选司机召回与排序
- **当前进度**：（示例）第 3/5 步——候选召回 SQL 已完成，待写排序
- **下一步动作**：（示例）实现排序逻辑 + 单元测试
- **待确认问题**：（示例）派单卡片字段口径待与第 2 人确认
- **最近一次测试结果**：（示例）make test 全绿（2026-10-26 14:30）

---

## 1. 记录模板（每完成一个接口 / 一个改动点，按此追加一条）

```markdown
### [YYYY-MM-DD HH:MM] <功能编号或简述>
- **类型**：接口实现 / 修复 / 重构 / 文档 / 其他
- **改动文件**：（绝对或相对仓库根路径，如 internal/dispatch/service/candidate.go）
- **改动集合**：（如 dispatches 新增字段 xxx；未动写"无"）
- **测试方式与结果**：（怎么跑 + 看到什么，如 make test 通过；curl POST /v1/dispatch/recall 返回 200，候选 5 个）
- **是否提交**：未提交 / 已 commit（填 commit 摘要）
- **遗留 / 下一步**：...
```

---

## 2. 台账记录（倒序：最新记录放最上面）

<!-- 从这一行往下追加新记录，最新的放在本注释块之后的第一条 -->

### [2026-10-26 14:30] C02 派单候选司机召回与排序（示例，可删除）
- **类型**：接口实现
- **改动文件**：internal/dispatch/service/candidate.go
- **改动集合**：dispatches（新增）
- **测试方式与结果**：make test 通过；curl POST /v1/dispatch/recall 返回 200，候选 5 个
- **是否提交**：未提交
- **遗留 / 下一步**：待写排序逻辑 + 单测

---

## 3. 写入时机（MUST）

1. 每完成一个接口 / 一个可独立验证的改动点，**立即**追加一条记录，不等整个功能做完。
2. 收工 / 切换会话 / 上下文明显变长时，更新 §0"当前状态摘要"。
3. 记录要保证"只看本文件就能继续干活"：文件写绝对路径，测试写"怎么跑 + 期望什么结果"。
4. 时间一律 `YYYY-MM-DD HH:MM` 本地时间；新记录追加在 §2 最上方。

## 4. 上下文丢失时的恢复流程

1. 先读 §0"当前状态摘要"→ 知道进行到哪、下一步做什么。
2. 再按 §2 台账倒序往回翻 → 找回最近几次改动的细节（文件、集合、测试结果）。
3. 若 §0 有"待确认问题"→ 先停下找人确认，不要脑补继续写。

---

## 5. 与本文件配套的规则

- `ai行为约束.md`：AI 行为底线与自检清单。
- `git提交约束.md`：本文件属于 `Agent约束规则/` 目录，随规则文件一起提交仓库，MUST NOT 被 `.gitignore` 排除；每次提交前把 §2 最新记录同步提交，防止台账本身丢失。
*（内容由AI生成，仅供参考）*
