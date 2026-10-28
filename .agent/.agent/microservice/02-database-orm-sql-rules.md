# 规则 5：数据库 & ORM & SQL 约束

> 适用范围：`bw-cli` 中所有数据库（SQLite/MySQL/PostgreSQL/MongoDB）相关的 DDL、ORM、SQL 编写。
> 目标：**DDL 一律走变更工单评审；SQL 防注入、防全量误更新、防 N+1，事务边界清晰。**

---

## 1. DDL 变更约束（表结构、字段、索引）

- **禁止 AI 自行新建表、修改表、新增/删除/修改字段、增删索引、修改约束。**
- 任何 DDL 变更必须：
  1. 单独发起变更工单（说明业务背景、变更内容、兼容性、回滚方案）；
  2. 经人工评审通过后，由人工或人工确认后执行迁移脚本；
  3. 迁移脚本纳入版本管理，禁止只在本地手动执行。
- `AutoMigrate`（如 `repo.AutoMigrate`）仅用于脚手架演示/本地开发，**不得**在生产环境靠 `AutoMigrate` 变更线上表结构。
- 字段新增原则：只允许新增可空（含默认值）字段，禁止删除字段、禁止改字段类型、禁止改字段语义。
- 索引变更必须评估：是否命中现有查询、是否冗余、对写性能影响。

---

## 2. SQL 编写约束

- **禁止 `SELECT *`**：查询必须显式列出所需字段（用 Gorm 的 `Select` 或明确模型字段）。
- **禁止拼接 SQL（防注入）**：一律使用参数绑定（`?` 占位符 / Gorm 条件式），禁止 `fmt.Sprintf` / 字符串拼接拼接用户输入。
- **update / delete 必须带 WHERE 条件**：禁止无 `WHERE` 的全量更新/删除。Gorm 中禁止 `.Save` 无主键、禁止 `.Update` 不带条件、禁止 `.Delete` 不带条件。
- 全量删除/更新若确需，必须显式写明条件并加注释说明，且需人工确认。

### 2.1 正确示例（Gorm 参数化 + 明确字段）
```go
// 正确：参数绑定 + 显式字段 + WHERE 条件
var rec dbmodel.OrderModel
err := r.db.WithContext(ctx).
    Select("id", "name", "description", "created_at").
    Where("id = ?", id).
    First(&rec).Error

// 正确：条件更新
err = r.db.WithContext(ctx).
    Model(&dbmodel.OrderModel{}).
    Where("id = ?", id).
    Updates(map[string]any{"name": name, "updated_at": now}).Error

// 正确：条件删除
err = r.db.WithContext(ctx).
    Where("id = ?", id).
    Delete(&dbmodel.OrderModel{}).Error
```

### 2.2 错误示例（禁止）
```go
// 错误：拼接 SQL（注入风险）
q := fmt.Sprintf("SELECT * FROM orders WHERE id = '%s'", id) // 禁止

// 错误：SELECT * 无字段约束
r.db.Raw("SELECT * FROM orders").Scan(&items) // 禁止

// 错误：无 WHERE 全量删除/更新
r.db.Delete(&dbmodel.OrderModel{}) // 禁止
r.db.Save(&dbmodel.OrderModel{Name: name}) // 禁止（无主键会全量写入语义不清）
```

---

## 3. 分页规范

- 分页入参统一走 `dto.ListCommand`（`Page` / `PageSize`）。
- 分页参数由 `service` 层统一归一化（默认页=1、默认大小=20、上限=100），参考项目 `normalizePagination`。
- 分页查询必须分两步：先 `Count` 得到 `total`，再 `Offset/Limit` 取数据。
- 禁止用 `Offset` 做超大页码深翻页；大数据量场景改用游标/`WHERE id > ?` 键集分页。

---

## 4. 事务边界约束

- 涉及多步写操作且必须原子时，必须显式使用事务（`db.Transaction`），保证"要么全成、要么全回滚"。
- 事务范围尽量小，禁止在事务内做远程调用、发消息、长耗时操作（事务持有锁越久越危险）。
- 领域事件发布与本地写的一致性：先落可靠事件表（同一事务），再异步发布，禁止在事务内直接发 MQ。
- 支付/退款/提现等资金操作必须事务 + 幂等键（`idempotency_key`）双重保护。

```go
// 正确：事务边界清晰
err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
    if err := tx.Create(&record).Error; err != nil {
        return err
    }
    return tx.Create(&eventRecord).Error // 可靠事件同事务
})
```

---

## 5. ORM 与结构体映射约束

- 持久化模型放 `internal/{服务名}/model/`，实现 `TableName()`（Gorm）或 `MongoCollectionName()`（Mongo）。
- entity ↔ model 转换必须在 `repo` 层完成（`toRecord` / `toDomain`），禁止跨层直接透传模型。
- 禁止在 `entity` 加 `gorm`/`bson` 标签；禁止在 `model` 加业务逻辑。
- Mongo 文档模型用 `bson` 标签，集合名用 `MongoCollectionName()`；Gorm 模型用 `gorm` 标签。

---

## 6. 防 N+1 与性能约束

- 列表查询 + 关联查询必须一次性 `Preload` / `Find`，禁止在循环里逐条查询（N+1）。
- 明确需要索引的查询字段，走 DDL 变更工单补索引，禁止靠全表扫描。
- 大批量写入用批量插入，禁止逐条 `Create`。
- 查询只取必要字段，禁止无谓加载大字段（`description`/`text` 等）。

---

## 7. 禁止行为

- ❌ AI 自行建表、改字段、改索引（DDL 必须走变更工单）。
- ❌ 生产环境依赖 `AutoMigrate` 变更表结构。
- ❌ `SELECT *`。
- ❌ 拼接 SQL / 不参数化的查询（防注入红线）。
- ❌ `UPDATE` / `DELETE` 不带 `WHERE`（全量更新/删除）。
- ❌ 在事务内发消息、做远程调用、做长耗时操作。
- ❌ 循环里逐条查询（N+1）。
- ❌ 在 `entity` 写 `gorm`/`bson` 标签、在 `model` 写业务逻辑。
- ❌ 跨层直接透传持久化模型。
- ❌ 资金操作缺少事务或幂等保护。

---

## 8. 交付自查清单

- [ ] 本次是否涉及 DDL？若是，是否已走变更工单并人工评审？
- [ ] 所有查询是否显式字段（无 `SELECT *`）、是否参数化（无拼接）
- [ ] 所有 update/delete 是否都带 WHERE
- [ ] 分页是否走统一 `ListCommand` + `normalizePagination`
- [ ] 多步写是否用事务、事务内是否避免了远程调用/发消息
- [ ] 是否避免了 N+1、是否只在 repo 层做 entity↔model 转换
