# data-agent.md — 数据模型硬约束

> 前置：先加载 `core-agent.md`。
> 适用场景：任何会**落库 / 读库 / 定义模型 / 写 SQL / 写 Mongo 查询**的代码。
> 来源：`06-MongoDB数据字典-补齐版.md` §0 全局设计规范。违反任何一条 = 返工。

---

1. **金额一律用「分」为单位的整数**，字段名以 `Fee / Amount / Price / Income` 结尾。**全链路禁止 float/double 表示钱**。示例：`totalFee: 3850` = 38.50 元。

2. **敏感三件套**：手机号 / 身份证 / 银行卡落库 = 加密字段（后缀 `Enc`，AES-256-GCM）+ 哈希字段（后缀 `Hash`，用于等值查询建唯一索引）+ 脱敏字段（后缀 `Mask`，如 `138****8888`，用于展示）。**任何接口返回、日志输出敏感信息时 MUST 输出 Mask 版本**。

3. **经纬度统一 GeoJSON**：`{ type: "Point", coordinates: [lng, lat] }`，建 `2dsphere` 索引。MUST NOT 自造 `lat`/`lng` 双字段。

4. **时间统一 UTC 存储**，展示层转本地时区（默认 `Asia/Shanghai`）。

5. **状态枚举全部小写下划线**（如 `in_service`、`listening`），代码中以常量导出，MUST NOT 在代码里散落裸字符串。订单状态机例外，用大写（见 `business-agent.md` §1）。

6. **软删除**：业务数据一律 `isDeleted: true`，**MUST NOT 物理删除任何业务数据**；轨迹/日志/验证码类用 TTL 索引自动过期。

7. **通用审计字段**：所有集合必带 `createdAt / updatedAt / isDeleted`；并发写集合（订单/账单/钱包/支付）必带 `version` 乐观锁。

8. **集合与字段以 `06` 数据字典为唯一 schema**。AI MUST NOT 擅自新增集合、改字段名、改类型、改索引。确实需要变更：先提变更说明（写清业务理由），**人工确认并更新数据字典之后**才允许改代码。代码先行的 schema 变更一律打回。

---

## 并发写集合清单（必须带乐观锁 `version`）

`orders`、`order_status_logs`、`bills`、`bill_items`、`payments`、`refunds`、`driver_wallets`、`wallet_transactions`、`withdrawals`、`settlements`、`dispatches`、`coupon_batches`、`user_coupons`、`driver_qualifications`、`driver_audits`

## 模型定义时的自查

- [ ] 字段名 / 类型 / 索引与 `06` 字典逐字段核对过？
- [ ] 金额字段是整数（分）？
- [ ] 敏感字段三件套齐了？
- [ ] 坐标是 GeoJSON？
- [ ] 审计字段 + （并发集合的）`version` 带了？
- [ ] 需要变更 schema 的话——是否已经停下走人工流程？
