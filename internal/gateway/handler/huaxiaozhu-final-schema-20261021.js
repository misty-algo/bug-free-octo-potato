// ============================================================================
// 花小猪打车平台 · 最终 MongoDB 全量建集合脚本（2026-10-21）
// ----------------------------------------------------------------------------
// 依据：
//   1. 《花小猪打车平台_完整业务汇总》第五章「业务实体与唯一约束」§5.1
//      与第四章「公共登录与跨端唯一业务规则」（状态机、资金桶、分账公式）。
//   2. 《06-MongoDB数据字典-补齐版.md》v1.2（60 集合，字段级权威基准）。
//   3. huaxiaozhu-schema-patch-20261021.js（旧缺口补齐脚本，本次已并入）。
//
// 运行方式（mongosh，不能用 Navicat 的 JSON/CSV 导入入口）：
//   mongosh "mongodb://dev:dev123456@localhost:27017/huaxiaozhu?authSource=admin" \
//     --file huaxiaozhu-final-schema-20261021.js
//
// 命名统一裁定（评审口径）：
//   1. 集合名统一 snake_case；字段名统一 camelCase；审计字段 createdAt/updatedAt/isDeleted。
//   2. 并发写集合带 version 乐观锁（清单见 CONCURRENT_SET）。
//   3. 金额一律「分」整数；敏感三件套 = Enc(加密)/Hash(等值查询)/Mask(脱敏)。
//   4. 坐标统一 GeoJSON Point/Polygon，建 2dsphere；禁止自造 lat/lng 双字段。
//   5. 状态枚举统一小写下划线；订单行程状态机例外用大写六态：
//      WAITING / ACCEPTED / ARRIVED / IN_TRIP / ENDED / CANCELLED。
//   6. 本脚本为「只增不改」全量基线：不 drop 已存在集合；字段冲突以本文件为准。
// ============================================================================

const DB_NAME = db.getName();

// ---------------- 字段类型辅助函数 ----------------
// 这些函数把「中文说明 + 类型约束」封装成 JSON Schema 片段，供每个集合的 fields 复用。
// 例如 T('手机号') 会生成 { bsonType:'string', description:'手机号' }。

// 文本字段（字符串）
const T = (description) => ({ bsonType: 'string', description });
// 引用字段（关联其它集合的 _id，类型 ObjectId）
const R = (description) => ({ bsonType: 'objectId', description });
// 日期时间字段（UTC 存储）
const D = (description) => ({ bsonType: 'date', description });
// 布尔字段
const B = (description) => ({ bsonType: 'bool', description });
// 整数字段（默认 ≥0；用于计数、版本号、时长秒数等）
const I = (description, minimum = 0) => ({ bsonType: ['int', 'long'], minimum, multipleOf: 1, description });
// 金额字段（「分」为单位的整数，全链路禁止 float/double 表示钱）
const M = (description) => ({ bsonType: ['int', 'long'], minimum: 0, multipleOf: 1, description });
// 数值字段（整数或小数；用于比例、评分、里程等）
const N = (description, minimum = 0) => ({ bsonType: ['int', 'long', 'double', 'decimal'], minimum, description });
// 枚举字段（字符串，限定取值范围；状态、类型、渠道等）
const C = (description, values) => ({ bsonType: 'string', enum: values, description });
// 内嵌对象字段（子文档，默认不允许额外属性）
const O = (description, properties, required = []) => ({
  bsonType: 'object', description, additionalProperties: false, properties,
  ...(required.length ? { required } : {})
});
// 数组字段（可指定元素类型与最大长度）
const A = (description, items, maxItems) => ({
  bsonType: 'array', description, items, ...(maxItems ? { maxItems } : {})
});
// 可空引用（system 操作或自动任务时为 null，如 operatorId）
const RN = (description) => ({ bsonType: ['objectId', 'string', 'null'], description });
// GeoJSON 点（坐标：[经度, 纬度]），配合 2dsphere 索引
const P = (description) => ({
  bsonType: 'object', description,
  properties: {
    type: C('固定为 Point', ['Point']),  // 固定为 Point
    coordinates: A('[经度, 纬度]', N('经纬度', -180))  // [经度, 纬度]
  },
  required: ['type', 'coordinates']
});
// GeoJSON 多边形（闭合环坐标数组），用于接单片区、围栏、行政边界
const PG = (description) => ({
  bsonType: 'object', description,
  properties: {
    type: C('固定为 Polygon', ['Polygon']),  // 固定为 Polygon
    coordinates: A('闭合环坐标数组', A('[经度, 纬度]', N('经纬度', -180)))  // 闭合环坐标数组
  },
  required: ['type', 'coordinates']
});
// 通用审计字段（所有集合必带）：创建时间 / 更新时间 / 软删除标记
const AUDIT = { createdAt: D('创建时间'), updatedAt: D('更新时间'), isDeleted: B('软删除标记；禁止物理删除') };
// 乐观锁字段（并发写集合必带）：version 递增，状态/资金变更时防并发覆盖
const LOCK = { version: I('乐观锁版本号；状态/资金变更防并发覆盖') };

// ---------------- 订单行程状态机（业务汇总 §4.2） ----------------
// 订单行程六态（跨端统一大写字符串，取代原 10/20/30 数值编码）
const TRIP_STATE = ['WAITING', 'ACCEPTED', 'ARRIVED', 'IN_TRIP', 'ENDED', 'CANCELLED'];
// 账单状态：待核算 / 待支付 / 已支付 / 作废（与行程状态分离）
const BILL_STATE = ['CALCULATING', 'UNPAID', 'PAID', 'VOID'];
// 支付尝试状态：已创建 / 处理中 / 成功 / 失败 / 已关单
const PAY_STATE = ['CREATED', 'PROCESSING', 'SUCCESS', 'FAILED', 'CLOSED'];
// 退款状态：审批中 / 处理中 / 成功 / 失败 / 已驳回
const REFUND_STATE = ['REVIEW', 'PROCESSING', 'SUCCESS', 'FAILED', 'REJECTED'];
// 运输任务状态：开放 / 已预占 / 已分配 / 进行中 / 已结束 / 已取消
const TASK_STATE = ['OPEN', 'RESERVED', 'ASSIGNED', 'ACTIVE', 'FINISHED', 'CANCELLED'];
// 司机资金五桶：U 待结算 / A 可用 / Fz 冻结 / W 提现预占 / Debt 待追偿（业务汇总 §4.4）
const FUND_BUCKET = ['U', 'A', 'Fz', 'W', 'Debt'];

// ---------------- 并发写集合（必须带 version） ----------------
// 这些集合涉及状态变更或资金扣减，写入时 MUST 使用 findOneAndUpdate + version 乐观锁
const CONCURRENT_SET = ['orders', 'order_status_logs', 'transport_tasks', 'bills', 'bill_items',
  'payments', 'refunds', 'driver_wallets', 'wallet_transactions', 'accounting_entries',
  'withdrawals', 'settlements', 'dispatches', 'dispatch_attempts', 'coupon_batches', 'user_coupons',
  'driver_qualifications', 'driver_audits', 'estimates', 'cancels_quotes', 'destination_changes'];

// ---------------- 集合定义 ----------------
// 每个集合：{ name, required, fields, indexes }
const collections = [];

// ============================ 1. 账户域 ============================
collections.push({
  // 【乘客账户：乘客主账号，含实名、会员等级、信用分、钱包与邀请关系】
  name: 'users',
  required: ['phoneHash', 'phoneMask', 'nickname', 'cityCode', 'realNameStatus', 'status',
    'creditScore', 'walletBalance', 'memberLevel', 'inviteCode', 'registerChannel', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    phoneEnc: T('加密手机号 AES-256-GCM'),  // 加密手机号 AES-256-GCM
    phoneHash: T('手机号 HMAC，等值查询唯一索引'),  // 手机号 HMAC，等值查询唯一索引
    phoneMask: T('脱敏手机号，如 138****8888'),  // 脱敏手机号，如 138****8888
    nickname: T('昵称，默认「乘客xxxx」'),  // 昵称，默认「乘客xxxx」
    avatarUrl: T('头像 OSS 地址'),  // 头像 OSS 地址
    gender: C('性别', ['male', 'female', 'unknown']),  // 性别
    birthday: D('生日'),  // 生日
    cityCode: T('常驻城市码'),  // 常驻城市码
    realNameStatus: C('实名状态', ['unverified', 'verifying', 'verified', 'failed']),  // 实名状态
    realName: T('真实姓名（加密）'),  // 真实姓名（加密）
    idCardEnc: T('身份证号（加密）'),  // 身份证号（加密）
    idCardMask: T('脱敏身份证'),  // 脱敏身份证
    faceVerifyAt: D('最近人脸核身通过时间'),  // 最近人脸核身通过时间
    status: C('账号状态', ['enabled', 'disabled', 'closing', 'closed', 'frozen', 'banned']),  // 账号状态
    banReason: T('封禁原因'),  // 封禁原因
    banUntil: D('封禁到期（空=永久）'),  // 封禁到期（空=永久）
    creditScore: N('信用分 0~100'),  // 信用分 0~100
    isNewUser: B('是否未下过首单'),  // 是否未下过首单
    firstOrderAt: D('首单完成时间'),  // 首单完成时间
    totalOrderCount: I('累计完单数'),  // 累计完单数
    cancelOrderCount: I('累计取消数（乘客责任）'),  // 累计取消数（乘客责任）
    noShowCount: I('爽约次数'),  // 爽约次数
    walletBalance: M('钱包余额（分）'),  // 钱包余额（分）
    walletGiftBalance: M('赠送余额（分，不可提现）'),  // 赠送余额（分，不可提现）
    memberLevel: C('会员等级', ['none', 'silver', 'gold', 'platinum']),  // 会员等级
    memberExpireAt: D('会员到期'),  // 会员到期
    inviteCode: T('邀请码（6 位）'),  // 邀请码（6 位）
    invitedBy: R('邀请人 userId'),  // 邀请人 userId
    enterpriseId: R('所属企业'),  // 所属企业
    partnerId: R('渠道归属'),  // 渠道归属
    lastLoginAt: D('最近登录'),  // 最近登录
    lastLoginIp: T('最近登录 IP'),  // 最近登录 IP
    registerChannel: C('注册渠道', ['app', 'h5', 'wechat', 'alipay', 'cp']),  // 注册渠道
    registerSource: T('渠道明细 utm'),  // 渠道明细 utm
    wechatOpenid: T('微信 openid'),  // 微信 openid
    unionid: T('微信 unionid'),  // 微信 unionid
    defaultInvoiceTitleId: R('默认发票抬头'),  // 默认发票抬头
    noPassPayEnabled: B('是否开启免密支付'),  // 是否开启免密支付
    deletedAt: D('注销申请时间；7 天冷静期重新登录自动撤销'),  // 注销申请时间；7 天冷静期重新登录自动撤销
    preferences: O('乘车偏好', { quiet: B('安静'), noSmoking: B('禁烟'), temperature: T('温度'), luggage: B('行李') }),  // 乘车偏好
    tags: A('运营标签', T('标签')),  // 运营标签
    ...AUDIT
  },
  indexes: [
    { key: { phoneHash: 1 }, unique: true, name: 'phoneHash_1' },
    { key: { inviteCode: 1 }, unique: true, name: 'inviteCode_1' },
    { key: { idCardMask: 1 }, name: 'idCardMask_1' },
    { key: { cityCode: 1, status: 1 }, name: 'cityCode_status' },
    { key: { registerChannel: 1, createdAt: -1 }, name: 'registerChannel_createdAt' }
  ]
});

collections.push({
  // 【乘客常用地址：家/公司/自定义常用地址，支持默认与使用次数排序】
  name: 'user_addresses',
  required: ['userId', 'label', 'name', 'address', 'location', 'cityCode', 'useCount', 'isDefault',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    userId: R('所属乘客'),  // 所属乘客
    label: C('标签', ['home', 'company', 'custom']),  // 标签
    name: T('地址名称'),  // 地址名称
    address: T('详细地址文本'),  // 详细地址文本
    poiId: T('关联 POI ID'),  // 关联 POI ID
    location: P('经纬度 GeoJSON'),  // 经纬度 GeoJSON
    houseNumber: T('门牌号'),  // 门牌号
    cityCode: T('城市码'),  // 城市码
    useCount: I('使用次数（排序权重）'),  // 使用次数（排序权重）
    isDefault: B('是否默认'),  // 是否默认
    ...AUDIT
  },
  indexes: [
    { key: { userId: 1, isDefault: -1, useCount: -1 }, name: 'userId_default_useCount' },
    { key: { location: '2dsphere' }, name: 'location_2dsphere' }
  ]
});

collections.push({
  // 【紧急联系人：一键报警优先通知的联系人】
  name: 'user_emergency_contacts',
  required: ['userId', 'name', 'phoneEnc', 'isPrimary', 'notifyEnabled', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    userId: R('乘客 ID'),  // 乘客 ID
    name: T('联系人姓名'),  // 联系人姓名
    phoneEnc: T('加密手机号'),  // 加密手机号
    relation: C('关系', ['family', 'friend', 'colleague', 'other']),  // 关系
    isPrimary: B('是否主联系人'),  // 是否主联系人
    notifyEnabled: B('是否接收异常行程通知'),  // 是否接收异常行程通知
    ...AUDIT
  },
  indexes: [{ key: { userId: 1 }, name: 'userId_1' }]
});

collections.push({
  // 【设备指纹：登录设备与风控因子（越狱/模拟器/风险等级）】
  name: 'user_devices',
  required: ['deviceId', 'platform', 'riskLevel', 'lastActiveAt', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    userId: R('绑定用户（未登录为空）'),  // 绑定用户（未登录为空）
    deviceId: T('设备唯一标识'),  // 设备唯一标识
    platform: C('平台', ['ios', 'android', 'h5', 'web']),  // 平台
    deviceModel: T('机型'),  // 机型
    osVersion: T('系统版本'),  // 系统版本
    appVersion: T('App 版本'),  // App 版本
    ip: T('最近 IP'),  // 最近 IP
    isRooted: B('是否越狱/ROOT'),  // 是否越狱/ROOT
    isEmulator: B('是否模拟器'),  // 是否模拟器
    riskLevel: C('风险等级', ['low', 'medium', 'high']),  // 风险等级
    lastActiveAt: D('最近活跃'),  // 最近活跃
    ...AUDIT
  },
  indexes: [
    { key: { deviceId: 1 }, name: 'deviceId_1' },
    { key: { userId: 1 }, name: 'userId_1' },
    { key: { riskLevel: 1 }, name: 'riskLevel_1' }
  ]
});

collections.push({
  // 【短信验证码：登录/注册等场景验证码，TTL 自动过期】
  name: 'sms_codes',
  required: ['phoneHash', 'scene', 'code', 'expireAt', 'sendCount', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    phoneHash: T('手机号哈希'),  // 手机号哈希
    scene: C('场景', ['login', 'register', 'resetPwd', 'bindCard', 'changePhone']),  // 场景
    code: T('验证码（生产仅存哈希）'),  // 验证码（生产仅存哈希）
    expireAt: D('过期时间（TTL）'),  // 过期时间（TTL）
    usedAt: D('使用时间'),  // 使用时间
    sendCount: I('当日发送次数（频控）'),  // 当日发送次数（频控）
    ip: T('请求 IP'),  // 请求 IP
    ...AUDIT
  },
  indexes: [
    { key: { phoneHash: 1, scene: 1 }, name: 'phoneHash_scene' },
    { key: { expireAt: 1 }, expireAfterSeconds: 0, name: 'expireAt_ttl' }
  ]
});

// ============================ 2. 司机与车辆域 ============================
collections.push({
  // 【司机账户：司机主账号，含资质生命周期、在线状态、服务分、收入统计】
  name: 'drivers',
  required: ['driverNo', 'phoneHash', 'phoneMask', 'realName', 'idCardEnc', 'cityCode',
    'status', 'onlineStatus', 'workStatus', 'serviceScore', 'acceptRate', 'balance',
    'contractStatus', 'createdAt', 'updatedAt', 'isDeleted', 'version'],
  fields: {
    driverNo: T('司机工号'),  // 司机工号
    phoneEnc: T('加密手机号'),  // 加密手机号
    phoneHash: T('手机号哈希'),  // 手机号哈希
    phoneMask: T('脱敏手机号'),  // 脱敏手机号
    realName: T('真实姓名'),  // 真实姓名
    idCardEnc: T('身份证（加密）'),  // 身份证（加密）
    gender: C('性别', ['male', 'female']),  // 性别
    birthday: D('出生日期'),  // 出生日期
    avatarUrl: T('头像'),  // 头像
    cityCode: T('服务城市'),  // 服务城市
    serviceCityCodes: A('可接单城市列表', T('城市码')),  // 可接单城市列表
    driverType: C('司机类型', ['full_time', 'part_time', 'fleet']),  // 司机类型
    partnerId: R('所属加盟商'),  // 所属加盟商
    fleetId: R('所属车队'),  // 所属车队
    inviteCode: T('注册邀约码'),  // 注册邀约码
    status: C('生命周期', ['registered', 'auditing', 'audited', 'training', 'active', 'suspended', 'frozen', 'banned', 'quit']),  // 生命周期
    onlineStatus: C('在线状态', ['offline', 'online', 'listening', 'in_service', 'resting']),  // 在线状态
    workStatus: C('接单模式', ['auto', 'manual', 'appoint_only']),  // 接单模式
    acceptMode: C('接单方式（与 onlineStatus 配合）', ['dispatch', 'grab']),  // 接单方式（与 onlineStatus 配合）
    serviceScore: N('服务分'),  // 服务分
    starLevel: N('星级 1~5'),  // 星级 1~5
    acceptRate: N('接单率'),  // 接单率
    completeRate: N('完单率'),  // 完单率
    cancelRate: N('司机责任取消率'),  // 司机责任取消率
    complaintRate: N('投诉率'),  // 投诉率
    totalOrderCount: I('累计完单'),  // 累计完单
    todayOrderCount: I('当日完单'),  // 当日完单
    todayIncome: M('当日收入（分）'),  // 当日收入（分）
    totalIncome: M('累计收入（分）'),  // 累计收入（分）
    monthIncome: M('本月累计已结算收入（分）；派单优先级第一维度'),  // 本月累计已结算收入（分）；派单优先级第一维度
    balance: M('可提现余额（分）'),  // 可提现余额（分）
    frozenBalance: M('冻结金额（分）'),  // 冻结金额（分）
    creditScore: N('司机信用分'),  // 司机信用分
    lastLocation: P('最近上报位置'),  // 最近上报位置
    lastLocationAt: D('最近上报时间'),  // 最近上报时间
    todayOnlineMinutes: I('当日累计在线分钟'),  // 当日累计在线分钟
    continuousOnlineMinutes: I('连续在线分钟'),  // 连续在线分钟
    restUntil: D('强制休息截止'),  // 强制休息截止
    currentOrderId: R('当前进行中订单'),  // 当前进行中订单
    bankCardEnc: T('银行卡（加密）'),  // 银行卡（加密）
    bankName: T('开户行'),  // 开户行
    bankCardAuditStatus: C('银行卡审核状态', ['pending', 'approved', 'rejected']),  // 银行卡审核状态
    contractStatus: C('签约状态', ['unsigned', 'signed', 'expired']),  // 签约状态
    contractSignedAt: D('签约时间'),  // 签约时间
    activatedAt: D('激活接单时间'),  // 激活接单时间
    quitAt: D('注销时间'),  // 注销时间
    areaLimitSwitch: C('接单区域限制开关', ['0', '1']),  // 接单区域限制开关
    listenRadius: I('听单半径（米）'),  // 听单半径（米）
    listenPreferences: O('听单偏好', {  // 听单偏好
      destKeyword: T('目的地关键词'), preferAreas: A('偏好区域', T('区域码')),
      avoidAreas: A('避开区域', T('区域码')), maxPickupDistance: I('最大接驾距离')
    }),
    preferProductCodes: A('可接受产品线', T('产品线 code')),  // 可接受产品线
    carpoolEnabled: B('是否接拼车单'),  // 是否接拼车单
    tags: A('运营标签', T('标签')),  // 运营标签
    deviceId: T('当前登录设备'),  // 当前登录设备
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { driverNo: 1 }, unique: true, name: 'driverNo_1' },
    { key: { phoneHash: 1 }, unique: true, name: 'phoneHash_1' },
    { key: { cityCode: 1, status: 1, onlineStatus: 1 }, name: 'cityCode_status_online' },
    { key: { lastLocation: '2dsphere' }, name: 'lastLocation_2dsphere' },
    { key: { serviceScore: -1 }, name: 'serviceScore_-1' }
  ]
});

collections.push({
  // 【司机资质证件：身份证/驾驶证/车证/保险等证件与到期预警】
  name: 'driver_qualifications',
  required: ['driverId', 'type', 'verifyStatus', 'isExpired', 'createdAt', 'updatedAt', 'isDeleted', 'version'],
  fields: {
    driverId: R('司机 ID'),  // 司机 ID
    type: C('证件类型', ['id_card', 'driver_license', 'vehicle_license', 'netcar_driver_permit',  // 证件类型
      'netcar_transport_permit', 'insurance', 'annual_inspect', 'background_check']),
    certNo: T('证件编号（加密保存）'),  // 证件编号（加密保存）
    realName: T('证件姓名'),  // 证件姓名
    issueDate: D('发证日期'),  // 发证日期
    expireDate: D('有效期至'),  // 有效期至
    frontImageUrl: T('正面照'),  // 正面照
    backImageUrl: T('反面照'),  // 反面照
    ocrData: O('OCR 原始结果', { raw: T('原文') }),  // OCR 原始结果
    ocrStatus: C('OCR 状态', ['pending', 'success', 'failed']),  // OCR 状态
    verifyStatus: C('审核状态', ['pending', 'passed', 'rejected']),  // 审核状态
    rejectReason: T('驳回原因'),  // 驳回原因
    verifiedBy: R('审核人'),  // 审核人
    verifiedAt: D('审核时间'),  // 审核时间
    faceCompareScore: N('人脸比对分数'),  // 人脸比对分数
    isExpired: B('是否已过期'),  // 是否已过期
    remindLevel: C('预警等级', ['none', 'd30', 'd7', 'd1', 'expired']),  // 预警等级
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { driverId: 1, type: 1 }, name: 'driverId_type' },
    { key: { expireDate: 1, isExpired: 1 }, name: 'expireDate_isExpired' },
    { key: { verifyStatus: 1 }, name: 'verifyStatus_1' }
  ]
});

collections.push({
  // 【司机审核记录：机审+人审全流水，驳回重提递增轮次】
  name: 'driver_audits',
  required: ['driverId', 'round', 'submitAt', 'machineResult', 'finalStatus',
    'createdAt', 'updatedAt', 'isDeleted', 'version'],
  fields: {
    driverId: R('司机 ID'),  // 司机 ID
    round: I('第几轮审核'),  // 第几轮审核
    submitAt: D('提交时间'),  // 提交时间
    machineResult: C('机审结果', ['pass', 'reject', 'review']),  // 机审结果
    machineDetail: O('机审明细', { ocrPass: B('OCR'), facePass: B('人脸'), blacklistPass: B('黑名单'), licenseValid: B('证件有效') }),  // 机审明细
    manualResult: C('人审结果', ['pass', 'reject']),  // 人审结果
    manualRemark: T('人审备注'),  // 人审备注
    operatorId: RN('审核人'),  // 审核人
    operateAt: D('审核时间'),  // 审核时间
    finalStatus: C('终态', ['pending', 'approved', 'rejected']),  // 终态
    rejectReasons: A('驳回原因列表', T('原因')),  // 驳回原因列表
    slaMinutes: I('审核耗时'),  // 审核耗时
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { driverId: 1, round: -1 }, name: 'driverId_round' },
    { key: { finalStatus: 1, submitAt: 1 }, name: 'finalStatus_submitAt' }
  ]
});

collections.push({
  // 【司机培训与考试：准入/安全/服务培训与考试成绩】
  name: 'driver_trainings',
  required: ['driverId', 'courseCode', 'courseName', 'courseType', 'studyProgress',
    'passScore', 'isPassed', 'attemptCount', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    driverId: R('司机 ID'),  // 司机 ID
    courseCode: T('课程编码'),  // 课程编码
    courseName: T('课程名称'),  // 课程名称
    courseType: C('课程类型', ['onboarding', 'safety', 'service', 'rectify']),  // 课程类型
    studyProgress: N('学习进度 0~100'),  // 学习进度 0~100
    examScore: N('考试成绩 0~100'),  // 考试成绩 0~100
    passScore: N('及格分'),  // 及格分
    isPassed: B('是否通过'),  // 是否通过
    attemptCount: I('考试次数'),  // 考试次数
    completedAt: D('完成时间'),  // 完成时间
    expireAt: D('有效期'),  // 有效期
    ...AUDIT
  },
  indexes: [
    { key: { driverId: 1, courseType: 1 }, name: 'driverId_courseType' },
    { key: { isPassed: 1 }, name: 'isPassed_1' }
  ]
});

collections.push({
  // 【车辆档案：车牌、车型、车架号、保险/年检/运输证到期】
  name: 'vehicles',
  required: ['plateNo', 'cityCode', 'brand', 'model', 'color', 'seatCount', 'vehicleType',
    'energyType', 'vin', 'ownerType', 'usageType', 'status', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    plateNo: T('车牌号'),  // 车牌号
    cityCode: T('归属城市'),  // 归属城市
    brand: T('品牌'),  // 品牌
    model: T('型号'),  // 型号
    color: T('颜色'),  // 颜色
    seatCount: I('座位数', 1),  // 座位数
    vehicleType: C('车型', ['sedan', 'suv', 'mpv', 'ev']),  // 车型
    energyType: C('能源', ['gasoline', 'hybrid', 'electric']),  // 能源
    vin: T('车架号'),  // 车架号
    engineNo: T('发动机号'),  // 发动机号
    purchaseDate: D('购车日期'),  // 购车日期
    vehicleAge: N('车龄（年）'),  // 车龄（年）
    mileage: N('行驶里程 km'),  // 行驶里程 km
    ownerName: T('车辆所有人'),  // 车辆所有人
    ownerType: C('所有人类型', ['personal', 'company', 'leasing']),  // 所有人类型
    usageType: C('使用性质', ['non_operating', 'ride_hailing']),  // 使用性质
    insuranceExpireAt: D('保险到期'),  // 保险到期
    annualInspectExpireAt: D('年检到期'),  // 年检到期
    netcarPermitExpireAt: D('运输证到期'),  // 运输证到期
    images: O('图片', { front: T('正面'), back: T('反面'), interior: T('内饰'), license: T('证件') }),  // 图片
    status: C('车辆状态', ['normal', 'maintenance', 'insurance_expired', 'inspect_expired', 'stopped', 'scrapped']),  // 车辆状态
    partnerId: R('所属加盟商/车队'),  // 所属加盟商/车队
    gpsDeviceNo: T('车载终端编号'),  // 车载终端编号
    ...AUDIT
  },
  indexes: [
    { key: { plateNo: 1 }, unique: true, name: 'plateNo_1' },
    { key: { vin: 1 }, unique: true, name: 'vin_1' },
    { key: { cityCode: 1, status: 1 }, name: 'cityCode_status' }
  ]
});

collections.push({
  // 【司机车辆绑定：司机与车辆的一对一有效绑定关系】
  name: 'driver_vehicle_binds',
  required: ['driverId', 'vehicleId', 'isCurrent', 'bindAt', 'bindStatus', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    driverId: R('司机 ID'),  // 司机 ID
    vehicleId: R('车辆 ID'),  // 车辆 ID
    isCurrent: B('是否当前使用车辆'),  // 是否当前使用车辆
    bindAt: D('绑定时间'),  // 绑定时间
    unbindAt: D('解绑时间'),  // 解绑时间
    bindStatus: C('绑定状态', ['active', 'unbound']),  // 绑定状态
    ...AUDIT
  },
  indexes: [
    { key: { driverId: 1, isCurrent: 1 }, name: 'driverId_isCurrent' },
    { key: { vehicleId: 1, bindStatus: 1 }, name: 'vehicleId_bindStatus' }
  ]
});

collections.push({
  // 【出车收车记录：每日出车时长、完单数与在线收入】
  name: 'driver_shifts',
  required: ['driverId', 'date', 'startAt', 'onlineMinutes', 'serviceMinutes', 'orderCount',
    'income', 'faceVerifyPassed', 'restCount', 'cityCode', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    driverId: R('司机 ID'),  // 司机 ID
    date: T('统计日期 YYYY-MM-DD'),  // 统计日期 YYYY-MM-DD
    startAt: D('出车时间'),  // 出车时间
    endAt: D('收车时间'),  // 收车时间
    onlineMinutes: I('在线时长（分钟）'),  // 在线时长（分钟）
    serviceMinutes: I('服务时长'),  // 服务时长
    orderCount: I('完单数'),  // 完单数
    income: M('当日收入（分）'),  // 当日收入（分）
    faceVerifyPassed: B('出车核身是否通过'),  // 出车核身是否通过
    restCount: I('强制休息次数'),  // 强制休息次数
    cityCode: T('出车城市'),  // 出车城市
    ...AUDIT
  },
  indexes: [
    { key: { driverId: 1, date: -1 }, name: 'driverId_date' },
    { key: { date: 1, cityCode: 1 }, name: 'date_cityCode' }
  ]
});

collections.push({
  // 【司机接单片区：司机自定义接单多边形区域】
  name: 'driver_service_areas',
  required: ['driverId', 'areaName', 'polygon', 'enabled', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    driverId: R('司机 ID'),  // 司机 ID
    areaName: T('片区名称'),  // 片区名称
    polygon: PG('片区多边形 GeoJSON'),  // 片区多边形 GeoJSON
    cityCode: T('所属城市代码'),  // 所属城市代码
    enabled: B('是否启用'),  // 是否启用
    ...AUDIT
  },
  indexes: [
    { key: { driverId: 1 }, name: 'driverId_1' },
    { key: { polygon: '2dsphere' }, name: 'polygon_2dsphere' }
  ]
});

// ============================ 3. 渠道域 ============================
collections.push({
  // 【加盟商(CP)：加盟商企业信息、分润比例与结算周期】
  name: 'partners',
  required: ['partnerNo', 'companyName', 'licenseNo', 'legalPerson', 'contactName',
    'contactPhoneEnc', 'cityCodes', 'deposit', 'commissionRate', 'settleCycle', 'status', 'inviteCode',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    partnerNo: T('CP 编号'),  // CP 编号
    companyName: T('企业名称'),  // 企业名称
    licenseNo: T('统一社会信用代码'),  // 统一社会信用代码
    legalPerson: T('法人姓名'),  // 法人姓名
    contactName: T('联系人'),  // 联系人
    contactPhoneEnc: T('联系电话（加密）'),  // 联系电话（加密）
    cityCodes: A('经营城市', T('城市码')),  // 经营城市
    deposit: M('保证金（分）'),  // 保证金（分）
    commissionRate: N('分润比例 0~1'),  // 分润比例 0~1
    settleCycle: C('结算周期', ['weekly', 'monthly']),  // 结算周期
    status: C('状态', ['pending', 'active', 'suspended', 'terminated']),  // 状态
    contractStartAt: D('合同开始'),  // 合同开始
    contractEndAt: D('合同结束'),  // 合同结束
    inviteCode: T('CP 邀约码'),  // CP 邀约码
    ...AUDIT
  },
  indexes: [
    { key: { partnerNo: 1 }, unique: true, name: 'partnerNo_1' },
    { key: { licenseNo: 1 }, unique: true, name: 'licenseNo_1' },
    { key: { inviteCode: 1 }, unique: true, name: 'inviteCode_1' }
  ]
});

collections.push({
  // 【车队：车队管理，司机与车辆的归属组织】
  name: 'fleets',
  required: ['fleetNo', 'name', 'cityCode', 'driverCount', 'vehicleCount', 'status',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    fleetNo: T('车队编号'),  // 车队编号
    name: T('车队名称'),  // 车队名称
    partnerId: R('所属加盟商（空=直营）'),  // 所属加盟商（空=直营）
    cityCode: T('经营城市'),  // 经营城市
    managerName: T('车队长'),  // 车队长
    managerPhoneEnc: T('联系电话（加密）'),  // 联系电话（加密）
    driverCount: I('在册司机数'),  // 在册司机数
    vehicleCount: I('在册车辆数'),  // 在册车辆数
    commissionRate: N('车队分润比例'),  // 车队分润比例
    status: C('状态', ['active', 'inactive']),  // 状态
    contractStartAt: D('合作开始'),  // 合作开始
    contractEndAt: D('合作结束'),  // 合作结束
    ...AUDIT
  },
  indexes: [
    { key: { fleetNo: 1 }, unique: true, name: 'fleetNo_1' },
    { key: { partnerId: 1 }, name: 'partnerId_1' },
    { key: { cityCode: 1, status: 1 }, name: 'cityCode_status' }
  ]
});

// ============================ 4. 地理域（LBS） ============================
collections.push({
  // 【城市：开城状态、服务时段、中心点与产品线】
  name: 'cities',
  required: ['code', 'name', 'province', 'center', 'openStatus', 'timezone', 'productCodes',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    code: T('城市码'),  // 城市码
    name: T('城市名'),  // 城市名
    province: T('省份'),  // 省份
    center: P('城市中心点'),  // 城市中心点
    boundary: PG('行政边界'),  // 行政边界
    openStatus: C('开城状态', ['opened', 'closed', 'preparing']),  // 开城状态
    openedAt: D('开城时间'),  // 开城时间
    serviceTimeStart: T('服务开始 05:00'),  // 服务开始 05:00
    serviceTimeEnd: T('服务结束 24:00'),  // 服务结束 24:00
    timezone: T('时区'),  // 时区
    productCodes: A('已开通产品线', T('产品线 code')),  // 已开通产品线
    hotline: T('客服热线'),  // 客服热线
    ...AUDIT
  },
  indexes: [
    { key: { code: 1 }, unique: true, name: 'code_1' },
    { key: { center: '2dsphere' }, name: 'center_2dsphere' },
    { key: { openStatus: 1 }, name: 'openStatus_1' }
  ]
});

collections.push({
  // 【POI与推荐上车点：地图兴趣点与推荐上车点】
  name: 'pois',
  required: ['poiId', 'name', 'address', 'location', 'cityCode', 'category',
    'isRecommendPickup', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    poiId: T('三方地图 POI ID'),  // 三方地图 POI ID
    name: T('名称'),  // 名称
    address: T('地址'),  // 地址
    location: P('经纬度'),  // 经纬度
    cityCode: T('城市码'),  // 城市码
    category: C('分类', ['airport', 'station', 'hospital', 'mall', 'school', 'office', 'scenic', 'other']),  // 分类
    isRecommendPickup: B('是否推荐上车点'),  // 是否推荐上车点
    parentPoiId: T('父 POI'),  // 父 POI
    guideImages: A('上车指引图', T('图片地址')),  // 上车指引图
    guideText: T('指引文案'),  // 指引文案
    heatScore: N('热度（排序）'),  // 热度（排序）
    ...AUDIT
  },
  indexes: [
    { key: { poiId: 1 }, unique: true, name: 'poiId_1' },
    { key: { location: '2dsphere' }, name: 'location_2dsphere' },
    { key: { cityCode: 1, category: 1 }, name: 'cityCode_category' }
  ]
});

collections.push({
  // 【电子围栏：禁停/限行/服务区等围栏区域】
  name: 'geo_fences',
  required: ['name', 'cityCode', 'type', 'geometry', 'status', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    name: T('围栏名称'),  // 围栏名称
    cityCode: T('城市码'),  // 城市码
    type: C('围栏类型', ['no_parking', 'restricted', 'pickup_point', 'airport_pool', 'service_area']),  // 围栏类型
    geometry: PG('围栏区域'),  // 围栏区域
    status: C('状态', ['enabled', 'disabled']),  // 状态
    effectiveStart: D('生效开始'),  // 生效开始
    effectiveEnd: D('生效结束'),  // 生效结束
    action: C('命中动作', ['forbid_order', 'warn', 'allow']),  // 命中动作
    remark: T('备注'),  // 备注
    ...AUDIT
  },
  indexes: [
    { key: { cityCode: 1, type: 1 }, name: 'cityCode_type' },
    { key: { geometry: '2dsphere' }, name: 'geometry_2dsphere' }
  ]
});

collections.push({
  // 【搜索历史：乘客 POI 搜索历史，按次数倒序】
  name: 'search_history',
  required: ['userId', 'keyword', 'searchCount', 'lastSearchAt', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    userId: R('用户 ID'),  // 用户 ID
    poiId: RN('命中 POI'),  // 命中 POI
    keyword: T('搜索关键词'),  // 搜索关键词
    searchCount: I('搜索次数（倒序展示）'),  // 搜索次数（倒序展示）
    lastSearchAt: D('最近搜索时间'),  // 最近搜索时间
    ...AUDIT
  },
  indexes: [
    { key: { userId: 1, keyword: 1 }, unique: true, name: 'userId_keyword' },
    { key: { userId: 1, searchCount: -1 }, name: 'userId_searchCount' }
  ]
});

collections.push({
  // 【行程轨迹点：司机位置上报轨迹，高频写入按月归档】
  name: 'trajectories',
  required: ['orderId', 'driverId', 'location', 'pointType', 'reportedAt', 'source',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    orderId: R('订单 ID'),  // 订单 ID
    driverId: R('司机 ID'),  // 司机 ID
    location: P('上报位置'),  // 上报位置
    speed: N('瞬时速度 km/h'),  // 瞬时速度 km/h
    bearing: N('方向角'),  // 方向角
    accuracy: N('定位精度（米）'),  // 定位精度（米）
    mileage: N('累计里程（米）'),  // 累计里程（米）
    pointType: C('轨迹点类型', ['pickup', 'service']),  // 轨迹点类型
    reportedAt: D('上报时间'),  // 上报时间
    collectedAt: D('采集时间'),  // 采集时间
    source: C('来源', ['app', 'device']),  // 来源
    ...AUDIT
  },
  indexes: [
    { key: { orderId: 1, reportedAt: 1 }, name: 'orderId_reportedAt' },
    { key: { driverId: 1, reportedAt: -1 }, name: 'driverId_reportedAt' },
    { key: { location: '2dsphere' }, name: 'location_2dsphere' }
  ]
});

// ============================ 5. 订单与调度域 ============================
collections.push({
  // 【产品线(车型)：快车/优享/专车/拼车等车型配置】
  name: 'products',
  required: ['code', 'name', 'seatCapacity', 'priceRatio', 'isFixedPrice', 'cityCodes',
    'status', 'sort', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    code: C('产品线 code', ['express', 'discount', 'premium', 'luxury', 'carpool', 'taxi']),  // 产品线 code
    name: T('展示名称'),  // 展示名称
    iconUrl: T('车型图标'),  // 车型图标
    seatCapacity: I('座位数', 1),  // 座位数
    priceRatio: N('相对基准价系数'),  // 相对基准价系数
    isFixedPrice: B('是否一口价模式'),  // 是否一口价模式
    cityCodes: A('开通城市', T('城市码')),  // 开通城市
    status: C('状态', ['enabled', 'disabled']),  // 状态
    sort: I('展示排序'),  // 展示排序
    description: T('说明文案'),  // 说明文案
    ...AUDIT
  },
  indexes: [
    { key: { code: 1 }, unique: true, name: 'code_1' },
    { key: { status: 1, sort: 1 }, name: 'status_sort' }
  ]
});

// 订单（核心集合）
collections.push({
  // 【订单：核心订单，含行程状态、账单状态、计价、司机车辆快照】
  name: 'orders',
  required: ['orderNo', 'bizType', 'productCode', 'cityCode', 'userId', 'passengerPhoneEnc',
    'startLocation', 'startAddress', 'endLocation', 'endAddress', 'estimateDistance',
    'estimateDuration', 'estimateFee', 'status', 'tripState', 'billState', 'surgeRatio',
    'surgeFee', 'discountFee', 'extraFee', 'totalFee', 'payStatus', 'isNoPassPay', 'isShared',
    'riskLevel', 'source', 'invoiceStatus', 'isReviewed', 'version', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    orderNo: T('订单号 HZ+yyyyMMdd+序列'),  // 订单号 HZ+yyyyMMdd+序列
    bizType: C('业务类型', ['instant', 'reserve', 'airport', 'proxy', 'carpool', 'enterprise']),  // 业务类型
    productCode: T('产品线 code'),  // 产品线 code
    cityCode: T('下单城市'),  // 下单城市
    // 乘客侧
    userId: R('下单乘客'),  // 下单乘客
    passengerPhoneEnc: T('乘车人电话'),  // 乘车人电话
    passengerName: T('乘车人姓名'),  // 乘车人姓名
    isProxy: B('是否代叫车'),  // 是否代叫车
    // 起终点
    startLocation: P('上车点坐标'),  // 上车点坐标
    startAddress: T('上车点地址'),  // 上车点地址
    startPoiId: T('上车点 POI'),  // 上车点 POI
    startHouseNumber: T('门牌号'),  // 门牌号
    endLocation: P('目的地坐标'),  // 目的地坐标
    endAddress: T('目的地地址'),  // 目的地地址
    endPoiId: T('目的地 POI'),  // 目的地 POI
    endHouseNumber: T('门牌号'),  // 门牌号
    estimateDistance: I('预估里程（米）'),  // 预估里程（米）
    estimateDuration: I('预估时长（秒）'),  // 预估时长（秒）
    estimateFee: M('预估价（分）'),  // 预估价（分）
    fixedPrice: M('一口价（分）'),  // 一口价（分）
    // 司机侧
    driverId: R('接单司机'),  // 接单司机
    vehicleId: R('接单车辆'),  // 接单车辆
    plateNo: T('车牌快照'),  // 车牌快照
    driverPhoneEnc: T('司机电话（隐私号）'),  // 司机电话（隐私号）
    virtualNumber: T('虚拟中间号'),  // 虚拟中间号
    virtualNumberExpireAt: D('中间号有效期'),  // 中间号有效期
    driverSnapshot: O('司机与车辆快照', {  // 司机与车辆快照
      nickname: T('昵称'), avatar: T('头像'), phoneMask: T('脱敏手机号'),
      serviceScore: N('服务分'), plateNumber: T('车牌'), model: T('车型'), color: T('颜色')
    }),
    // 状态机（业务汇总 §4.2：行程状态 + 账单状态分离）
    status: C('兼容状态', TRIP_STATE),  // 兼容状态
    tripState: C('行程状态', TRIP_STATE),  // 行程状态
    billState: C('账单状态', BILL_STATE),  // 账单状态
    version: I('乐观锁'),  // 乐观锁
    cancelSide: C('取消责任方', ['passenger', 'driver', 'system', 'cs']),  // 取消责任方
    cancelReasonCode: T('取消原因码'),  // 取消原因码
    cancelFee: M('取消费（分）'),  // 取消费（分）
    // 时间节点
    reserveTime: D('预约用车时间'),  // 预约用车时间
    createTime: D('下单时间'),  // 下单时间
    acceptTime: D('接单时间'),  // 接单时间
    arriveTime: D('到达时间'),  // 到达时间
    boardTime: D('上车时间'),  // 上车时间
    endServiceTime: D('行程结束时间'),  // 行程结束时间
    payTime: D('支付完成时间'),  // 支付完成时间
    cancelTime: D('取消时间'),  // 取消时间
    finishTime: D('终态时间'),  // 终态时间
    // 实际里程时长
    pickupDistance: I('接驾里程（米）'),  // 接驾里程（米）
    pickupDuration: I('接驾时长（秒）'),  // 接驾时长（秒）
    actualDistance: I('实际行驶里程（米）'),  // 实际行驶里程（米）
    actualDuration: I('实际行驶时长（秒）'),  // 实际行驶时长（秒）
    waitDuration: I('免费等待时长（秒）'),  // 免费等待时长（秒）
    paidWaitDuration: I('计费等待时长（秒）'),  // 计费等待时长（秒）
    // 计价（业务汇总 §4.3：Q/H/K/F/Cp/Cd/P 字段）
    pricingRuleId: R('命中计价规则'),  // 命中计价规则
    pricingSnapshotId: R('计价快照'),  // 计价快照
    quoteId: R('关联估价单 estimates；一张估价单只能成一单'),  // 关联估价单 estimates；一张估价单只能成一单
    surgeRatio: N('动态调价系数'),  // 动态调价系数
    surgeFee: M('溢价金额（分）'),  // 溢价金额（分）
    couponId: R('使用券 ID'),  // 使用券 ID
    discountFee: M('优惠金额（分）'),  // 优惠金额（分）
    extraFee: M('附加费（分）'),  // 附加费（分）
    totalFee: M('原车费 F（分）'),  // 原车费 F（分）
    payFee: M('实付金额 P（分）'),  // 实付金额 P（分）
    payAmount: M('券后应付（分）'),  // 券后应付（分）
    driverIncome: M('司机收入 D（分）'),  // 司机收入 D（分）
    driverNetIncome: M('司机实收（分）'),  // 司机实收（分）
    commissionFee: M('平台抽成 K（分）'),  // 平台抽成 K（分）
    subsidyFee: M('平台补贴（分）'),  // 平台补贴（分）
    // 支付
    payStatus: C('支付状态', ['unpaid', 'paying', 'paid', 'refunding', 'refunded', 'failed']),  // 支付状态
    payChannel: C('支付渠道', ['wechat', 'alipay', 'balance', 'enterprise', 'nopass']),  // 支付渠道
    paymentId: R('关联支付单'),  // 关联支付单
    settlementPaymentId: R('唯一有效结清支付单（防重复扣款）'),  // 唯一有效结清支付单（防重复扣款）
    isNoPassPay: B('是否免密支付'),  // 是否免密支付
    // 服务与安全
    passengerRemark: T('乘客备注（≤50 字）'),  // 乘客备注（≤50 字）
    recordUrl: T('行程录音地址'),  // 行程录音地址
    isShared: B('是否开启行程分享'),  // 是否开启行程分享
    safetyFlags: A('安全标记', C('标记', ['deviation', 'long_stop', 'night', 'sos'])),  // 安全标记
    riskLevel: C('风险等级', ['low', 'medium', 'high']),  // 风险等级
    // 拼车/预约扩展
    flightNo: T('航班号'),  // 航班号
    carpoolGroupId: R('拼车组 ID'),  // 拼车组 ID
    carpoolSeatCount: I('拼座数'),  // 拼座数
    taskId: R('关联运输任务'),  // 关联运输任务
    queueId: R('关联排队记录'),  // 关联排队记录
    // 其他
    source: C('下单来源', ['app', 'h5', 'wechat_mini', 'cp', 'cs']),  // 下单来源
    deviceId: T('下单设备'),  // 下单设备
    invoiceStatus: C('开票状态', ['none', 'applying', 'issued']),  // 开票状态
    isReviewed: B('是否已评价'),  // 是否已评价
    partnerId: R('渠道归属'),  // 渠道归属
    ...AUDIT
  },
  indexes: [
    { key: { orderNo: 1 }, unique: true, name: 'orderNo_1' },
    { key: { userId: 1, createTime: -1 }, name: 'userId_createTime' },
    { key: { driverId: 1, createTime: -1 }, name: 'driverId_createTime' },
    { key: { tripState: 1, cityCode: 1, createTime: -1 }, name: 'tripState_cityCode' },
    { key: { taskId: 1 }, name: 'taskId_1' },
    { key: { startLocation: '2dsphere' }, name: 'startLocation_2dsphere' },
    { key: { billState: 1, createTime: -1 }, name: 'billState_createTime' },
    { key: { quoteId: 1 }, name: 'quoteId_1' }
  ]
});

// 运输任务（业务汇总 §4.2：普通一单、拼车最多两单，司机占用依据）【缺口新增】
collections.push({
  // 【运输任务：普通一单/拼车最多两单，司机占用依据】
  name: 'transport_tasks',
  required: ['taskNo', 'orderIds', 'cityCode', 'productCode', 'driverId', 'state', 'version',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    taskNo: T('运输任务号'),  // 运输任务号
    orderIds: A('组内订单（普通 1 个，拼车最多 2 个）', R('订单 ID'), 2),  // 组内订单（普通 1 个，拼车最多 2 个）
    cityCode: T('城市码'),  // 城市码
    productCode: T('产品线'),  // 产品线
    driverId: R('分配司机'),  // 分配司机
    state: C('运输任务状态', TASK_STATE),  // 运输任务状态
    carpoolGroupId: R('拼车组 ID（拼车时）'),  // 拼车组 ID（拼车时）
    boardedAt: D('拼车发车时间'),  // 拼车发车时间
    finishedAt: D('任务终结时间'),  // 任务终结时间
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { taskNo: 1 }, unique: true, name: 'taskNo_1' },
    { key: { driverId: 1, state: 1 }, name: 'driverId_state' },
    { key: { orderIds: 1 }, name: 'orderIds_1' }
  ]
});

collections.push({
  // 【订单状态流转日志：订单状态变更留痕（时间线）】
  name: 'order_status_logs',
  required: ['orderId', 'toStatus', 'operatorType', 'occurredAt', 'version', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    orderId: R('订单 ID'),  // 订单 ID
    fromStatus: T('变更前状态'),  // 变更前状态
    toStatus: T('变更后状态'),  // 变更后状态
    operatorType: C('操作人类型', ['passenger', 'driver', 'system', 'admin', 'job']),  // 操作人类型
    operatorId: RN('操作人 ID'),  // 操作人 ID
    reason: T('变更原因'),  // 变更原因
    extra: O('附加数据', { location: P('坐标'), distance: N('距离') }),  // 附加数据
    occurredAt: D('发生时间'),  // 发生时间
    ...AUDIT, ...LOCK
  },
  indexes: [{ key: { orderId: 1, occurredAt: 1 }, name: 'orderId_occurredAt' }]
});

collections.push({
  // 【取消记录：取消责任方、取消费与信用影响】
  name: 'order_cancels',
  required: ['orderId', 'cancelSide', 'reasonCode', 'orderStatusAtCancel', 'freeCancel',
    'cancelFee', 'actualChargeFee', 'charged', 'affectCredit', 'cancelTime',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    orderId: R('订单 ID'),  // 订单 ID
    cancelSide: C('取消责任方', ['passenger', 'driver', 'system', 'cs']),  // 取消责任方
    reasonCode: T('原因码'),  // 原因码
    reasonText: T('原因补充'),  // 原因补充
    orderStatusAtCancel: T('取消时订单状态'),  // 取消时订单状态
    freeCancel: B('是否免费取消时限内'),  // 是否免费取消时限内
    cancelFee: M('应收取消费（分）'),  // 应收取消费（分）
    actualChargeFee: M('实收取消费（分）'),  // 实收取消费（分）
    charged: B('是否已扣款'),  // 是否已扣款
    affectCredit: B('是否影响信用/服务分'),  // 是否影响信用/服务分
    scoreDelta: N('分值变动'),  // 分值变动
    operatorId: RN('操作人（客服代取消）'),  // 操作人（客服代取消）
    cancelTime: D('取消时间'),  // 取消时间
    ...AUDIT
  },
  indexes: [
    { key: { orderId: 1 }, unique: true, name: 'orderId_1' },
    { key: { cancelSide: 1, cancelTime: -1 }, name: 'cancelSide_cancelTime' },
    { key: { reasonCode: 1 }, name: 'reasonCode_1' }
  ]
});

// 派单记录（业务汇总 A06/D01/D02：听单/抢单/预约）
collections.push({
  // 【派单记录：每轮派单候选、推送与接单结果】
  name: 'dispatches',
  required: ['orderId', 'round', 'strategy', 'searchRadius', 'candidateCount',
    'result', 'elapsedMs', 'startedAt', 'endedAt', 'version', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    orderId: R('订单 ID'),  // 订单 ID
    taskId: R('运输任务'),  // 运输任务
    round: I('第几轮派单'),  // 第几轮派单
    strategy: C('派单策略', ['assign', 'grab', 'reserve']),  // 派单策略
    mode: C('派单方式', ['dispatch', 'grab']),  // 派单方式
    searchRadius: I('本轮搜索半径（米）'),  // 本轮搜索半径（米）
    candidateCount: I('召回司机数'),  // 召回司机数
    candidates: A('候选明细', O('候选', {  // 候选明细
      driverId: R('司机'), distance: N('距离'), duration: N('时长'), score: N('评分'), rank: I('排名')
    })),
    broadcasts: A('推送明细', O('推送', {  // 推送明细
      driverId: R('司机'), pushedAt: D('推送时间'), respondedAt: D('响应时间'),
      result: C('结果', ['accept', 'reject', 'timeout']), score: N('评分')
    })),
    acceptedDriverId: R('最终接单司机'),  // 最终接单司机
    result: C('派单结果', ['accepted', 'timeout', 'no_candidate', 'cancelled']),  // 派单结果
    elapsedMs: I('本轮耗时'),  // 本轮耗时
    operatorId: RN('人工干预操作人'),  // 人工干预操作人
    operatorType: C('操作类型', ['system', 'admin']),  // 操作类型
    queueId: R('排队叫号触发'),  // 排队叫号触发
    startedAt: D('开始'),  // 开始
    endedAt: D('结束'),  // 结束
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { orderId: 1, round: 1 }, name: 'orderId_round' },
    { key: { result: 1, createdAt: -1 }, name: 'result_createdAt' },
    { key: { acceptedDriverId: 1 }, name: 'acceptedDriverId_1' }
  ]
});

// 派单尝试/预占（业务汇总 §5.1「邀请及预占」：15s 邀请、抢单锁）【缺口新增】
collections.push({
  // 【派单邀请/预占：15s 邀请、抢单锁、过期回收】
  name: 'dispatch_attempts',
  required: ['attemptNo', 'taskId', 'orderId', 'driverId', 'mode', 'status',
    'dispatchRevision', 'expiresAt', 'version', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    attemptNo: T('尝试号（attempt_id）'),  // 尝试号（attempt_id）
    taskId: R('运输任务'),  // 运输任务
    orderId: R('订单'),  // 订单
    driverId: R('司机'),  // 司机
    mode: C('方式', ['dispatch', 'grab']),  // 方式
    status: C('状态', ['pushed', 'accepted', 'rejected', 'timeout', 'cancelled']),  // 状态
    dispatchRevision: I('派单版本；过期确认无效'),  // 派单版本；过期确认无效
    distanceKm: N('司机距起点距离（公里）'),  // 司机距起点距离（公里）
    pushedAt: D('推送时间'),  // 推送时间
    acceptedAt: D('接单时间'),  // 接单时间
    expiresAt: D('截止时间；超时回收重新派单'),  // 截止时间；超时回收重新派单
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { attemptNo: 1 }, unique: true, name: 'attemptNo_1' },
    { key: { taskId: 1, driverId: 1 }, unique: true, name: 'taskId_driverId' },
    { key: { status: 1, expiresAt: 1 }, name: 'status_expiresAt' }
  ]
});

collections.push({
  // 【高峰排队队列：高峰/恶劣天气排队叫号】
  name: 'dispatch_queues',
  required: ['queueNo', 'orderId', 'userId', 'cityCode', 'productCode', 'queuePosition',
    'estimatedWaitSec', 'status', 'joinedAt', 'expireAt', 'notifyCount', 'priority',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    queueNo: T('排队号'),  // 排队号
    orderId: R('订单'),  // 订单
    userId: R('排队乘客'),  // 排队乘客
    cityCode: T('城市码'),  // 城市码
    productCode: T('产品线'),  // 产品线
    queuePosition: I('排队位次'),  // 排队位次
    estimatedWaitSec: I('预计等待秒数'),  // 预计等待秒数
    status: C('排队状态', ['waiting', 'called', 'matched', 'expired', 'left']),  // 排队状态
    joinedAt: D('入队时间'),  // 入队时间
    calledAt: D('叫号时间'),  // 叫号时间
    expireAt: D('出队过期（TTL）'),  // 出队过期（TTL）
    notifyCount: I('已通知次数'),  // 已通知次数
    dispatchId: R('关联派单记录'),  // 关联派单记录
    priority: I('优先级'),  // 优先级
    ...AUDIT
  },
  indexes: [
    { key: { queueNo: 1 }, unique: true, name: 'queueNo_1' },
    { key: { cityCode: 1, productCode: 1, status: 1, queuePosition: 1 }, name: 'city_product_status_pos' },
    { key: { expireAt: 1 }, expireAfterSeconds: 0, name: 'expireAt_ttl' }
  ]
});

collections.push({
  // 【派单规则配置：召回半径、打分权重、轮次与超时】
  name: 'dispatch_rules',
  required: ['ruleCode', 'cityCode', 'productCode', 'searchRadiusStart', 'searchRadiusStep',
    'searchRadiusMax', 'maxRounds', 'roundTimeoutSec', 'scoreWeights', 'requireQualified',
    'priority', 'enabled', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    ruleCode: T('规则编码'),  // 规则编码
    cityCode: T('城市'),  // 城市
    productCode: T('产品线（* 全部）'),  // 产品线（* 全部）
    searchRadiusStart: I('首轮召回半径'),  // 首轮召回半径
    searchRadiusStep: I('扩圈步长'),  // 扩圈步长
    searchRadiusMax: I('召回上限半径'),  // 召回上限半径
    maxRounds: I('最大轮次'),  // 最大轮次
    roundTimeoutSec: I('单轮推送超时（秒）'),  // 单轮推送超时（秒）
    scoreWeights: O('打分权重（和=10000）', {  // 打分权重（和=10000）
      distance: I('距离'), serviceScore: I('服务分'), acceptRate: I('接单率'),
      onlineMinutes: I('在线时长'), detour: I('顺路度'), bigOrder: I('大单')
    }),
    requireQualified: B('仅召回资质有效司机'),  // 仅召回资质有效司机
    maxPickupDistance: I('最大接驾距离'),  // 最大接驾距离
    priority: I('命中优先级'),  // 命中优先级
    enabled: B('是否启用'),  // 是否启用
    effectiveStart: D('生效开始'),  // 生效开始
    effectiveEnd: D('生效结束'),  // 生效结束
    ...AUDIT
  },
  indexes: [
    { key: { ruleCode: 1 }, unique: true, name: 'ruleCode_1' },
    { key: { cityCode: 1, productCode: 1, enabled: 1 }, name: 'city_product_enabled' }
  ]
});

// 改点申请（业务汇总 P10：每单最多 2 次成功变更，司机 30 秒确认）【缺口新增】
collections.push({
  // 【中途改点申请：每单最多 2 次，司机 30 秒确认】
  name: 'destination_changes',
  required: ['orderId', 'newEndLocation', 'newEndAddress', 'newEstimateFee', 'status',
    'requestedAt', 'expiresAt', 'version', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    orderId: R('订单'),  // 订单
    newEndLocation: P('新目的地坐标'),  // 新目的地坐标
    newEndAddress: T('新目的地地址'),  // 新目的地地址
    newEndPoiId: T('新目的地 POI'),  // 新目的地 POI
    newEstimateFee: M('变更后预估金额（分）'),  // 变更后预估金额（分）
    status: C('状态', ['pending', 'accepted', 'rejected', 'timeout', 'cancelled']),  // 状态
    requestedAt: D('申请时间'),  // 申请时间
    confirmedAt: D('司机确认时间'),  // 司机确认时间
    expiresAt: D('30 秒确认截止'),  // 30 秒确认截止
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { orderId: 1, status: 1 }, name: 'orderId_status' },
    { key: { expiresAt: 1 }, name: 'expiresAt_1' }
  ]
});

// 取消报价（业务汇总 P12：30 秒有效取消报价）【缺口新增】
collections.push({
  // 【取消报价：30 秒有效取消报价，绑定订单版本】
  name: 'cancel_quotes',
  required: ['orderId', 'quoteNo', 'cancelFee', 'boundVersion', 'expiresAt', 'used',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    orderId: R('订单'),  // 订单
    quoteNo: T('报价单号'),  // 报价单号
    cancelFee: M('取消报价金额（分）'),  // 取消报价金额（分）
    boundVersion: I('绑定订单版本；状态变化则重新报价'),  // 绑定订单版本；状态变化则重新报价
    reasonCode: T('取消原因码'),  // 取消原因码
    expiresAt: D('30 秒有效期'),  // 30 秒有效期
    used: B('是否已确认消费'),  // 是否已确认消费
    ...AUDIT
  },
  indexes: [
    { key: { quoteNo: 1 }, unique: true, name: 'quoteNo_1' },
    { key: { orderId: 1, expiresAt: 1 }, name: 'orderId_expiresAt' }
  ]
});

// 行程分享（业务汇总 P11：令牌 24h、结束 30 分钟失效、限流）【缺口新增】
collections.push({
  // 【行程分享：只读分享令牌，最长 24 小时】
  name: 'trip_shares',
  required: ['orderId', 'shareToken', 'creatorId', 'expireAt', 'revoked', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    orderId: R('订单'),  // 订单
    shareToken: T('不可预测分享令牌'),  // 不可预测分享令牌
    creatorId: R('创建乘客'),  // 创建乘客
    expireAt: D('最长 24 小时，结束/取消 30 分钟后失效'),  // 最长 24 小时，结束/取消 30 分钟后失效
    revoked: B('是否已撤销'),  // 是否已撤销
    accessCount: I('访问次数（限流）'),  // 访问次数（限流）
    ...AUDIT
  },
  indexes: [
    { key: { shareToken: 1 }, unique: true, name: 'shareToken_1' },
    { key: { orderId: 1 }, name: 'orderId_1' },
    { key: { expireAt: 1 }, expireAfterSeconds: 0, name: 'expireAt_ttl' }
  ]
});

// 虚拟号（业务汇总 P09/D07：订单存续 +7 天有效）【缺口新增】
collections.push({
  // 【虚拟号绑定：中间虚拟号，订单存续 +7 天有效】
  name: 'virtual_numbers',
  required: ['orderId', 'virtualNo', 'status', 'boundAt', 'expireAt', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    orderId: R('订单'),  // 订单
    passengerId: R('乘客'),  // 乘客
    driverId: R('司机'),  // 司机
    virtualNo: T('中间虚拟号码'),  // 中间虚拟号码
    status: C('状态', ['bound', 'released', 'expired']),  // 状态
    boundAt: D('绑定时间'),  // 绑定时间
    releasedAt: D('释放时间'),  // 释放时间
    expireAt: D('失效时间（存续 +7 天）'),  // 失效时间（存续 +7 天）
    ...AUDIT
  },
  indexes: [
    { key: { orderId: 1 }, unique: true, name: 'orderId_1' },
    { key: { expireAt: 1 }, name: 'expireAt_1' }
  ]
});

// ============================ 6. 计价域 ============================
collections.push({
  // 【计价规则：起步/里程/时长/佣金/封顶，版本化发布】
  name: 'pricing_rules',
  required: ['ruleNo', 'name', 'cityCode', 'productCode', 'version', 'status', 'effectiveTime',
    'baseFee', 'baseDistance', 'minFee', 'waitingFreeMinutes', 'waitingFeePerMin', 'commissionRate',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    ruleNo: T('规则编号'),  // 规则编号
    name: T('规则名称'),  // 规则名称
    cityCode: T('适用城市'),  // 适用城市
    productCode: T('适用产品线'),  // 适用产品线
    version: I('规则版本（递增）'),  // 规则版本（递增）
    status: C('状态', ['draft', 'effective', 'expired']),  // 状态
    effectiveTime: D('生效时间'),  // 生效时间
    expireTime: D('失效时间'),  // 失效时间
    baseFee: M('起步价（分）'),  // 起步价（分）
    baseDistance: I('起步里程（米）'),  // 起步里程（米）
    baseDuration: I('起步时长（秒）'),  // 起步时长（秒）
    mileageSegments: A('分段里程', O('段', { fromKm: N('起'), toKm: N('止'), pricePerKm: M('价') })),  // 分段里程
    durationRules: A('时长费', O('规则', { period: C('时段', ['peak', 'flat', 'night']), start: T('始'), end: T('止'), pricePerMin: M('价') })),  // 时长费
    longDistanceRule: O('远途费', { fromKm: N('起'), pricePerKm: M('价') }),  // 远途费
    lowSpeedRule: O('低速费', { speedThreshold: N('阈值'), pricePerMin: M('价') }),  // 低速费
    nightRule: O('夜间费', { start: T('始'), end: T('止'), extraPerKm: M('价') }),  // 夜间费
    minFee: M('最低消费（分）'),  // 最低消费（分）
    maxFee: M('封顶价（分）'),  // 封顶价（分）
    waitingFreeMinutes: I('免费等待分钟'),  // 免费等待分钟
    waitingFeePerMin: M('超时等待费（分/分钟）'),  // 超时等待费（分/分钟）
    extraFeeTypes: A('允许附加费', C('类型', ['toll', 'parking', 'highway'])),  // 允许附加费
    commissionRate: N('平台抽成比例 0~1'),  // 平台抽成比例 0~1
    driverMinIncomePerOrder: M('司机单均保底'),  // 司机单均保底
    subsidyRule: O('补贴规则', { type: T('类型'), value: M('值') }),  // 补贴规则
    remark: T('备注'),  // 备注
    ...AUDIT
  },
  indexes: [
    { key: { ruleNo: 1 }, unique: true, name: 'ruleNo_1' },
    { key: { cityCode: 1, productCode: 1, status: 1, effectiveTime: -1 }, name: 'city_product_status_time' }
  ]
});

collections.push({
  // 【计价规则快照：下单时固化计价规则，保证历史可追溯】
  name: 'pricing_snapshots',
  required: ['orderId', 'ruleId', 'ruleVersion', 'snapshot', 'surgeRatio', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    orderId: R('订单 ID'),  // 订单 ID
    ruleId: R('来源规则'),  // 来源规则
    ruleVersion: I('规则版本'),  // 规则版本
    snapshot: O('完整规则快照', {}),  // 完整规则快照
    surgeRatio: N('下单时溢价系数'),  // 下单时溢价系数
    ...AUDIT
  },
  indexes: [{ key: { orderId: 1 }, unique: true, name: 'orderId_1' }]
});

collections.push({
  // 【动态调价规则：高峰/区域溢价与监管上限】
  name: 'surge_rules',
  required: ['cityCode', 'productCode', 'demandSupplyRatio', 'surgeRatio', 'maxSurgeRatio',
    'enabled', 'sort', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    cityCode: T('城市'),  // 城市
    productCode: T('产品线'),  // 产品线
    geoHash: T('区域网格'),  // 区域网格
    demandSupplyRatio: N('供需比阈值'),  // 供需比阈值
    surgeRatio: N('溢价系数'),  // 溢价系数
    maxSurgeRatio: N('监管上限'),  // 监管上限
    timeWindow: O('时段', { start: T('始'), end: T('止') }),  // 时段
    weatherTrigger: A('天气触发', C('天气', ['rain', 'snow', 'extreme'])),  // 天气触发
    enabled: B('是否启用'),  // 是否启用
    sort: I('优先级'),  // 优先级
    ...AUDIT
  },
  indexes: [
    { key: { cityCode: 1, productCode: 1 }, name: 'city_product' },
    { key: { enabled: 1 }, name: 'enabled_1' }
  ]
});

collections.push({
  // 【取消费规则：免费取消时限与取消费配置】
  name: 'cancel_rules',
  required: ['cityCode', 'productCode', 'cancelSide', 'statusFrom', 'freeWindowSeconds',
    'feeType', 'feeValue', 'enabled', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    cityCode: T('城市'),  // 城市
    productCode: T('产品线'),  // 产品线
    cancelSide: C('责任方', ['passenger', 'driver', 'system', 'cs']),  // 责任方
    statusFrom: T('取消时订单状态'),  // 取消时订单状态
    freeWindowSeconds: I('免费取消时限（秒）'),  // 免费取消时限（秒）
    feeType: C('费用类型', ['fixed', 'percent', 'none']),  // 费用类型
    feeValue: M('固定金额或百分比'),  // 固定金额或百分比
    maxFee: M('封顶金额'),  // 封顶金额
    enabled: B('是否启用'),  // 是否启用
    ...AUDIT
  },
  indexes: [{ key: { cityCode: 1, productCode: 1 }, name: 'city_product' }]
});

// 估价单（业务汇总 P06：5 分钟有效 + 下单幂等键）【缺口新增】
collections.push({
  // 【估价单：预估价，5 分钟有效，下单幂等键】
  name: 'estimates',
  required: ['quoteNo', 'userId', 'serviceType', 'estimatedAmount', 'status', 'expiresAt',
    'createdAt', 'updatedAt', 'isDeleted', 'version'],
  fields: {
    quoteNo: T('估价单号，全局唯一，下单幂等键'),  // 估价单号，全局唯一，下单幂等键
    userId: R('用户'),  // 用户
    serviceType: C('车型', ['express', 'premium', 'carpool']),  // 车型
    origin: P('上车点'),  // 上车点
    destination: P('目的地'),  // 目的地
    originAddress: T('上车点地址'),  // 上车点地址
    destinationAddress: T('目的地地址'),  // 目的地地址
    estimatedAmount: M('预估金额（分）'),  // 预估金额（分）
    pricingRuleId: R('计价规则'),  // 计价规则
    distanceKm: N('预估里程（公里）'),  // 预估里程（公里）
    durationMinutes: N('预估时长（分钟）'),  // 预估时长（分钟）
    couponId: R('命中的券'),  // 命中的券
    couponDiscount: M('券抵扣金额（分）'),  // 券抵扣金额（分）
    payAmount: M('券后应付（分）'),  // 券后应付（分）
    status: C('状态', ['unused', 'used', 'expired']),  // 状态
    usedOrderId: R('消费该估价单的订单'),  // 消费该估价单的订单
    expiresAt: D('失效时间（5 分钟 TTL）'),  // 失效时间（5 分钟 TTL）
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { quoteNo: 1 }, unique: true, name: 'quoteNo_1' },
    { key: { expiresAt: 1 }, expireAfterSeconds: 0, name: 'expiresAt_ttl' },
    { key: { userId: 1, status: 1 }, name: 'userId_status' }
  ]
});

// ============================ 7. 账单与支付域 ============================
collections.push({
  // 【账单：正式账单，含分账字段 F/Q/H/K/Cp/Cd/P/D/N】
  name: 'bills',
  required: ['billNo', 'orderId', 'userId', 'cityCode', 'productCode', 'billType',
    'totalFee', 'discountFee', 'payFee', 'refundFee', 'driverIncome', 'commissionFee',
    'subsidyFee', 'state', 'version', 'settled', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    billNo: T('账单号'),  // 账单号
    orderId: R('订单 ID'),  // 订单 ID
    userId: R('乘客 ID'),  // 乘客 ID
    driverId: R('司机 ID'),  // 司机 ID
    cityCode: T('城市'),  // 城市
    productCode: T('产品线'),  // 产品线
    billType: C('账单类型', ['normal', 'cancel', 'extra', 'refund']),  // 账单类型
    // 分账字段（业务汇总 §4.3：F/Q/H/K/Cp/Cd/P/D/N）
    totalFee: M('原车费 F（分）'),  // 原车费 F（分）
    surgeFee: M('溢价后车费 Q 中的溢价部分'),  // 溢价后车费 Q 中的溢价部分
    serviceFee: M('乘客服务费 H（分）'),  // 乘客服务费 H（分）
    commissionFee: M('司机佣金 K（分）'),  // 司机佣金 K（分）
    platformCoupon: M('平台承担券 Cp（分）'),  // 平台承担券 Cp（分）
    driverCoupon: M('司机承担券 Cd（分）'),  // 司机承担券 Cd（分）
    discountFee: M('优惠总额（分）'),  // 优惠总额（分）
    payFee: M('乘客实付 P（分）'),  // 乘客实付 P（分）
    refundFee: M('已退金额（分）'),  // 已退金额（分）
    driverIncome: M('司机应得 D（分）'),  // 司机应得 D（分）
    platformNet: M('平台净额 N（分）'),  // 平台净额 N（分）
    channelFee: M('渠道手续费 G（分）'),  // 渠道手续费 G（分）
    subsidyFee: M('补贴（分）'),  // 补贴（分）
    state: C('账单状态', BILL_STATE),  // 账单状态
    version: I('乐观锁'),  // 乐观锁
    settled: B('是否已结算给司机'),  // 是否已结算给司机
    settlementId: R('结算单 ID'),  // 结算单 ID
    remark: T('备注'),  // 备注
    ...AUDIT
  },
  indexes: [
    { key: { billNo: 1 }, unique: true, name: 'billNo_1' },
    { key: { orderId: 1 }, unique: true, name: 'orderId_1' },
    { key: { userId: 1, createdAt: -1 }, name: 'userId_createdAt' },
    { key: { state: 1, settled: 1 }, name: 'state_settled' }
  ]
});

collections.push({
  // 【账单明细项：起步/里程/时长/附加费等逐项明细】
  name: 'bill_items',
  required: ['billId', 'orderId', 'itemType', 'itemName', 'amount', 'sort', 'version',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    billId: R('账单 ID'),  // 账单 ID
    orderId: R('订单 ID'),  // 订单 ID
    itemType: C('明细类型', ['base', 'mileage', 'duration', 'long_distance', 'waiting', 'night',  // 明细类型
      'surge', 'toll', 'parking', 'highway', 'cancel', 'coupon', 'member_discount', 'adjust']),
    itemName: T('展示名称'),  // 展示名称
    amount: M('金额（分，优惠类为负）'),  // 金额（分，优惠类为负）
    formula: T('计算式'),  // 计算式
    quantity: N('数量'),  // 数量
    unitPrice: M('单价'),  // 单价
    couponId: R('关联券'),  // 关联券
    sort: I('展示顺序'),  // 展示顺序
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { billId: 1, sort: 1 }, name: 'billId_sort' },
    { key: { orderId: 1 }, name: 'orderId_1' }
  ]
});

collections.push({
  // 【支付单：支付尝试、渠道交易号、幂等与关单】
  name: 'payments',
  required: ['payNo', 'orderId', 'userId', 'amount', 'channel', 'payScene', 'status',
    'idempotentKey', 'refundedAmount', 'notifyCount', 'version', 'expireAt', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    payNo: T('平台支付单号'),  // 平台支付单号
    orderId: R('订单 ID'),  // 订单 ID
    billId: R('账单 ID'),  // 账单 ID
    userId: R('付款用户'),  // 付款用户
    amount: M('支付金额（分）'),  // 支付金额（分）
    channel: C('渠道', ['wechat', 'alipay', 'balance', 'enterprise', 'nopass', 'zero']),  // 渠道
    payScene: C('场景', ['order_pay', 'cancel_fee', 'recharge', 'extra']),  // 场景
    status: C('支付状态', PAY_STATE),  // 支付状态
    idempotentKey: T('幂等键'),  // 幂等键
    channelTradeNo: T('渠道交易号'),  // 渠道交易号
    channelResponse: O('渠道回调原始报文', {}),  // 渠道回调原始报文
    prepayId: T('预支付会话 ID'),  // 预支付会话 ID
    paidAt: D('支付成功时间'),  // 支付成功时间
    expireAt: D('支付单过期（15 分钟关单）'),  // 支付单过期（15 分钟关单）
    refundedAmount: M('已退金额（分）'),  // 已退金额（分）
    failReason: T('失败原因'),  // 失败原因
    notifyCount: I('回调次数'),  // 回调次数
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { payNo: 1 }, unique: true, name: 'payNo_1' },
    { key: { idempotentKey: 1 }, unique: true, name: 'idempotentKey_1' },
    { key: { orderId: 1 }, name: 'orderId_1' },
    { key: { channelTradeNo: 1 }, name: 'channelTradeNo_1' },
    { key: { status: 1, createdAt: -1 }, name: 'status_createdAt' }
  ]
});

collections.push({
  // 【退款单：全额/部分/差额退款，责任分摊模式】
  name: 'refunds',
  required: ['refundNo', 'paymentId', 'orderId', 'amount', 'refundType', 'reasonCode',
    'refundTo', 'status', 'applicantType', 'couponReturned', 'version', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    refundNo: T('退款单号'),  // 退款单号
    paymentId: R('原支付单'),  // 原支付单
    orderId: R('订单 ID'),  // 订单 ID
    billId: R('账单 ID'),  // 账单 ID
    ticketId: R('发起退款工单'),  // 发起退款工单
    amount: M('退款金额（分）'),  // 退款金额（分）
    refundType: C('退款类型', ['full', 'partial', 'difference']),  // 退款类型
    reasonCode: T('退款原因码'),  // 退款原因码
    reasonText: T('原因说明'),  // 原因说明
    refundTo: C('退款去向', ['original', 'balance']),  // 退款去向
    status: C('退款状态', REFUND_STATE),  // 退款状态
    responsibilityMode: C('责任分摊模式', ['PROPORTIONAL', 'PLATFORM']),  // 责任分摊模式
    driverShare: M('司机承担额（分）'),  // 司机承担额（分）
    channelRefundNo: T('渠道退款号'),  // 渠道退款号
    applicantType: C('申请人类型', ['user', 'cs', 'system', 'admin']),  // 申请人类型
    applicantId: R('申请人'),  // 申请人
    approverId: R('审批人'),  // 审批人
    couponReturned: B('券是否退回'),  // 券是否退回
    refundedAt: D('退款完成时间'),  // 退款完成时间
    errorMessage: T('失败原因'),  // 失败原因
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { refundNo: 1 }, unique: true, name: 'refundNo_1' },
    { key: { orderId: 1 }, name: 'orderId_1' },
    { key: { status: 1, createdAt: -1 }, name: 'status_createdAt' }
  ]
});

// ============================ 8. 资金域（业务汇总 §4.4 资金五桶 + 复式记账） ============================
collections.push({
  // 【司机钱包：资金五桶 U/A/Fz/W/Debt 余额】
  name: 'driver_wallets',
  required: ['driverId', 'unsettled', 'available', 'frozen', 'withdrawing', 'debt',
    'totalIncome', 'totalWithdraw', 'version', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    driverId: R('司机 ID'),  // 司机 ID
    unsettled: M('待结算桶 U（分）'),  // 待结算桶 U（分）
    available: M('可用桶 A（分，可提现金额）'),  // 可用桶 A（分，可提现金额）
    frozen: M('冻结桶 Fz（分）'),  // 冻结桶 Fz（分）
    withdrawing: M('提现预占桶 W（分）'),  // 提现预占桶 W（分）
    debt: M('待追偿债务 Debt（分）'),  // 待追偿债务 Debt（分）
    balance: M('可提现余额快照（分，=A）'),  // 可提现余额快照（分，=A）
    frozenBalance: M('冻结金额快照（分）'),  // 冻结金额快照（分）
    totalIncome: M('累计收入（分）'),  // 累计收入（分）
    totalWithdraw: M('累计提现（分）'),  // 累计提现（分）
    todayIncome: M('当日收入（分）'),  // 当日收入（分）
    settleStatus: C('结算状态', ['normal', 'frozen']),  // 结算状态
    lastSettleAt: D('最近结算时间'),  // 最近结算时间
    ...AUDIT, ...LOCK
  },
  indexes: [{ key: { driverId: 1 }, unique: true, name: 'driverId_1' }]
});

// 资金桶流水（业务汇总 §4.4：U/A/Fz/W/Debt 互斥，不得重复扣减）【缺口新增】
collections.push({
  // 【资金桶变动流水：U/A/Fz/W/Debt 互斥桶变动记录】
  name: 'fund_bucket_movements',
  required: ['driverId', 'bucket', 'direction', 'amount', 'bizType', 'bizId', 'movementNo',
    'balanceAfter', 'version', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    driverId: R('司机 ID'),  // 司机 ID
    bucket: C('资金桶', FUND_BUCKET),  // 资金桶
    direction: C('方向', ['in', 'out']),  // 方向
    amount: M('变动金额（分，正数）'),  // 变动金额（分，正数）
    movementNo: T('桶变动唯一键'),  // 桶变动唯一键
    bizType: C('业务类型', ['settle', 'withdraw', 'freeze', 'unfreeze', 'refund_freeze',  // 业务类型
      'refund_charge', 'debt_repay', 'adjust']),
    bizId: R('关联业务单'),  // 关联业务单
    refundId: R('退款单（冻结来源）'),  // 退款单（冻结来源）
    sourceBucket: C('来源桶', FUND_BUCKET),  // 来源桶
    balanceAfter: M('变动后余额（分）'),  // 变动后余额（分）
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { movementNo: 1 }, unique: true, name: 'movementNo_1' },
    { key: { driverId: 1, bucket: 1, createdAt: -1 }, name: 'driverId_bucket' }
  ]
});

collections.push({
  // 【钱包流水：收入/奖励/罚款/提现/退款等流水】
  name: 'wallet_transactions',
  required: ['txNo', 'ownerType', 'ownerId', 'direction', 'amount', 'balanceAfter', 'bizType',
    'status', 'createdAt', 'updatedAt', 'isDeleted', 'version'],
  fields: {
    txNo: T('流水号'),  // 流水号
    ownerType: C('归属类型', ['driver', 'passenger', 'platform']),  // 归属类型
    ownerId: R('钱包所属人'),  // 钱包所属人
    direction: C('方向', ['in', 'out']),  // 方向
    amount: M('变动金额（分，正数）'),  // 变动金额（分，正数）
    balanceAfter: M('变动后余额（分）'),  // 变动后余额（分）
    bizType: C('流水类型', ['order_income', 'reward', 'penalty', 'withdraw', 'withdraw_fail',  // 流水类型
      'recharge', 'refund', 'adjust', 'subsidy']),
    bizId: R('关联业务 ID'),  // 关联业务 ID
    orderId: R('关联订单'),  // 关联订单
    description: T('描述'),  // 描述
    operatorId: RN('操作人'),  // 操作人
    status: C('状态', ['success', 'pending', 'failed']),  // 状态
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { txNo: 1 }, unique: true, name: 'txNo_1' },
    { key: { ownerType: 1, ownerId: 1, createdAt: -1 }, name: 'owner_createdAt' },
    { key: { bizType: 1, createdAt: -1 }, name: 'bizType_createdAt' }
  ]
});

// 复式记账分录（业务汇总 §4.4：借贷平衡，只新增不修改）【缺口新增】
collections.push({
  // 【账务分录：复式记账凭证，借贷平衡，只增不改】
  name: 'accounting_entries',
  required: ['entryId', 'voucherId', 'businessId', 'businessType', 'account', 'debit',
    'credit', 'currency', 'occurredAt', 'version', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    entryId: T('分录号'),  // 分录号
    voucherId: T('凭证号；同一凭证借贷平衡'),  // 凭证号；同一凭证借贷平衡
    businessId: T('业务单号（支付/退款/结算/提现/调整）'),  // 业务单号（支付/退款/结算/提现/调整）
    businessType: C('业务类型', ['payment', 'refund', 'settlement', 'withdrawal', 'subsidy',  // 业务类型
      'penalty', 'adjust', 'coupon', 'channel_fee']),
    account: T('科目（如 渠道现金/司机应付/平台业务收入/补贴费用）'),  // 科目（如 渠道现金/司机应付/平台业务收入/补贴费用）
    debit: M('借方金额（分）'),  // 借方金额（分）
    credit: M('贷方金额（分）'),  // 贷方金额（分）
    currency: T('币种，默认 CNY'),  // 币种，默认 CNY
    originalEntryId: T('冲正关联原分录'),  // 冲正关联原分录
    occurredAt: D('发生时间'),  // 发生时间
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { entryId: 1 }, unique: true, name: 'entryId_1' },
    { key: { voucherId: 1 }, name: 'voucherId_1' },
    { key: { businessId: 1, businessType: 1 }, name: 'businessId_type' }
  ]
});

collections.push({
  // 【提现单：提现申请、风控、审核与出款】
  name: 'withdrawals',
  required: ['withdrawNo', 'driverId', 'amount', 'fee', 'actualAmount', 'bankCardEnc',
    'bankName', 'accountName', 'status', 'freeQuotaUsed', 'riskChecked', 'version',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    withdrawNo: T('提现单号'),  // 提现单号
    driverId: R('司机 ID'),  // 司机 ID
    amount: M('提现金额（分）'),  // 提现金额（分）
    fee: M('手续费（分）'),  // 手续费（分）
    actualAmount: M('实到金额（分）'),  // 实到金额（分）
    bankCardEnc: T('收款卡（加密）'),  // 收款卡（加密）
    bankName: T('开户行'),  // 开户行
    accountName: T('收款人姓名'),  // 收款人姓名
    status: C('状态', ['created', 'auditing', 'approved', 'rejected', 'paying', 'paid', 'failed']),  // 状态
    balanceSnapshot: M('申请时可提现余额快照（分）'),  // 申请时可提现余额快照（分）
    freeQuotaUsed: B('是否已用当日免费额度'),  // 是否已用当日免费额度
    riskChecked: B('是否通过风控'),  // 是否通过风控
    riskLevel: C('风控等级', ['low', 'medium', 'high']),  // 风控等级
    riskRemark: T('风控结论'),  // 风控结论
    auditorId: R('审核人'),  // 审核人
    approverId: R('复核人（大额双审）'),  // 复核人（大额双审）
    auditedAt: D('审核时间'),  // 审核时间
    rejectReason: T('驳回原因'),  // 驳回原因
    channelTradeNo: T('打款渠道流水'),  // 打款渠道流水
    paidAt: D('打款时间'),  // 打款时间
    failReason: T('失败原因'),  // 失败原因
    appealStatus: C('申诉状态', ['none', 'appealing', 'accepted', 'rejected']),  // 申诉状态
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { withdrawNo: 1 }, unique: true, name: 'withdrawNo_1' },
    { key: { driverId: 1, createdAt: -1 }, name: 'driverId_createdAt' },
    { key: { status: 1, createdAt: 1 }, name: 'status_createdAt' }
  ]
});

collections.push({
  // 【结算单：司机/CP 周期结算与对账结果】
  name: 'settlements',
  required: ['settleNo', 'targetType', 'targetId', 'periodStart', 'periodEnd', 'orderCount',
    'grossAmount', 'commission', 'rewardAmount', 'penaltyAmount', 'netAmount', 'invoiceStatus',
    'status', 'version', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    settleNo: T('结算单号'),  // 结算单号
    targetType: C('结算对象', ['driver', 'partner']),  // 结算对象
    targetId: R('司机/CP ID'),  // 司机/CP ID
    businessDate: T('业务日 YYYY-MM-DD'),  // 业务日 YYYY-MM-DD
    periodStart: D('周期开始'),  // 周期开始
    periodEnd: D('周期结束'),  // 周期结束
    orderCount: I('订单数'),  // 订单数
    grossAmount: M('流水总额（分）'),  // 流水总额（分）
    commission: M('平台抽成（分）'),  // 平台抽成（分）
    rewardAmount: M('奖励（分）'),  // 奖励（分）
    penaltyAmount: M('罚款（分）'),  // 罚款（分）
    netAmount: M('实结金额（分）'),  // 实结金额（分）
    invoiceStatus: C('开票状态', ['none', 'issued']),  // 开票状态
    status: C('状态', ['pending', 'processing', 'settled', 'suspended']),  // 状态
    reconciliationStatus: C('对账结果', ['matched', 'discrepancy']),  // 对账结果
    paidAt: D('打款时间'),  // 打款时间
    remark: T('备注'),  // 备注
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { settleNo: 1 }, unique: true, name: 'settleNo_1' },
    { key: { targetId: 1, businessDate: 1 }, unique: true, name: 'targetId_businessDate' }
  ]
});

// 对账差异单（业务汇总 A10：三方对账差异挂起）【缺口新增】
collections.push({
  // 【对账差异单：三方对账差异挂起处理】
  name: 'reconciliation_discrepancies',
  required: ['discrepancyNo', 'bizDate', 'bizType', 'bizId', 'expectedAmount', 'actualAmount',
    'diffAmount', 'status', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    discrepancyNo: T('差异单号'),  // 差异单号
    bizDate: T('业务日'),  // 业务日
    bizType: C('差异类型', ['channel', 'ledger', 'settlement']),  // 差异类型
    bizId: T('关联业务单（支付/收益记录）'),  // 关联业务单（支付/收益记录）
    expectedAmount: M('应有金额（分）'),  // 应有金额（分）
    actualAmount: M('实有金额（分）'),  // 实有金额（分）
    diffAmount: M('差异金额（分）'),  // 差异金额（分）
    status: C('状态', ['open', 'processing', 'resolved', 'suspended']),  // 状态
    handlerId: R('处理人'),  // 处理人
    remark: T('说明'),  // 说明
    ...AUDIT
  },
  indexes: [
    { key: { discrepancyNo: 1 }, unique: true, name: 'discrepancyNo_1' },
    { key: { bizDate: 1, status: 1 }, name: 'bizDate_status' }
  ]
});

collections.push({
  // 【发票：乘客开票申请与红冲】
  name: 'invoices',
  required: ['invoiceNo', 'userId', 'orderIds', 'amount', 'invoiceType', 'titleType',
    'title', 'email', 'status', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    invoiceNo: T('发票申请单号'),  // 发票申请单号
    userId: R('申请人'),  // 申请人
    orderIds: A('关联订单', R('订单 ID')),  // 关联订单
    amount: M('开票金额（分）'),  // 开票金额（分）
    invoiceType: C('发票类型', ['normal_elec', 'special_vat']),  // 发票类型
    titleType: C('抬头类型', ['personal', 'company']),  // 抬头类型
    title: T('发票抬头'),  // 发票抬头
    taxNo: T('税号'),  // 税号
    companyAddress: T('企业地址'),  // 企业地址
    companyPhone: T('企业电话'),  // 企业电话
    bankName: T('开户行'),  // 开户行
    bankAccount: T('银行账号'),  // 银行账号
    email: T('接收邮箱'),  // 接收邮箱
    status: C('状态', ['applying', 'issued', 'failed', 'red']),  // 状态
    invoiceFileUrl: T('发票文件地址'),  // 发票文件地址
    issuedAt: D('开票时间'),  // 开票时间
    failReason: T('失败原因'),  // 失败原因
    ...AUDIT
  },
  indexes: [
    { key: { invoiceNo: 1 }, unique: true, name: 'invoiceNo_1' },
    { key: { userId: 1, createdAt: -1 }, name: 'userId_createdAt' }
  ]
});

collections.push({
  // 【常用发票抬头：个人/企业发票抬头簿】
  name: 'invoice_titles',
  required: ['userId', 'titleType', 'title', 'isDefault', 'status', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    userId: R('所属乘客'),  // 所属乘客
    titleType: C('抬头类型', ['personal', 'company']),  // 抬头类型
    title: T('发票抬头'),  // 发票抬头
    taxNo: T('税号'),  // 税号
    companyAddress: T('企业地址'),  // 企业地址
    companyPhone: T('企业电话'),  // 企业电话
    bankName: T('开户银行'),  // 开户银行
    bankAccountEnc: T('银行账号（加密）'),  // 银行账号（加密）
    email: T('接收邮箱'),  // 接收邮箱
    isDefault: B('是否默认'),  // 是否默认
    status: C('状态', ['enabled', 'disabled']),  // 状态
    ...AUDIT
  },
  indexes: [
    { key: { userId: 1, isDefault: 1 }, name: 'userId_isDefault' },
    { key: { taxNo: 1 }, name: 'taxNo_1' }
  ]
});

// ============================ 9. 营销域 ============================
collections.push({
  // 【优惠券模板：满减/折扣券模板与成本承担】
  name: 'coupon_templates',
  required: ['templateNo', 'name', 'couponType', 'faceValue', 'minCharge', 'validType',
    'stackable', 'perLimit', 'settleMode', 'status', 'version', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    templateNo: T('模板编号'),  // 模板编号
    name: T('券名称'),  // 券名称
    couponType: C('券类型', ['discount', 'cash', 'new_user', 'ride_times']),  // 券类型
    faceValue: M('面额（分）'),  // 面额（分）
    minCharge: M('最低消费门槛（分）'),  // 最低消费门槛（分）
    maxDiscount: M('折扣券最高优惠（分）'),  // 折扣券最高优惠（分）
    cityCodes: A('适用城市', T('城市码')),  // 适用城市
    productCodes: A('适用产品线', T('产品线 code')),  // 适用产品线
    timeRanges: A('可用时段', O('时段', { start: T('始'), end: T('止') })),  // 可用时段
    validType: C('有效期类型', ['fixed', 'relative']),  // 有效期类型
    validStart: D('固定开始'),  // 固定开始
    validEnd: D('固定结束'),  // 固定结束
    validDays: I('领取后有效天数'),  // 领取后有效天数
    stackable: B('是否可叠加'),  // 是否可叠加
    mutualCodes: A('互斥券模板', T('模板 code')),  // 互斥券模板
    perLimit: I('每人限领'),  // 每人限领
    settleMode: C('成本承担', ['platform', 'driver', 'share']),  // 成本承担
    shareRatio: N('分摊比例'),  // 分摊比例
    status: C('状态', ['draft', 'enabled', 'disabled', 'expired']),  // 状态
    description: T('使用说明'),  // 使用说明
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { templateNo: 1 }, unique: true, name: 'templateNo_1' },
    { key: { status: 1 }, name: 'status_1' }
  ]
});

collections.push({
  // 【券批次：发放批次、库存与预算】
  name: 'coupon_batches',
  required: ['batchNo', 'templateId', 'totalCount', 'issuedCount', 'usedCount', 'channel',
    'status', 'startAt', 'endAt', 'version', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    batchNo: T('批次号'),  // 批次号
    templateId: R('模板 ID'),  // 模板 ID
    totalCount: I('发行总量'),  // 发行总量
    issuedCount: I('已发放数'),  // 已发放数
    usedCount: I('已核销数'),  // 已核销数
    channel: C('发放渠道', ['new_user', 'activity', 'manual', 'invite', 'compensate']),  // 发放渠道
    targetUserIds: A('定向用户', R('用户 ID')),  // 定向用户
    budget: M('预算金额（分）'),  // 预算金额（分）
    status: C('状态', ['pending', 'running', 'finished', 'stopped']),  // 状态
    startAt: D('发放开始'),  // 发放开始
    endAt: D('发放结束'),  // 发放结束
    operatorId: R('创建运营'),  // 创建运营
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { batchNo: 1 }, unique: true, name: 'batchNo_1' },
    { key: { templateId: 1 }, name: 'templateId_1' }
  ]
});

collections.push({
  // 【用户优惠券：个人券 AVAILABLE/LOCKED/USED/EXPIRED/VOID】
  name: 'user_coupons',
  required: ['couponNo', 'userId', 'templateId', 'status', 'obtainAt', 'validStart', 'validEnd',
    'source', 'version', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    couponNo: T('券码'),  // 券码
    userId: R('所属用户'),  // 所属用户
    templateId: R('模板 ID'),  // 模板 ID
    batchId: R('批次 ID'),  // 批次 ID
    status: C('券状态', ['AVAILABLE', 'LOCKED', 'USED', 'EXPIRED', 'VOID']),  // 券状态
    obtainAt: D('领取时间'),  // 领取时间
    validStart: D('有效期开始'),  // 有效期开始
    validEnd: D('有效期结束'),  // 有效期结束
    usedAt: D('使用时间'),  // 使用时间
    orderId: R('使用订单'),  // 使用订单
    lockOrderId: R('锁定订单'),  // 锁定订单
    discountAmount: M('实际抵扣（分）'),  // 实际抵扣（分）
    source: C('来源', ['new_user', 'activity', 'invite', 'manual', 'exchange', 'compensate']),  // 来源
    ...AUDIT, ...LOCK
  },
  indexes: [
    { key: { couponNo: 1 }, unique: true, name: 'couponNo_1' },
    { key: { userId: 1, status: 1, validEnd: 1 }, name: 'userId_status_validEnd' },
    { key: { lockOrderId: 1 }, name: 'lockOrderId_1' },
    { key: { validEnd: 1, status: 1 }, name: 'validEnd_status' }
  ]
});

collections.push({
  // 【营销活动：拉新/冲单/补贴等活动与预算】
  name: 'campaigns',
  required: ['code', 'name', 'type', 'targetAudience', 'rules', 'startAt', 'endAt',
    'status', 'participantCount', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    code: T('活动编码'),  // 活动编码
    name: T('活动名称'),  // 活动名称
    type: C('活动类型', ['new_user_gift', 'invite', 'recharge', 'sign_in', 'task_center',  // 活动类型
      'driver_bonus', 'flash_sale']),
    targetAudience: C('目标人群', ['passenger', 'driver', 'all']),  // 目标人群
    rules: O('活动规则', {}),  // 活动规则
    rewardCouponTemplateIds: A('奖励券模板', R('模板 ID')),  // 奖励券模板
    rewardAmount: M('现金奖励（分）'),  // 现金奖励（分）
    cityCodes: A('适用城市', T('城市码')),  // 适用城市
    startAt: D('开始'),  // 开始
    endAt: D('结束'),  // 结束
    status: C('状态', ['draft', 'running', 'paused', 'ended']),  // 状态
    participantCount: I('参与人数'),  // 参与人数
    budget: M('预算（分）'),  // 预算（分）
    usedBudget: M('已用（分）'),  // 已用（分）
    taskType: T('关联任务类型'),  // 关联任务类型
    needEnroll: B('是否需要报名'),  // 是否需要报名
    pageUrl: T('活动页地址'),  // 活动页地址
    ...AUDIT
  },
  indexes: [
    { key: { code: 1 }, unique: true, name: 'code_1' },
    { key: { status: 1, startAt: 1 }, name: 'status_startAt' }
  ]
});

collections.push({
  // 【邀请关系：邀请人与被邀请人及奖励状态】
  name: 'invitations',
  required: ['inviterId', 'inviteeId', 'inviteCode', 'channel', 'registeredAt', 'rewardStatus',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    inviterId: R('邀请人'),  // 邀请人
    inviteeId: R('被邀请人'),  // 被邀请人
    inviteCode: T('邀请码'),  // 邀请码
    channel: C('分享渠道', ['wechat', 'sms', 'poster', 'link']),  // 分享渠道
    registeredAt: D('被邀请人注册时间'),  // 被邀请人注册时间
    firstOrderAt: D('首单时间'),  // 首单时间
    rewardStatus: C('奖励状态', ['pending', 'granted', 'invalid']),  // 奖励状态
    inviterReward: O('邀请人奖励', { couponId: R('券'), amount: M('金额') }),  // 邀请人奖励
    inviteeReward: O('被邀请人奖励', { couponId: R('券'), amount: M('金额') }),  // 被邀请人奖励
    ...AUDIT
  },
  indexes: [
    { key: { inviteeId: 1 }, unique: true, name: 'inviteeId_1' },
    { key: { inviterId: 1 }, name: 'inviterId_1' }
  ]
});

// ============================ 10. 评价客服域 ============================
collections.push({
  // 【评价：乘客/司机互评，星级标签匿名与申诉】
  name: 'reviews',
  required: ['orderId', 'reviewerType', 'reviewerId', 'targetId', 'rating', 'isAnonymous',
    'isVisible', 'appealStatus', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    orderId: R('订单 ID'),  // 订单 ID
    reviewerType: C('评价方', ['passenger', 'driver']),  // 评价方
    reviewerId: R('评价人'),  // 评价人
    targetId: R('被评价人'),  // 被评价人
    rating: I('星级 1~5', 1),  // 星级 1~5
    tags: A('标签', T('标签')),  // 标签
    content: T('文字评价（≤200 字）'),  // 文字评价（≤200 字）
    isAnonymous: B('是否匿名'),  // 是否匿名
    isVisible: B('是否可见'),  // 是否可见
    auditStatus: C('敏感词审核', ['pending', 'approved', 'rejected']),  // 敏感词审核
    appealStatus: C('申诉状态', ['none', 'appealing', 'accepted', 'rejected']),  // 申诉状态
    appealReason: T('申诉理由'),  // 申诉理由
    replyContent: T('回复'),  // 回复
    scoreDelta: N('服务分影响'),  // 服务分影响
    ...AUDIT
  },
  indexes: [
    { key: { orderId: 1, reviewerType: 1 }, unique: true, name: 'orderId_reviewerType' },
    { key: { targetId: 1, rating: 1, createdAt: -1 }, name: 'targetId_rating' }
  ]
});

collections.push({
  // 【客服工单：投诉/费用异议/安全工单，SLA 升级】
  name: 'tickets',
  required: ['ticketNo', 'applicantType', 'category', 'title', 'content', 'priority',
    'status', 'isEscalated', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    ticketNo: T('工单号'),  // 工单号
    orderId: R('关联订单'),  // 关联订单
    userId: R('乘客'),  // 乘客
    driverId: R('司机'),  // 司机
    applicantType: C('发起人类型', ['passenger', 'driver', 'system', 'admin']),  // 发起人类型
    category: C('分类', ['complaint_driver', 'complaint_passenger', 'fee_dispute', 'lost_item',  // 分类
      'accident', 'account', 'invoice', 'coupon', 'safety', 'other']),
    complaintType: C('投诉类型', ['detour', 'attitude', 'dangerous_driving', 'fare_dispute', 'other']),  // 投诉类型
    subCategory: T('二级分类'),  // 二级分类
    title: T('工单标题'),  // 工单标题
    content: T('问题描述（≤500 字）'),  // 问题描述（≤500 字）
    attachments: A('附件', T('图片地址'), 6),  // 附件
    location: P('事发定位'),  // 事发定位
    priority: C('优先级', ['low', 'normal', 'high', 'urgent']),  // 优先级
    status: C('状态', ['pending', 'processing', 'waiting_user', 'resolved', 'closed', 'reopened']),  // 状态
    assigneeId: R('处理人'),  // 处理人
    skillGroup: T('技能组'),  // 技能组
    slaDeadline: D('SLA 截止'),  // SLA 截止
    firstResponseAt: D('首次响应'),  // 首次响应
    resolvedAt: D('解决时间'),  // 解决时间
    resolution: T('处理结论'),  // 处理结论
    compensation: O('赔付', { amount: M('金额'), type: C('类型', ['coupon', 'cash']), status: T('状态') }),  // 赔付
    satisfaction: I('满意度 1~5', 1),  // 满意度 1~5
    isEscalated: B('是否升级'),  // 是否升级
    escalateTo: R('升级给谁'),  // 升级给谁
    appealRound: I('申诉轮次'),  // 申诉轮次
    tags: A('标签', T('标签')),  // 标签
    ...AUDIT
  },
  indexes: [
    { key: { ticketNo: 1 }, unique: true, name: 'ticketNo_1' },
    { key: { status: 1, priority: -1, createdAt: 1 }, name: 'status_priority' },
    { key: { assigneeId: 1, status: 1 }, name: 'assigneeId_status' },
    { key: { orderId: 1 }, name: 'orderId_1' },
    { key: { slaDeadline: 1, status: 1 }, name: 'slaDeadline_status' }
  ]
});

collections.push({
  // 【工单处理日志：工单流转操作记录】
  name: 'ticket_logs',
  required: ['ticketId', 'action', 'operatorId', 'operatorType', 'isInternal',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    ticketId: R('工单 ID'),  // 工单 ID
    action: C('动作', ['create', 'assign', 'reply', 'escalate', 'resolve', 'close', 'reopen', 'transfer']),  // 动作
    operatorId: R('操作人'),  // 操作人
    operatorType: C('操作人类型', ['cs', 'user', 'system', 'admin']),  // 操作人类型
    content: T('操作内容'),  // 操作内容
    fromStatus: T('变更前'),  // 变更前
    toStatus: T('变更后'),  // 变更后
    attachments: A('附件', T('地址')),  // 附件
    isInternal: B('是否内部备注'),  // 是否内部备注
    ...AUDIT
  },
  indexes: [{ key: { ticketId: 1, createdAt: 1 }, name: 'ticketId_createdAt' }]
});

collections.push({
  // 【遗失物品：报失、匹配、认领与归还】
  name: 'lost_items',
  required: ['orderId', 'userId', 'itemName', 'itemCategory', 'contactPhoneEnc', 'status',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    orderId: R('订单 ID'),  // 订单 ID
    userId: R('报失乘客'),  // 报失乘客
    driverId: R('司机'),  // 司机
    itemName: T('物品名称'),  // 物品名称
    itemCategory: C('分类', ['phone', 'wallet', 'bag', 'document', 'key', 'electronic', 'other']),  // 分类
    description: T('特征描述'),  // 特征描述
    images: A('图片', T('地址')),  // 图片
    contactPhoneEnc: T('联系方式（加密）'),  // 联系方式（加密）
    status: C('状态', ['reported', 'searching', 'found', 'returned', 'closed']),  // 状态
    foundAt: D('找到时间'),  // 找到时间
    returnedAt: D('归还时间'),  // 归还时间
    handlerId: R('处理客服'),  // 处理客服
    remark: T('备注'),  // 备注
    ...AUDIT
  },
  indexes: [
    { key: { orderId: 1 }, name: 'orderId_1' },
    { key: { status: 1 }, name: 'status_1' }
  ]
});

// ============================ 11. 风控安全域 ============================
collections.push({
  // 【风控事件：刷单/虚假定位/薅券等风险事件处置】
  name: 'risk_events',
  required: ['eventNo', 'bizScene', 'riskType', 'hitRules', 'riskScore', 'decision', 'status',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    eventNo: T('事件编号'),  // 事件编号
    bizScene: C('场景', ['order_create', 'order_pay', 'coupon_use', 'withdraw', 'login',  // 场景
      'driver_accept', 'location_report']),
    bizId: R('关联业务 ID'),  // 关联业务 ID
    userId: R('用户'),  // 用户
    driverId: R('司机'),  // 司机
    deviceId: T('设备'),  // 设备
    riskType: C('风险类型', ['fake_order', 'fake_location', 'device_farm', 'coupon_abuse',  // 风险类型
      'account_relation', 'abnormal_route', 'payment_risk', 'withdraw_risk']),
    hitRules: A('命中规则', O('规则', { ruleId: R('规则'), ruleName: T('名称'), weight: I('权重') })),  // 命中规则
    riskScore: N('风险评分 0~100'),  // 风险评分 0~100
    decision: C('决策', ['pass', 'review', 'reject']),  // 决策
    actionTaken: C('处置', ['none', 'warn', 'limit_order', 'freeze_wallet', 'ban']),  // 处置
    evidence: O('证据快照', {}),  // 证据快照
    reviewerId: R('复核人'),  // 复核人
    reviewResult: T('复核结论'),  // 复核结论
    reviewedAt: D('复核时间'),  // 复核时间
    status: C('状态', ['open', 'reviewing', 'closed']),  // 状态
    ...AUDIT
  },
  indexes: [
    { key: { eventNo: 1 }, unique: true, name: 'eventNo_1' },
    { key: { bizScene: 1, decision: 1, createdAt: -1 }, name: 'bizScene_decision' },
    { key: { riskScore: -1 }, name: 'riskScore_-1' }
  ]
});

collections.push({
  // 【风控规则：硬规则+评分卡，灰度与回滚】
  name: 'risk_rules',
  required: ['ruleCode', 'ruleName', 'bizScene', 'ruleType', 'condition', 'weight',
    'action', 'enabled', 'priority', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    ruleCode: T('规则编码'),  // 规则编码
    ruleName: T('规则名称'),  // 规则名称
    bizScene: C('场景', ['order_create', 'order_pay', 'coupon_use', 'withdraw', 'login', 'driver_accept', 'location_report']),  // 场景
    ruleType: C('规则类型', ['hard', 'score']),  // 规则类型
    condition: O('条件表达式', {}),  // 条件表达式
    weight: I('权重分'),  // 权重分
    threshold: N('触发阈值'),  // 触发阈值
    action: C('动作', ['pass', 'review', 'reject']),  // 动作
    enabled: B('是否启用'),  // 是否启用
    priority: I('优先级'),  // 优先级
    cityCodes: A('适用城市', T('城市码')),  // 适用城市
    grayRatio: N('灰度比例 0~1'),  // 灰度比例 0~1
    grayCityCodes: A('灰度城市', T('城市码')),  // 灰度城市
    remark: T('说明'),  // 说明
    ...AUDIT
  },
  indexes: [
    { key: { ruleCode: 1 }, unique: true, name: 'ruleCode_1' },
    { key: { bizScene: 1, enabled: 1 }, name: 'bizScene_enabled' }
  ]
});

collections.push({
  // 【黑名单：用户/司机/设备/手机号/银行卡/IP 黑名单】
  name: 'blacklists',
  required: ['targetType', 'targetValue', 'reasonType', 'reason', 'source', 'scope',
    'startAt', 'status', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    targetType: C('目标类型', ['user', 'driver', 'device', 'phone', 'bank_card', 'ip']),  // 目标类型
    targetValue: T('目标值'),  // 目标值
    reasonType: C('原因类型', ['fraud', 'abuse', 'violence', 'arrears', 'illegal', 'manual']),  // 原因类型
    reason: T('具体原因'),  // 具体原因
    source: C('来源', ['system', 'risk_engine', 'cs', 'admin']),  // 来源
    relatedRiskEventId: R('关联风控事件'),  // 关联风控事件
    scope: C('范围', ['global', 'city']),  // 范围
    cityCodes: A('生效城市', T('城市码')),  // 生效城市
    startAt: D('生效开始'),  // 生效开始
    endAt: D('生效结束（空=永久）'),  // 生效结束（空=永久）
    status: C('状态', ['active', 'expired', 'revoked']),  // 状态
    operatorId: R('操作人'),  // 操作人
    ...AUDIT
  },
  indexes: [
    { key: { targetType: 1, targetValue: 1, status: 1 }, name: 'target_status' },
    { key: { endAt: 1, status: 1 }, name: 'endAt_status' }
  ]
});

collections.push({
  // 【安全事件：SOS/偏航/长停/事故等安全闭环】
  name: 'safety_incidents',
  required: ['incidentNo', 'orderId', 'userId', 'driverId', 'type', 'level', 'triggerSource',
    'policeNotified', 'status', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    incidentNo: T('事件编号'),  // 事件编号
    orderId: R('关联订单'),  // 关联订单
    userId: R('乘客'),  // 乘客
    driverId: R('司机'),  // 司机
    type: C('类型', ['sos', 'deviation', 'long_stop', 'route_change', 'accident', 'conflict', 'night_alert']),  // 类型
    level: C('级别', ['p0', 'p1', 'p2']),  // 级别
    triggerSource: C('触发源', ['user', 'driver', 'system_detect', 'emergency_contact']),  // 触发源
    location: P('事发位置'),  // 事发位置
    description: T('描述'),  // 描述
    evidence: O('证据', { recordUrl: T('录音'), trajectorySnapshot: T('轨迹快照'), images: A('图片', T('地址')) }),  // 证据
    policeNotified: B('是否通知警方'),  // 是否通知警方
    policeNotifiedAt: D('通知时间'),  // 通知时间
    contactsNotified: A('已通知紧急联系人', R('联系人')),  // 已通知紧急联系人
    status: C('状态', ['open', 'handling', 'resolved', 'closed', 'false_alarm']),  // 状态
    handlerId: R('处置人'),  // 处置人
    handlingLog: A('处置记录', O('记录', { at: D('时间'), by: R('人'), content: T('内容') })),  // 处置记录
    resolvedAt: D('解决时间'),  // 解决时间
    conclusion: T('结论'),  // 结论
    ...AUDIT
  },
  indexes: [
    { key: { incidentNo: 1 }, unique: true, name: 'incidentNo_1' },
    { key: { orderId: 1 }, name: 'orderId_1' },
    { key: { status: 1, level: 1, createdAt: -1 }, name: 'status_level' },
    { key: { location: '2dsphere' }, name: 'location_2dsphere' }
  ]
});

// ============================ 12. 通知域 ============================
collections.push({
  // 【消息模板：短信/推送/站内信模板与频控】
  name: 'message_templates',
  required: ['code', 'name', 'channel', 'scene', 'content', 'enabled', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    code: T('模板编码'),  // 模板编码
    name: T('模板名称'),  // 模板名称
    channel: C('通道', ['sms', 'push', 'in_app', 'wechat', 'voice']),  // 通道
    scene: T('业务场景'),  // 业务场景
    title: T('标题'),  // 标题
    content: T('内容模板 ${var}'),  // 内容模板 ${var}
    variables: A('变量列表', T('变量')),  // 变量列表
    thirdPartyTemplateId: T('三方模板 ID'),  // 三方模板 ID
    jumpUrl: T('点击跳转'),  // 点击跳转
    enabled: B('是否启用'),  // 是否启用
    frequencyLimit: I('频控（分钟）'),  // 频控（分钟）
    quietHours: O('免打扰时段', { start: T('始'), end: T('止') }),  // 免打扰时段
    ...AUDIT
  },
  indexes: [
    { key: { code: 1 }, unique: true, name: 'code_1' },
    { key: { channel: 1, enabled: 1 }, name: 'channel_enabled' }
  ]
});

collections.push({
  // 【消息发送记录：消息触达、已读与重试】
  name: 'notifications',
  required: ['templateCode', 'channel', 'receiverType', 'receiverId', 'content', 'status',
    'retryCount', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    templateCode: T('模板编码'),  // 模板编码
    channel: C('通道', ['sms', 'push', 'in_app', 'wechat', 'voice']),  // 通道
    receiverType: C('接收人类型', ['user', 'driver', 'admin']),  // 接收人类型
    receiverId: R('接收人'),  // 接收人
    receiverAddress: T('手机号/openId/deviceToken'),  // 手机号/openId/deviceToken
    bizId: R('关联业务 ID'),  // 关联业务 ID
    bizType: T('业务类型'),  // 业务类型
    title: T('标题'),  // 标题
    content: T('渲染后内容'),  // 渲染后内容
    status: C('状态', ['pending', 'sent', 'delivered', 'failed', 'blocked']),  // 状态
    isRead: B('站内信是否已读'),  // 站内信是否已读
    readAt: D('阅读时间'),  // 阅读时间
    sentAt: D('发送时间'),  // 发送时间
    deliveredAt: D('到达时间'),  // 到达时间
    failReason: T('失败原因'),  // 失败原因
    thirdPartyMsgId: T('三方消息 ID'),  // 三方消息 ID
    retryCount: I('重试次数'),  // 重试次数
    ...AUDIT
  },
  indexes: [
    { key: { receiverId: 1, createdAt: -1 }, name: 'receiverId_createdAt' },
    { key: { status: 1, channel: 1 }, name: 'status_channel' },
    { key: { bizId: 1 }, name: 'bizId_1' }
  ]
});

collections.push({
  // 【平台公告：弹窗/横幅/消息中心公告】
  name: 'announcements',
  required: ['announceNo', 'title', 'content', 'targetAudience', 'channel', 'priority',
    'isPinned', 'requireConfirm', 'publishAt', 'status', 'targetCount', 'readCount',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    announceNo: T('公告编号'),  // 公告编号
    title: T('标题'),  // 标题
    content: T('正文'),  // 正文
    targetAudience: C('目标人群', ['all', 'passenger', 'driver', 'city', 'partner', 'fleet']),  // 目标人群
    cityCodes: A('生效城市', T('城市码')),  // 生效城市
    partnerId: R('指定加盟商'),  // 指定加盟商
    channel: C('通道', ['popup', 'banner', 'message_center', 'sms']),  // 通道
    priority: I('优先级'),  // 优先级
    isPinned: B('是否置顶'),  // 是否置顶
    requireConfirm: B('是否必须确认已读'),  // 是否必须确认已读
    publishAt: D('发布时间'),  // 发布时间
    offlineAt: D('下线时间'),  // 下线时间
    status: C('状态', ['draft', 'published', 'offline']),  // 状态
    targetCount: I('应读人数'),  // 应读人数
    readCount: I('已读人数'),  // 已读人数
    publisherId: R('发布运营'),  // 发布运营
    ...AUDIT
  },
  indexes: [
    { key: { announceNo: 1 }, unique: true, name: 'announceNo_1' },
    { key: { targetAudience: 1, status: 1, publishAt: -1 }, name: 'audience_status' },
    { key: { isPinned: -1, priority: -1 }, name: 'pinned_priority' }
  ]
});

collections.push({
  // 【公告已读：公告已读与强制确认记录】
  name: 'announcement_reads',
  required: ['announceId', 'receiverType', 'receiverId', 'readAt', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    announceId: R('公告 ID'),  // 公告 ID
    receiverType: C('接收人类型', ['user', 'driver']),  // 接收人类型
    receiverId: R('接收人'),  // 接收人
    readAt: D('阅读时间'),  // 阅读时间
    confirmedAt: D('确认时间'),  // 确认时间
    ...AUDIT
  },
  indexes: [
    { key: { announceId: 1, receiverId: 1 }, unique: true, name: 'announceId_receiverId' },
    { key: { receiverId: 1, readAt: -1 }, name: 'receiverId_readAt' }
  ]
});

// ============================ 13. 司机奖惩排班域 ============================
collections.push({
  // 【司机奖惩记录：冲单奖/时段补贴/罚款与申诉】
  name: 'driver_reward_penalties',
  required: ['rpNo', 'driverId', 'direction', 'bizType', 'amount', 'reason', 'status',
    'operatorType', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    rpNo: T('奖惩单号'),  // 奖惩单号
    driverId: R('司机 ID'),  // 司机 ID
    direction: C('方向', ['reward', 'penalty']),  // 方向
    bizType: C('业务类型', ['peak_bonus', 'complete_task', 'time_subsidy', 'referral',  // 业务类型
      'late_arrive', 'driver_cancel', 'complaint', 'fake_order', 'other']),
    campaignId: R('关联活动'),  // 关联活动
    orderId: R('关联订单'),  // 关联订单
    amount: M('金额（分，正数）'),  // 金额（分，正数）
    scoreDelta: N('服务分变动'),  // 服务分变动
    reason: T('事由'),  // 事由
    evidence: A('证据附件', T('地址')),  // 证据附件
    status: C('状态', ['created', 'effective', 'appealing', 'reversed', 'rejected']),  // 状态
    effectiveAt: D('生效时间'),  // 生效时间
    expireAt: D('失效时间'),  // 失效时间
    appealReason: T('申诉理由'),  // 申诉理由
    appealResult: T('申诉结论'),  // 申诉结论
    appealAt: D('申诉时间'),  // 申诉时间
    reversedAt: D('撤销时间'),  // 撤销时间
    operatorId: R('操作人'),  // 操作人
    operatorType: C('操作人类型', ['system', 'admin', 'cs']),  // 操作人类型
    ...AUDIT
  },
  indexes: [
    { key: { rpNo: 1 }, unique: true, name: 'rpNo_1' },
    { key: { driverId: 1, createdAt: -1 }, name: 'driverId_createdAt' },
    { key: { status: 1, createdAt: 1 }, name: 'status_createdAt' }
  ]
});

collections.push({
  // 【司机排班与时长任务：在线时长/完单任务报名与领奖】
  name: 'driver_schedule_tasks',
  required: ['taskNo', 'driverId', 'cityCode', 'taskDate', 'taskType', 'periodStart',
    'periodEnd', 'targetValue', 'currentValue', 'rewardType', 'status', 'source',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    taskNo: T('任务单号'),  // 任务单号
    driverId: R('司机 ID'),  // 司机 ID
    cityCode: T('城市码'),  // 城市码
    taskDate: T('任务日期'),  // 任务日期
    taskType: C('任务类型', ['online_minutes', 'order_count', 'peak_shift', 'time_slot', 'airport_shift']),  // 任务类型
    periodStart: D('开始'),  // 开始
    periodEnd: D('结束'),  // 结束
    targetValue: N('目标值'),  // 目标值
    currentValue: N('当前完成值'),  // 当前完成值
    rewardType: C('奖励类型', ['cash', 'coupon', 'score', 'none']),  // 奖励类型
    rewardAmount: M('奖励金额（分）'),  // 奖励金额（分）
    status: C('状态', ['enrolled', 'ongoing', 'completed', 'failed', 'claimed', 'cancelled']),  // 状态
    claimedAt: D('领奖时间'),  // 领奖时间
    source: C('来源', ['campaign', 'auto', 'manual']),  // 来源
    campaignId: R('关联活动'),  // 关联活动
    rewardRpId: R('关联奖惩记录'),  // 关联奖惩记录
    ...AUDIT
  },
  indexes: [
    { key: { taskNo: 1 }, unique: true, name: 'taskNo_1' },
    { key: { driverId: 1, taskDate: -1 }, name: 'driverId_taskDate' },
    { key: { cityCode: 1, taskDate: 1, status: 1 }, name: 'city_taskDate_status' }
  ]
});

// ============================ 14. 拼车域 ============================
collections.push({
  // 【拼车组：两订单同起终点共乘，封组与座位】
  name: 'carpool_groups',
  required: ['groupNo', 'orderIds', 'cityCode', 'productCode', 'seatTotal', 'seatUsed',
    'discountRate', 'status', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    groupNo: T('拼车组号'),  // 拼车组号
    orderIds: A('组内订单（≤2）', R('订单 ID'), 2),  // 组内订单（≤2）
    cityCode: T('城市码'),  // 城市码
    productCode: T('拼车产品线'),  // 拼车产品线
    seatTotal: I('总座位数'),  // 总座位数
    seatUsed: I('已用座位'),  // 已用座位
    routeHash: T('路线哈希'),  // 路线哈希
    origin: P('起区中心点'),  // 起区中心点
    destination: P('终区中心点'),  // 终区中心点
    discountRate: N('拼车折扣率 0~1'),  // 拼车折扣率 0~1
    status: C('状态', ['matching', 'matched', 'started', 'finished', 'cancelled']),  // 状态
    matchedAt: D('封组时间'),  // 封组时间
    expireAt: D('拼单超时（TTL）'),  // 拼单超时（TTL）
    ...AUDIT
  },
  indexes: [
    { key: { groupNo: 1 }, unique: true, name: 'groupNo_1' },
    { key: { cityCode: 1, status: 1, routeHash: 1 }, name: 'city_status_route' },
    { key: { origin: '2dsphere' }, name: 'origin_2dsphere' },
    { key: { expireAt: 1 }, expireAfterSeconds: 0, name: 'expireAt_ttl' }
  ]
});

// ============================ 15. 系统域（管理端支撑） ============================
collections.push({
  // 【管理端账号：后台账号、角色、数据范围与登录】
  name: 'admins',
  required: ['username', 'passwordHash', 'realName', 'phoneEnc', 'roleIds', 'dataScope',
    'status', 'loginFailCount', 'mfaEnabled', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    username: T('登录名'),  // 登录名
    passwordHash: T('密码哈希'),  // 密码哈希
    realName: T('姓名'),  // 姓名
    phoneEnc: T('手机号（加密）'),  // 手机号（加密）
    email: T('邮箱'),  // 邮箱
    roleIds: A('角色', R('角色 ID')),  // 角色
    deptId: R('部门'),  // 部门
    dataScope: C('数据范围', ['all', 'city', 'partner', 'self']),  // 数据范围
    cityCodes: A('可见城市', T('城市码')),  // 可见城市
    partnerId: R('可见加盟商'),  // 可见加盟商
    status: C('状态', ['active', 'disabled']),  // 状态
    lastLoginAt: D('最近登录'),  // 最近登录
    lastLoginIp: T('最近登录 IP'),  // 最近登录 IP
    loginFailCount: I('连续失败次数'),  // 连续失败次数
    lockedUntil: D('锁定至'),  // 锁定至
    mfaEnabled: B('二次验证'),  // 二次验证
    ...AUDIT
  },
  indexes: [
    { key: { username: 1 }, unique: true, name: 'username_1' },
    { key: { status: 1 }, name: 'status_1' }
  ]
});

collections.push({
  // 【角色：RBAC 角色与权限点】
  name: 'roles',
  required: ['code', 'name', 'permissionIds', 'dataScope', 'status', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    code: T('角色编码'),  // 角色编码
    name: T('角色名称'),  // 角色名称
    permissionIds: A('权限点', R('权限 ID')),  // 权限点
    dataScope: C('数据范围', ['all', 'city', 'partner', 'self']),  // 数据范围
    status: C('状态', ['enabled', 'disabled']),  // 状态
    remark: T('备注'),  // 备注
    ...AUDIT
  },
  indexes: [{ key: { code: 1 }, unique: true, name: 'code_1' }]
});

collections.push({
  // 【权限点：菜单/按钮/接口权限】
  name: 'permissions',
  required: ['code', 'name', 'module', 'type', 'sort', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    code: T('权限码，如 order:list'),  // 权限码，如 order:list
    name: T('权限名称'),  // 权限名称
    module: C('模块', ['dashboard', 'order', 'driver', 'user', 'pricing', 'marketing',  // 模块
      'finance', 'cs', 'risk', 'system']),
    type: C('类型', ['menu', 'button', 'api']),  // 类型
    parentId: R('父级'),  // 父级
    sort: I('排序'),  // 排序
    ...AUDIT
  },
  indexes: [{ key: { code: 1 }, unique: true, name: 'code_1' }]
});

collections.push({
  // 【操作日志：后台操作审计，前后快照】
  name: 'audit_logs',
  required: ['adminId', 'adminName', 'module', 'action', 'targetType', 'ip', 'result',
    'operatedAt', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    adminId: R('操作人'),  // 操作人
    adminName: T('操作人姓名快照'),  // 操作人姓名快照
    module: T('模块'),  // 模块
    action: C('动作', ['create', 'update', 'delete', 'audit', 'export', 'approve']),  // 动作
    targetType: T('目标类型'),  // 目标类型
    targetId: R('目标 ID'),  // 目标 ID
    before: O('变更前快照', {}),  // 变更前快照
    after: O('变更后快照', {}),  // 变更后快照
    ip: T('操作 IP'),  // 操作 IP
    userAgent: T('UA'),  // UA
    result: C('结果', ['success', 'fail']),  // 结果
    failReason: T('失败原因'),  // 失败原因
    operatedAt: D('操作时间'),  // 操作时间
    ...AUDIT
  },
  indexes: [
    { key: { adminId: 1, operatedAt: -1 }, name: 'adminId_operatedAt' },
    { key: { targetType: 1, targetId: 1 }, name: 'targetType_targetId' },
    { key: { module: 1, operatedAt: -1 }, name: 'module_operatedAt' }
  ]
});

collections.push({
  // 【系统配置：订单/支付/派单/安全等参数】
  name: 'system_configs',
  required: ['configKey', 'configValue', 'group', 'name', 'valueType', 'editable',
    'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    configKey: T('配置键'),  // 配置键
    configValue: T('配置值（JSON 字符串）'),  // 配置值（JSON 字符串）
    group: C('分组', ['order', 'pay', 'dispatch', 'safety', 'general']),  // 分组
    name: T('配置名称'),  // 配置名称
    description: T('说明'),  // 说明
    valueType: C('值类型', ['string', 'number', 'boolean', 'json']),  // 值类型
    editable: B('是否允许后台修改'),  // 是否允许后台修改
    cityCode: T('城市级配置'),  // 城市级配置
    ...AUDIT
  },
  indexes: [{ key: { configKey: 1 }, unique: true, name: 'configKey_1' }]
});

collections.push({
  // 【数据字典：取消原因/投诉分类/券类型等字典】
  name: 'dicts',
  required: ['type', 'code', 'label', 'sort', 'status', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    type: T('字典类型'),  // 字典类型
    code: T('字典码'),  // 字典码
    label: T('展示文案'),  // 展示文案
    value: T('附加值'),  // 附加值
    sort: I('排序'),  // 排序
    status: C('状态', ['enabled', 'disabled']),  // 状态
    remark: T('备注'),  // 备注
    ...AUDIT
  },
  indexes: [{ key: { type: 1, code: 1 }, unique: true, name: 'type_code' }]
});

collections.push({
  // 【文件资源：上传文件、加密存储与业务关联】
  name: 'files',
  required: ['fileName', 'fileUrl', 'fileSize', 'mimeType', 'bizType', 'uploaderId',
    'isEncrypted', 'storageType', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    fileName: T('文件名'),  // 文件名
    fileUrl: T('访问地址'),  // 访问地址
    fileSize: I('大小（字节）'),  // 大小（字节）
    mimeType: T('类型'),  // 类型
    bizType: C('业务类型', ['avatar', 'qualification', 'vehicle', 'invoice', 'complaint', 'record']),  // 业务类型
    bizId: R('关联业务 ID'),  // 关联业务 ID
    uploaderId: R('上传人'),  // 上传人
    isEncrypted: B('是否加密存储'),  // 是否加密存储
    storageType: C('存储', ['local', 'oss', 'cos']),  // 存储
    ...AUDIT
  },
  indexes: [
    { key: { bizType: 1, bizId: 1 }, name: 'bizType_bizId' },
    { key: { uploaderId: 1 }, name: 'uploaderId_1' }
  ]
});

// 可靠事件表（业务汇总 §5.2：可靠事件投递、消费者去重）【缺口新增】
collections.push({
  // 【可靠事件表：跨服务可靠事件投递与消费者去重】
  name: 'outbox_events',
  required: ['eventId', 'aggregateId', 'aggregateVersion', 'eventType', 'occurredAt',
    'schemaVersion', 'status', 'createdAt', 'updatedAt', 'isDeleted'],
  fields: {
    eventId: T('事件 ID'),  // 事件 ID
    aggregateId: T('聚合根 ID'),  // 聚合根 ID
    aggregateVersion: I('聚合版本'),  // 聚合版本
    eventType: C('事件类型', ['OrderCreated', 'TaskAssigned', 'DriverArrived', 'TripStarted',  // 事件类型
      'TripEnded', 'OrderCancelled', 'BillConfirmed', 'PaymentSucceeded', 'RefundSucceeded',
      'QualificationChanged', 'IncomePosted', 'SettlementPosted', 'WithdrawalFinalized']),
    payload: O('事件负载', {}),  // 事件负载
    occurredAt: D('发生时间'),  // 发生时间
    schemaVersion: I('schema 版本'),  // schema 版本
    status: C('状态', ['pending', 'published', 'consumed', 'failed']),  // 状态
    retryCount: I('重试次数'),  // 重试次数
    consumerKey: T('消费者去重键'),  // 消费者去重键
    ...AUDIT
  },
  indexes: [
    { key: { eventId: 1 }, unique: true, name: 'eventId_1' },
    { key: { aggregateId: 1, aggregateVersion: 1 }, unique: true, name: 'aggregateId_version' },
    { key: { status: 1, occurredAt: 1 }, name: 'status_occurredAt' }
  ]
});

// ============================================================================
// 执行：只增不改（已存在集合跳过；并发集合确保 version 字段在 required 中已有）
// ============================================================================
// ---------------- 集合中文备注（表名 → 中文名 + 说明） ----------------
const COLLECTION_META = {
  // 账户域
  users: { title: '乘客账户', desc: '乘客主账号，含实名、会员等级、信用分、钱包与邀请关系' },
  user_addresses: { title: '乘客常用地址', desc: '家/公司/自定义常用地址，支持默认与使用次数排序' },
  user_emergency_contacts: { title: '紧急联系人', desc: '一键报警优先通知的联系人' },
  user_devices: { title: '设备指纹', desc: '登录设备与风控因子（越狱/模拟器/风险等级）' },
  sms_codes: { title: '短信验证码', desc: '登录/注册等场景验证码，TTL 自动过期' },
  // 司机车辆域
  drivers: { title: '司机账户', desc: '司机主账号，含资质生命周期、在线状态、服务分、收入统计' },
  driver_qualifications: { title: '司机资质证件', desc: '身份证/驾驶证/车证/保险等证件与到期预警' },
  driver_audits: { title: '司机审核记录', desc: '机审+人审全流水，驳回重提递增轮次' },
  driver_trainings: { title: '司机培训与考试', desc: '准入/安全/服务培训与考试成绩' },
  vehicles: { title: '车辆档案', desc: '车牌、车型、车架号、保险/年检/运输证到期' },
  driver_vehicle_binds: { title: '司机车辆绑定', desc: '司机与车辆的一对一有效绑定关系' },
  driver_shifts: { title: '出车收车记录', desc: '每日出车时长、完单数与在线收入' },
  driver_service_areas: { title: '司机接单片区', desc: '司机自定义接单多边形区域' },
  partners: { title: '加盟商(CP)', desc: '加盟商企业信息、分润比例与结算周期' },
  fleets: { title: '车队', desc: '车队管理，司机与车辆的归属组织' },
  // 地理域
  cities: { title: '城市', desc: '开城状态、服务时段、中心点与产品线' },
  pois: { title: 'POI与推荐上车点', desc: '地图兴趣点与推荐上车点' },
  geo_fences: { title: '电子围栏', desc: '禁停/限行/服务区等围栏区域' },
  search_history: { title: '搜索历史', desc: '乘客 POI 搜索历史，按次数倒序' },
  trajectories: { title: '行程轨迹点', desc: '司机位置上报轨迹，高频写入按月归档' },
  // 订单调度域
  products: { title: '产品线(车型)', desc: '快车/优享/专车/拼车等车型配置' },
  orders: { title: '订单', desc: '核心订单，含行程状态、账单状态、计价、司机车辆快照' },
  transport_tasks: { title: '运输任务', desc: '普通一单/拼车最多两单，司机占用依据' },
  order_status_logs: { title: '订单状态流转日志', desc: '订单状态变更留痕（时间线）' },
  order_cancels: { title: '取消记录', desc: '取消责任方、取消费与信用影响' },
  dispatches: { title: '派单记录', desc: '每轮派单候选、推送与接单结果' },
  dispatch_attempts: { title: '派单邀请/预占', desc: '15s 邀请、抢单锁、过期回收' },
  dispatch_queues: { title: '高峰排队队列', desc: '高峰/恶劣天气排队叫号' },
  dispatch_rules: { title: '派单规则配置', desc: '召回半径、打分权重、轮次与超时' },
  destination_changes: { title: '中途改点申请', desc: '每单最多 2 次，司机 30 秒确认' },
  cancel_quotes: { title: '取消报价', desc: '30 秒有效取消报价，绑定订单版本' },
  trip_shares: { title: '行程分享', desc: '只读分享令牌，最长 24 小时' },
  virtual_numbers: { title: '虚拟号绑定', desc: '中间虚拟号，订单存续 +7 天有效' },
  // 计价账单域
  pricing_rules: { title: '计价规则', desc: '起步/里程/时长/佣金/封顶，版本化发布' },
  pricing_snapshots: { title: '计价规则快照', desc: '下单时固化计价规则，保证历史可追溯' },
  surge_rules: { title: '动态调价规则', desc: '高峰/区域溢价与监管上限' },
  cancel_rules: { title: '取消费规则', desc: '免费取消时限与取消费配置' },
  estimates: { title: '估价单', desc: '预估价，5 分钟有效，下单幂等键' },
  bills: { title: '账单', desc: '正式账单，含分账字段 F/Q/H/K/Cp/Cd/P/D/N' },
  bill_items: { title: '账单明细项', desc: '起步/里程/时长/附加费等逐项明细' },
  // 支付资金域
  payments: { title: '支付单', desc: '支付尝试、渠道交易号、幂等与关单' },
  refunds: { title: '退款单', desc: '全额/部分/差额退款，责任分摊模式' },
  driver_wallets: { title: '司机钱包', desc: '资金五桶 U/A/Fz/W/Debt 余额' },
  fund_bucket_movements: { title: '资金桶变动流水', desc: 'U/A/Fz/W/Debt 互斥桶变动记录' },
  wallet_transactions: { title: '钱包流水', desc: '收入/奖励/罚款/提现/退款等流水' },
  accounting_entries: { title: '账务分录', desc: '复式记账凭证，借贷平衡，只增不改' },
  withdrawals: { title: '提现单', desc: '提现申请、风控、审核与出款' },
  settlements: { title: '结算单', desc: '司机/CP 周期结算与对账结果' },
  reconciliation_discrepancies: { title: '对账差异单', desc: '三方对账差异挂起处理' },
  invoices: { title: '发票', desc: '乘客开票申请与红冲' },
  invoice_titles: { title: '常用发票抬头', desc: '个人/企业发票抬头簿' },
  // 营销域
  coupon_templates: { title: '优惠券模板', desc: '满减/折扣券模板与成本承担' },
  coupon_batches: { title: '券批次', desc: '发放批次、库存与预算' },
  user_coupons: { title: '用户优惠券', desc: '个人券 AVAILABLE/LOCKED/USED/EXPIRED/VOID' },
  campaigns: { title: '营销活动', desc: '拉新/冲单/补贴等活动与预算' },
  invitations: { title: '邀请关系', desc: '邀请人与被邀请人及奖励状态' },
  // 评价客服域
  reviews: { title: '评价', desc: '乘客/司机互评，星级标签匿名与申诉' },
  tickets: { title: '客服工单', desc: '投诉/费用异议/安全工单，SLA 升级' },
  ticket_logs: { title: '工单处理日志', desc: '工单流转操作记录' },
  lost_items: { title: '遗失物品', desc: '报失、匹配、认领与归还' },
  // 风控安全域
  risk_events: { title: '风控事件', desc: '刷单/虚假定位/薅券等风险事件处置' },
  risk_rules: { title: '风控规则', desc: '硬规则+评分卡，灰度与回滚' },
  blacklists: { title: '黑名单', desc: '用户/司机/设备/手机号/银行卡/IP 黑名单' },
  safety_incidents: { title: '安全事件', desc: 'SOS/偏航/长停/事故等安全闭环' },
  // 通知域
  message_templates: { title: '消息模板', desc: '短信/推送/站内信模板与频控' },
  notifications: { title: '消息发送记录', desc: '消息触达、已读与重试' },
  announcements: { title: '平台公告', desc: '弹窗/横幅/消息中心公告' },
  announcement_reads: { title: '公告已读', desc: '公告已读与强制确认记录' },
  // 奖惩排班域
  driver_reward_penalties: { title: '司机奖惩记录', desc: '冲单奖/时段补贴/罚款与申诉' },
  driver_schedule_tasks: { title: '司机排班与时长任务', desc: '在线时长/完单任务报名与领奖' },
  // 拼车域
  carpool_groups: { title: '拼车组', desc: '两订单同起终点共乘，封组与座位' },
  // 系统域
  admins: { title: '管理端账号', desc: '后台账号、角色、数据范围与登录' },
  roles: { title: '角色', desc: 'RBAC 角色与权限点' },
  permissions: { title: '权限点', desc: '菜单/按钮/接口权限' },
  audit_logs: { title: '操作日志', desc: '后台操作审计，前后快照' },
  system_configs: { title: '系统配置', desc: '订单/支付/派单/安全等参数' },
  dicts: { title: '数据字典', desc: '取消原因/投诉分类/券类型等字典' },
  files: { title: '文件资源', desc: '上传文件、加密存储与业务关联' },
  outbox_events: { title: '可靠事件表', desc: '跨服务可靠事件投递与消费者去重' }
};

const existing = new Set(db.getCollectionNames());

function applyMeta(name, schema, info) {
  const meta = COLLECTION_META[name];
  if (!meta) return;
  schema.title = meta.title;
  schema.description = meta.desc;
  db.runCommand({
    collMod: name,
    validator: { $jsonSchema: schema },
    validationLevel: (info.options && info.options.validationLevel) || 'moderate',
    validationAction: (info.options && info.options.validationAction) || 'error'
  });
}

function createCollection(col) {
  const meta = COLLECTION_META[col.name];
  const label = meta ? (meta.title + '｜' + meta.desc) : '';
  if (existing.has(col.name)) {
    // 已存在：collMod 补写中文备注（title/description）
    try {
      const info = db.getCollectionInfos({ name: col.name })[0];
      const o = info.options || {};
      const schema = (o.validator && o.validator.$jsonSchema) || { bsonType: 'object', properties: {} };
      if (meta && schema.title !== meta.title) {
        applyMeta(col.name, schema, info);
        print('已补充中文备注：' + col.name + '（' + meta.title + '）');
      } else {
        print('已存在，跳过：' + col.name + (meta ? '（' + meta.title + '）' : ''));
      }
    } catch (e) {
      print('已存在，跳过：' + col.name + '（备注更新失败：' + e.message + '）');
    }
    return;
  }
  db.createCollection(col.name, {
    validator: {
      $jsonSchema: {
        bsonType: 'object',
        title: meta ? meta.title : col.name,
        description: meta ? meta.desc : '',
        additionalProperties: false,
        required: col.required,
        properties: { _id: R('MongoDB 自动生成的主键'), ...col.fields }
      }
    },
    validationLevel: 'moderate',
    validationAction: 'error'
  });
  for (const index of col.indexes || []) {
    const { key, ...options } = index;
    db.getCollection(col.name).createIndex(key, options);
  }
  print('已创建：' + col.name + '（' + meta.title + '｜字段 ' + Object.keys(col.fields).length
    + ' 个，索引 ' + (col.indexes || []).length + ' 个）');
}

print('\n========== 花小猪打车 最终全量建集合 ==========');
print('目标数据库：' + DB_NAME);
print('集合定义总数：' + collections.length);

let created = 0;
for (const col of collections) {
  if (!existing.has(col.name)) created++;
  createCollection(col);
}

print('\n========== 完成 ==========');
print('本次新建集合：' + created + ' 个');
print('数据库集合总数：' + db.getCollectionNames().length + ' 个');
print('并发写集合（需 version 乐观锁）：' + CONCURRENT_SET.join(', '));
print('提醒：本脚本为「只增不改」基线。字段级变更须先更新《06-MongoDB数据字典》再评审，否则打回。');
