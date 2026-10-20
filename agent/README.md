# 方案二：约束内置到代码仓库（给人看的安装说明）

> 这个 README 是**给人看的**，可以不拷进仓库。真正要拷进去的是下面「安装步骤」里的东西。

---

## 一、方案一和方案二有什么区别

| | 方案一（已做好：skill） | 方案二（本文件夹） |
|---|---|---|
| 约束存在哪 | `~/.workbuddy/skills/huaxiaozhu-rules/` | **代码仓库里**，随 git 走 |
| 谁能用 | 只有装了这个 skill 的人 | 任何 clone 仓库的人，开箱即用 |
| 生效范围 | 任何项目、任何目录 | 只在放进去的那个仓库里 |
| 适合场景 | 自己随时随地写码 | **团队协作、六人共享、交给老师检查** |

**两个都留着不冲突**：方案二保证队友 clone 下来就有约束，方案一保证你在别的地方写码时也能加载。

---

## 二、安装步骤

把本文件夹里的**这些**拷到仓库根目录（`bug-free-octo-potato/`）：

```
bug-free-octo-potato/
├── AGENTS.md          ← 总入口（通用标准）
├── CLAUDE.md          ← Claude Code 专用入口（内容与 AGENTS.md 相同）
├── .cursorrules       ← Cursor 专用入口（内容与 AGENTS.md 相同）
└── Agent约束规则/     ← 六份约束
    ├── 技术栈+标准用词.md
    ├── 数据模型约束.md
    ├── 业务约束.md
    ├── go代码规范约束.md
    ├── git提交约束.md
    └── ai行为约束.md
```

命令（把目标路径换成你自己的仓库）：

```bash
cp -r "C:/Users/91526/Desktop/花小猪打车/代码内置约束/." \
      "C:/Users/91526/Desktop/花小猪打车/bug-free-octo-potato/"
```

---

## ⚠️ 三、必做：处理 .gitignore（不做等于白装）

`bug-free-octo-potato` 的 `.gitignore` 里有这两行：

```
/AGENTS.md
/CLAUDE.md
```

**它们会把入口文件排除掉**，队友 clone 下来根本没有约束文件。必须先删掉这两行（AI 不许自己改 `.gitignore`，得由你来动）。

检查方式：

```bash
cd bug-free-octo-potato
git check-ignore -v AGENTS.md CLAUDE.md    # 有输出 = 仍被忽略，要处理
git add AGENTS.md CLAUDE.md .cursorrules Agent约束规则/
```

---

## 四、三份入口为什么内容一样

不同 AI 工具认的文件名不一样，没有哪个名字是通用的：

| 工具 | 自动读取 |
|---|---|
| Claude Code | `CLAUDE.md` |
| Cursor | `.cursorrules`（旧版） |
| GitHub Copilot | `.github/copilot-instructions.md` |
| 通用标准（越来越多工具支持） | `AGENTS.md` |

所以放三份同名内容。**代价是改一处要改三处**，同步命令：

```bash
cd 代码内置约束
cp AGENTS.md CLAUDE.md && cp AGENTS.md .cursorrules
```

嫌麻烦就只保留 `AGENTS.md`——现在主流工具基本都支持这个名字。

---

## 五、怎么验证 AI 真的读到了（关键）

不要相信 AI 说"我已了解约束"。开工前让它回答三个问题：

> 1. 这个项目的技术栈是什么？
> 2. 六条红线分别是哪六条？
> 3. 遇到要改数据库字段的情况，你应该怎么做？

**答不上来或答错 = 入口没被加载**，先解决加载问题再开工，否则后面全是返工。

---

## 六、日常怎么用

AI 进仓库 → 自动读到入口 → 入口里有张表告诉它"遇到什么问题去读哪个文件" → 它按需去 `Agent约束规则/` 里取。

不需要每次手动 @ 任何文件，这是方案二最大的好处。

---

## 七、可选增强（用到再说）

- **Cursor 新版**：可以把 `.cursorrules` 换成 `.cursor/rules/*.mdc`，支持按目录/文件类型自动加载（比如只在打开 `*.go` 时加载 Go 规范）。需要的话跟我说，我来生成。
- **按业务域分目录加载**：Claude Code 支持在子目录放 `CLAUDE.md`，比如 `internal/order/CLAUDE.md` 里写"本域约束见 ../../Agent约束规则/业务约束.md"，AI 一打开订单代码就自动加载订单约束。适合六人分域开发。
