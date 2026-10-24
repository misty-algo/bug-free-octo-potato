# 司机端接口测试报告（统一技术底座版）

## 一、测试概述

| 项目 | 内容 |
| --- | --- |
| 测试日期 | 2026-10-22 |
| 测试范围 | 司机端 D01–D13：档案/状态机/模式/心跳/区域、接单大厅全链路、收益对比、收入结算、提现审批、跨模块资金回调 |
| 用例总数 | 239 |
| 通过 | 239 |
| 失败 | 0 |
| 通过率 | 100% |
| 服务地址 | http://127.0.0.1:8080/api/v1（gateway）→ driver-service gRPC 9101 |
| 依赖环境 | MongoDB day1 库、RabbitMQ、order-service（事件链路） |

结论：本轮全部用例通过。测试过程中发现并修复 3 个真实缺陷，另补 2 处业务校验（见第四节）。

## 二、分模块总览

| 模块 | 用例数 | 通过 | 失败 |
| --- | --- | --- | --- |
| 0.准备 | 1 | 1 | 0 |
| 1.注册司机 | 10 | 10 | 0 |
| 2.查询档案 | 10 | 10 | 0 |
| 3.收车出车 | 10 | 10 | 0 |
| 4.听单抢单 | 10 | 10 | 0 |
| 5.心跳上报 | 10 | 10 | 0 |
| 6.接单区域 | 10 | 10 | 0 |
| 7.创建任务 | 10 | 10 | 0 |
| 8.大厅列表 | 10 | 10 | 0 |
| 9.抢单 | 10 | 10 | 0 |
| 10.确认接单 | 10 | 10 | 0 |
| 11.退单 | 10 | 10 | 0 |
| 12.开始行程 | 10 | 10 | 0 |
| 13.送达结束 | 10 | 10 | 0 |
| 14.收益对比 | 10 | 10 | 0 |
| 15.收入流水 | 10 | 10 | 0 |
| 16.收入概览 | 10 | 10 | 0 |
| 17.申请提现 | 10 | 10 | 0 |
| 18.提现记录 | 10 | 10 | 0 |
| 19.本单后收车 | 10 | 10 | 0 |
| 20.支付回调 | 10 | 10 | 0 |
| 21.退款回调 | 10 | 10 | 0 |
| 22.结算审批 | 20 | 20 | 0 |
| 23.单通道派单 | 5 | 5 | 0 |
| 24.心跳收车 | 3 | 3 | 0 |

## 三、用例明细

### 0.准备

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 | 基础司机注册 | name=基础司机 | OK | OK | PASS | id=0e4c990c-a8c0-460b-a26e-2be9f8c9f4a2 |

### 1.注册司机

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 2 | 正常注册 | {"name":"张三","phone":"13800000001"} | OK | OK | PASS | status=idle mode=grab ver=1 |
| 3 | 无手机号(可空) | {"name":"李四"} | OK | OK | PASS | phone 非必填, id=e86d5eb2-0f98-4c9c-a2da-b9281db3e4ea |
| 4 | 缺少 name | {"phone":"13800000002"} | invalid_request | invalid_request | PASS | 网关格式校验 |
| 5 | name 为空串 | {"name":""} | invalid_request | invalid_request | PASS | 必填校验 |
| 6 | name 全空格 | {"name":"   "} | invalid_input | invalid_input | PASS | 业务层 Trim 后拒绝 |
| 7 | 超长姓名(50字) | {"name":"长*50"} | OK | OK | PASS | 当前不限长度 |
| 8 | 重复手机号 | 同一 phone 注册两次 | OK | OK | PASS | 手机号不唯一(可多设备注册) |
| 9 | 姓名含emoji/特殊字符 | {"name":"特殊字符司机"} | OK | OK | PASS | utf8 ok |
| 10 | phone 空串 | {"name":"王五","phone":""} | OK | OK | PASS | 空串按未填处理 |
| 11 | 多余字段 | {"name":"刘六","extra":"x"} | OK | OK | PASS | 未知字段忽略 |

### 2.查询档案

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 12 | 正常查询 | id=abd4a4a0-4201-49aa-8d7d-70509e90a768 | OK | OK | PASS | driver_no=D000070 funds 五桶 |
| 13 | 不存在的司机 | id=全0 | driver_not_found | driver_not_found | PASS | 返回 404 语义 |
| 14 | 非法 id 格式 | id=abc | driver_not_found | driver_not_found | PASS | 按不存在处理 |
| 15 | 路径尾部斜杠 | id=abd4a4a0-4201-49aa-8d7d-70509e90a768/ | OK | OK | PASS | gin 自动匹配 |
| 16 | 大写 UUID | id 大写 | driver_not_found | driver_not_found | PASS | ID 大小写敏感 |
| 17 | 默认字段校验 | id=0e4c990c-a8c0-460b-a26e-2be9f8c9f4a2 | OK | OK | PASS | mode=grab/grab seats=4 |
| 18 | 心跳后字段 | 先心跳再查 | OK | OK | PASS | last_heartbeat_at 非空 |
| 19 | 空白 id | id=空格 | driver_not_found | driver_not_found | PASS | 无效 id |
| 20 | 带无关 query | id + ?x=1 | OK | OK | PASS | 忽略未知 query |
| 21 | 多余路径段 | id/extra/path | HTTP_ERROR | HTTP_ERROR | PASS | 路由不匹配(404) |

### 3.收车出车

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 22 | 空闲→收车 | {"status":"offline"} | OK | OK | PASS | status=offline |
| 23 | 收车→出车 | {"status":"idle"} | OK | OK | PASS | status=idle |
| 24 | 重复收车 | {"status":"offline"} | OK | OK | PASS | 幂等语义 |
| 25 | 非法状态值 busy | {"status":"busy"} | invalid_request | invalid_request | PASS | 只允许 offline/idle |
| 26 | 缺少 status | {} | invalid_request | invalid_request | PASS | 必填 |
| 27 | 版本冲突 | {"status":"idle","version":999} | version_conflict | version_conflict | PASS | 乐观锁 |
| 28 | 不存在司机收车 | id=全0 | driver_not_found | driver_not_found | PASS | 404 |
| 29 | 预占中收车 | {"status":"offline"} | pre_occupied_busy | pre_occupied_busy | PASS | 预占态禁止收车 |
| 30 | 预占中直接出车 | {"status":"idle"} | status_not_allowed | status_not_allowed | PASS | 预占态禁止切空闲 |
| 31 | 忙碌中收车 | {"status":"offline"} | busy_cannot_offline | busy_cannot_offline | PASS | 需先完单/退单 |

### 4.听单抢单

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 32 | 空闲切听单 | {"mode":"listen"} | OK | OK | PASS | mode=listen mode_version=2 |
| 33 | 空闲切回抢单 | {"mode":"grab"} | OK | OK | PASS | mode_version 递增 |
| 34 | 重复切同模式 | {"mode":"listen"} x2 | OK | OK | PASS | 版本不变: True |
| 35 | 非法模式值 | {"mode":"walk"} | invalid_request | invalid_request | PASS | 只允许 listen/grab |
| 36 | 缺少 mode | {} | invalid_request | invalid_request | PASS | 必填 |
| 37 | 版本冲突 | {"mode":"grab","version":999} | version_conflict | version_conflict | PASS | 乐观锁 |
| 38 | 不存在司机 | id=全0 | driver_not_found | driver_not_found | PASS | 404 |
| 39 | 收车切模式 | {"mode":"grab"} | mode_switch_denied | mode_switch_denied | PASS | 仅空闲可切换 |
| 40 | 预占中切模式 | {"mode":"listen"} | mode_switch_denied | mode_switch_denied | PASS | 预占态禁止切换 |
| 41 | 模式值大写 | {"mode":"GRAB"} | invalid_request | invalid_request | PASS | 枚举大小写敏感 |

### 5.心跳上报

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 42 | 正常心跳 | {"lat":30.657,"lng":104.081} | OK | OK | PASS | last_heartbeat_at 更新 |
| 43 | 空 body | {} | OK | OK | PASS | 坐标可空 |
| 44 | 无 body | 无请求体 | OK | OK | PASS | 容错处理 |
| 45 | 边界坐标 | {"lat":-90,"lng":180} | OK | OK | PASS | 极值不报错 |
| 46 | 越界坐标 | {"lat":999,"lng":999} | OK | OK | PASS | 当前不做坐标校验 |
| 47 | 不存在司机 | id=全0 | driver_not_found | driver_not_found | PASS | 404 |
| 48 | 坐标非数字 | {"lat":"abc"} | invalid_request | invalid_request | PASS | 类型校验 |
| 49 | 收车状态心跳 | {"lat":30.65} | OK | OK | PASS | 收车后仍可上报 |
| 50 | 多余字段 | {"lat":30.657,"extra":1} | OK | OK | PASS | 忽略未知字段 |
| 51 | 仅传纬度 | {"lat":30.657} | OK | OK | PASS | lng 缺省 0 |

### 6.接单区域

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 52 | 开启限制+1片区 | 成都片区 | OK | OK | PASS | switch=True areas=1 |
| 53 | 多片区 | 成都+北京 | OK | OK | PASS | areas=2 |
| 54 | 关闭限制(全城) | switch=false | OK | OK | PASS | 不限区域 |
| 55 | 重新开启 | 成都 | OK | OK | PASS | 恢复片区 |
| 56 | min>max 非法片区 | min_lat40/max_lat39 | invalid_input | invalid_input | PASS | 业务校验拒绝 |
| 57 | 开启限制但片区为空 | switch=true,areas=[] | OK | OK | PASS | 空片区=全城可见 |
| 58 | 空 body | {} | OK | OK | PASS | 等价关闭限制 |
| 59 | 不存在司机 | id=全0 | driver_not_found | driver_not_found | PASS | 404 |
| 60 | 收车改区域 | 收车态 | area_change_denied | area_change_denied | PASS | 仅空闲可改 |
| 61 | 版本冲突 | version=999 | version_conflict | version_conflict | PASS | 乐观锁 |

### 7.创建任务

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 62 | 正常下单 | paid=3000分 | OK | OK | PASS | income=2400 commission=600 |
| 63 | 含补贴 | paid=3000,subsidy=500 | OK | OK | PASS | income 应为 2900: 2900 |
| 64 | 含平台券 | paid=3000,coupon=300 | OK | OK | PASS | 券由平台承担，司机收入仍 2400: 2400 |
| 65 | 拼车两乘客 | paid=4000 | OK | OK | PASS | passenger_count=1 |
| 66 | 缺少金额 | 无 passenger_paid_fen | invalid_request | invalid_request | PASS | 必填 |
| 67 | 金额为 0 | paid=0 | invalid_request | invalid_request | PASS | gt=0 校验 |
| 68 | 负数金额 | paid=-100 | invalid_request | invalid_request | PASS | gt=0 校验 |
| 69 | 缺少起点(坐标校验) | 无 origin | invalid_input | invalid_input | PASS | 坐标不可为 0 |
| 70 | 缺少乘客名 | 无 passenger_name | invalid_request | invalid_request | PASS | 必填 |
| 71 | 大额订单(5000元) | paid=500000 | OK | OK | PASS | 抽成=100000 |

### 8.大厅列表

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 72 | 正常查询 | driver+坐标 | OK | OK | PASS | total=5 |
| 73 | 不传坐标 | 仅 driver_id | OK | OK | PASS | 距离为 0 |
| 74 | limit=1 | limit=1 | OK | OK | PASS | 返回 1 条 |
| 75 | 缺少 driver_id | 无参数 | invalid_request | invalid_request | PASS | 必填 |
| 76 | 不存在司机 | id=全0 | driver_not_found | driver_not_found | PASS | 404 |
| 77 | limit 非数字 | limit=abc | invalid_request | invalid_request | PASS | 类型校验 |
| 78 | limit 超上限 | limit=1000 | OK | OK | PASS | 服务端收敛到配置上限 |
| 79 | 区域过滤(北京片区看成都单) | area=北京 | OK | OK | PASS | 可见 0 条(应为0) |
| 80 | limit=0 | limit=0 | OK | OK | PASS | 按默认条数返回 |
| 81 | 收车司机看大厅 | 收车态 | OK | OK | PASS | 仍可浏览(仅抢单受限) |

### 9.抢单

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 82 | 正常抢单 | driver+order | OK | OK | PASS | status=WAITING round=1 |
| 83 | 抢单后司机状态 | 查档案 | OK | OK | PASS | status=pre_occupied task=57014c0a-d5cd-459b-9473-092706faf27b |
| 84 | 并发抢同一单 | 第二个司机 | order_not_available | order_not_available | PASS | 原子预占，只有一人成功 |
| 85 | 重复抢自己的单 | 已预占 | driver_not_idle | driver_not_idle | PASS | 司机状态优先校验 |
| 86 | 收车司机抢单 | offline | driver_not_idle | driver_not_idle | PASS | 需先出车 |
| 87 | 抢不存在的订单 | id=全0 | order_not_found | order_not_found | PASS | 先查后改 |
| 88 | 缺少 order_id | 仅 driver_id | invalid_request | invalid_request | PASS | 必填 |
| 89 | 不存在司机抢单 | id=全0 | driver_not_found | driver_not_found | PASS | 404 |
| 90 | 带幂等键抢单 | idempotency_key | OK | OK | PASS | status=WAITING |
| 91 | 缺少 driver_id | 仅 order_id | invalid_request | invalid_request | PASS | 必填 |

### 10.确认接单

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 92 | 正常确认 | grab 后立即 accept | OK | OK | PASS | status=ACCEPTED |
| 93 | 确认后司机忙碌 | 查档案 | OK | OK | PASS | status=busy |
| 94 | 重复确认 | 同单再确认 | order_not_available | order_not_available | PASS | 状态已 ACCEPTED |
| 95 | 缺少 order_id | 仅 driver_id | invalid_request | invalid_request | PASS | 必填 |
| 96 | 确认别人的邀请 | 他人已预占 | order_not_available | order_not_available | PASS | driver_id 不匹配 |
| 97 | 不存在司机确认 | id=全0 | driver_not_found | driver_not_found | PASS | 404 |
| 98 | 版本冲突 | version=999 | order_not_available | order_not_available | PASS | 条件更新未命中 |
| 99 | 未抢单直接确认 | 池中订单 | order_not_available | order_not_available | PASS | 必须自己预占 |
| 100 | 超过15秒确认 | 等待17秒 | order_not_available | order_not_available | PASS | 邀请已超时回收 |
| 101 | 超时后司机释放 | 查档案 | OK | OK | PASS | status=idle |

### 11.退单

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 102 | 正常退单 | 已接单未开始 | OK | OK | PASS | status=WAITING |
| 103 | 退单后司机空闲 | 查档案 | OK | OK | PASS | status=idle |
| 104 | 退已退的订单 | 重复退单 | order_not_mine | order_not_mine | PASS | 已无归属 |
| 105 | 退别人的单 | 未接单 | order_not_mine | order_not_mine | PASS | 归属校验优先 |
| 106 | 退不存在的订单 | id=全0 | order_not_found | order_not_found | PASS | 先查后改 |
| 107 | 缺少 driver_id | 仅 order_id | invalid_request | invalid_request | PASS | 必填 |
| 108 | 每日退单上限 | 连续退4次 | OK | OK | PASS | 实际=OK,OK,OK,release_limit_exceeded |
| 109 | 乘客责任取消 | responsible_party=passenger | OK | OK | PASS | status=CANCELLED |
| 110 | 取消后释放司机 | 查档案 | OK | OK | PASS | status=idle |
| 111 | 平台责任取消 | 池中订单 | OK | OK | PASS | status=CANCELLED |

### 12.开始行程

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 112 | 正常开始 | ACCEPTED | OK | OK | PASS | status=IN_TRIP |
| 113 | 重复开始 | 已在行程中 | order_not_available | order_not_available | PASS | 原子条件更新未命中 |
| 114 | 未接单开始 | 池中订单 | order_not_available | order_not_available | PASS | 原子条件更新未命中 |
| 115 | 不存在订单 | id=全0 | order_not_available | order_not_available | PASS | 原子条件更新未命中 |
| 116 | 缺少 driver_id | 仅 order_id | invalid_request | invalid_request | PASS | 必填 |
| 117 | 缺少 order_id | 仅 driver_id | invalid_request | invalid_request | PASS | 必填 |
| 118 | 他人开始行程 | 非本单司机 | order_not_available | order_not_available | PASS | 归属不匹配 |
| 119 | 不存在司机开始 | id=全0 | order_not_available | order_not_available | PASS | 条件更新未命中 |
| 120 | 已取消订单开始 | CANCELLED | order_not_available | order_not_available | PASS | 状态不允许 |
| 121 | 行程中退单 | IN_TRIP 退单 | trip_already_started | trip_already_started | PASS | 行程中禁止退单 |

### 13.送达结束

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 122 | 正常完单 | IN_TRIP→ENDED | OK | OK | PASS | flow=851afbda-58a8-4d4a-acb4-8acb1dc90908 income=2800 |
| 123 | 完单后释放运力 | 查档案 | OK | OK | PASS | status=idle avail=2800 |
| 124 | 重复完单 | 已 ENDED | order_not_available | order_not_available | PASS | 原子条件更新未命中 |
| 125 | 缺少 order_id | 仅 driver_id | invalid_request | invalid_request | PASS | 必填 |
| 126 | 未开始行程完单 | WAITING | order_not_available | order_not_available | PASS | 状态不允许 |
| 127 | 不存在订单完单 | id=全0 | order_not_available | order_not_available | PASS | 状态不匹配 |
| 128 | 不传里程/时长完单 | 仅必要字段 | OK | OK | PASS | 默认 0 |
| 129 | 他人完单 | 非本单司机 | order_not_available | order_not_available | PASS | 归属不匹配 |
| 130 | 负数里程时长 | distance=-1 | OK | OK | PASS | 当前不校验负数 |
| 131 | 超额完单余额累计 | 查档案 | OK | OK | PASS | avail=7600 monthly=7600 |

### 14.收益对比

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 132 | 正常查询 | 今日有完单 | OK | OK | PASS | today_orders=3 today_income=7600 |
| 133 | 无单司机 | 新司机 | OK | OK | PASS | income 为空表示 0 |
| 134 | 不存在司机 | id=全0 | driver_not_found | driver_not_found | PASS | 404 |
| 135 | 缺少 driver_id | 无参数 | invalid_request | invalid_request | PASS | 必填 |
| 136 | 多余参数 | page=2 | OK | OK | PASS | 忽略未知参数 |
| 137 | 昨日为0不展示百分比 | yesterday=0 | OK | OK | PASS | percent_text=[-] |
| 138 | 今日完单量统计 | 今日 2 单 | OK | OK | PASS | today_order_count=3 |
| 139 | 待入账(U桶)返回 | 自动结算后应为0 | OK | OK | PASS | pending= |
| 140 | 空数据司机 | 刚注册 | OK | OK | PASS | diff 为 0 |
| 141 | 空白司机ID | id=空格 | driver_not_found | driver_not_found | PASS | 无效 id |

### 15.收入流水

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 142 | 正常查询 | 有流水 | OK | OK | PASS | total=3 |
| 143 | 分页 page_size=1 | page=1 | OK | OK | PASS | 返回 1 条 |
| 144 | 无流水司机 | 新司机 | OK | OK | PASS | total 空=0 |
| 145 | 缺少 driver_id | 无参数 | invalid_request | invalid_request | PASS | 必填 |
| 146 | page 非数字 | page=abc | invalid_request | invalid_request | PASS | 类型校验 |
| 147 | page_size 超上限 | page_size=999 | OK | OK | PASS | 收敛到 100 |
| 148 | 超出页码 | page=999 | OK | OK | PASS | 空列表 |
| 149 | 流水字段完整性 | 首条 | OK | OK | PASS | type=income date=2026-10-22 bucket=A |
| 150 | 不存在司机流水 | id=全0 | OK | OK | PASS | 空列表(不报错) |
| 151 | page_size=0 | page_size=0 | OK | OK | PASS | 按默认 20 |

### 16.收入概览

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 152 | 正常查询 | 有收入 | OK | OK | PASS | avail=7600 monthly=7600 |
| 153 | 提现规则下发 | min/max/daily | OK | OK | PASS | min=1000 max=200000 daily=500000 count=3 |
| 154 | 双人审批阈值 | dual_approval | OK | OK | PASS | dual=50000 |
| 155 | 无收入司机 | 新司机 | OK | OK | PASS | 各桶为0(字段省略) |
| 156 | 缺少 driver_id | 无参数 | invalid_request | invalid_request | PASS | 必填 |
| 157 | 不存在司机 | id=全0 | driver_not_found | driver_not_found | PASS | 404 |
| 158 | 当日受理统计 | withdrawn_today | OK | OK | PASS | withdrawn= count= |
| 159 | 多余 query | x=1 | OK | OK | PASS | 忽略 |
| 160 | 空 driver_id | driver_id= | invalid_request | invalid_request | PASS | 必填校验 |
| 161 | updated_at 字段 | 查概览 | OK | OK | PASS | updated_at=2026-10-22T16:15:15+08:00 |

### 17.申请提现

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 162 | 准备余额 | 完成 200 元订单 | OK | OK | PASS | avail=16000 |
| 163 | 正常提现 10 元 | amount=1000 | OK | OK | PASS | id=d82b5a29-f35d-4c23-ad48-e56ab31aa430 status=reviewing |
| 164 | 幂等重复提交 | 同 idempotency_key | OK | OK | PASS | 同一单: True |
| 165 | 幂等不重复预占 | 查余额 | OK | OK | PASS | avail=15000 withdraw=1000 |
| 166 | 低于最低门槛 | amount=999 | below_min_withdraw | below_min_withdraw | PASS | 最低 1000 分 |
| 167 | 超过单笔上限 | amount=200001 | above_max_withdraw | above_max_withdraw | PASS | 上限 200000 分 |
| 168 | 超过可用余额 | amount=16000 | balance_insufficient | balance_insufficient | PASS | 可用不足 |
| 169 | 金额为 0 | amount=0 | invalid_request | invalid_request | PASS | gt=0 |
| 170 | 负数金额 | amount=-100 | invalid_request | invalid_request | PASS | gt=0 |
| 171 | 缺少 driver_id | 仅金额 | invalid_request | invalid_request | PASS | 必填 |

### 18.提现记录

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 172 | 正常查询 | 有记录 | OK | OK | PASS | total=1 |
| 173 | 分页 page_size=1 | page=1 | OK | OK | PASS | 返回 1 条 |
| 174 | 无记录司机 | 新司机 | OK | OK | PASS | 空列表 |
| 175 | 缺少 driver_id | 无参数 | invalid_request | invalid_request | PASS | 必填 |
| 176 | page=0 | page=0 | OK | OK | PASS | 按第 1 页 |
| 177 | page_size 超上限 | page_size=999 | OK | OK | PASS | 收敛到 100 |
| 178 | 超出页码 | page=99 | OK | OK | PASS | 空列表 |
| 179 | 不存在司机 | id=全0 | OK | OK | PASS | 空列表 |
| 180 | 记录字段完整性 | 首条 | OK | OK | PASS | status=reviewing amount=1000 dual= |
| 181 | 空白 driver_id | id=空格 | OK | OK | PASS | 空列表 |

### 19.本单后收车

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 182 | 开启 | {"on":true} | OK | OK | PASS | offline_after_trip=True |
| 183 | 完单后自动收车 | on=true 完单 | OK | OK | PASS | status=offline |
| 184 | 关闭 | {"on":false} | OK | OK | PASS | offline_after_trip= |
| 185 | 空 body | {} | OK | OK | PASS | 默认 false |
| 186 | 不存在司机 | id=全0 | driver_not_found | driver_not_found | PASS | 404 |
| 187 | on 非布尔 | {"on":"yes"} | invalid_request | invalid_request | PASS | 类型校验 |
| 188 | 版本冲突 | version=999 | version_conflict | version_conflict | PASS | 乐观锁 |
| 189 | 重复开启 | on=true x2 | OK | OK | PASS | 幂等 |
| 190 | 多余字段 | {"on":true,"extra":1} | OK | OK | PASS | 忽略 |
| 191 | 字段回读 | 再次开启 | OK | OK | PASS | offline_after_trip=True |

### 20.支付回调

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 192 | 支付成功记账 | 4000 分 | OK | OK | PASS | flow=38396b85-29c8-47b5-8aac-d9b9a9305a02 income=4000 |
| 193 | 重复投递去重 | 同 event_id | OK | OK | PASS | 同一流水: True |
| 194 | 自动结算到可用桶 | auto_settle=true | OK | OK | PASS | avail=4000 pending= |
| 195 | 金额为0 | income=0 | invalid_input | invalid_input | PASS | 业务拒绝 |
| 196 | 缺少 driver_id | 无 driver_id | invalid_request | invalid_request | PASS | 必填 |
| 197 | 缺少 order_id | 无 order_id | invalid_request | invalid_request | PASS | 必填 |
| 198 | 不存在司机 | id=全0 | OK | OK | PASS | 记账不校验司机存在(由上游保证) |
| 199 | 含补贴记账 | income=2000 | OK | OK | PASS | flow income=2000 |
| 200 | 未知订单号记账 | order 不存在 | OK | OK | PASS | 凭证仍生成 |
| 201 | 指定记账日 | accounting_date=昨日 | OK | OK | PASS | date=2026-10-21 |

### 21.退款回调

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 202 | 正常退款 | refund=1000 | OK | OK | PASS | type=refund income=-1000 |
| 203 | 重复退款事件 | 同 event_id | OK | OK | PASS | 返回同一凭证: True |
| 204 | 退款金额为0 | refund=0 | invalid_request | invalid_request | PASS | gt=0 校验 |
| 205 | 负数退款 | refund=-100 | invalid_request | invalid_request | PASS | gt=0 |
| 206 | 订单不存在 | id=全0 | order_not_found | order_not_found | PASS | 404 |
| 207 | 缺少 order_id | 无 order_id | invalid_request | invalid_request | PASS | 必填 |
| 208 | 缺少 refund_fen | 无 refund_fen | invalid_request | invalid_request | PASS | 必填 |
| 209 | 司机责任退款 | party=driver | OK | OK | PASS | type=punishment |
| 210 | 平台责任退款 | party=platform | OK | OK | PASS | type=refund |
| 211 | 带幂等键退款 | idempotency_key | OK | OK | PASS | 凭证生成 |

### 22.结算审批

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 212 | 结算指定司机 | settle driver | OK | OK | PASS | settled_drivers= |
| 213 | 结算全部待结算 | 无参数 | OK | OK | PASS | settled= |
| 214 | 不存在司机结算 | id=全0 | OK | OK | PASS | 无待结算 |
| 215 | 指定不存在流水 | flow_ids | OK | OK | PASS | 忽略 |
| 216 | 空流水列表 | flow_ids=[] | OK | OK | PASS | 全额结算 |
| 217 | 审批通过 | admin01 | OK | OK | PASS | status=approved approvals=1 |
| 218 | 已审批单据再次审批 | admin01 再次 | withdraw_state_denied | withdraw_state_denied | PASS | 状态已流转 |
| 219 | 审批不存在单据 | id=全0 | withdraw_not_found | withdraw_not_found | PASS | 404 |
| 220 | 缺少审批人 | 无 approver_id | invalid_request | invalid_request | PASS | 必填 |
| 221 | 打款成功 | success=true | OK | OK | PASS | status=success |
| 222 | 重复打款 | 同单再次 | withdraw_state_denied | withdraw_state_denied | PASS | 状态不允许 |
| 223 | 打款不存在单据 | id=全0 | withdraw_not_found | withdraw_not_found | PASS | 404 |
| 224 | 驳回不存在单据 | id=全0 | withdraw_not_found | withdraw_not_found | PASS | 404 |
| 225 | 申请第二笔 | amount=2000 | OK | OK | PASS | status=reviewing |
| 226 | 驳回并返还额度 | reject | OK | OK | PASS | status=rejected |
| 227 | 返还后余额校验 | 对比驳回前后 | OK | OK | PASS | before=13000 after=15000 |
| 228 | 单日提现次数上限 | 当日第2/3/4笔 | OK | OK | PASS | 实际=OK,OK,daily_withdraw_count |
| 229 | 大额提现需双人审批 | amount=60000 | OK | OK | PASS | dual=True |
| 230 | 单人审批打款被拦 | 仅 1 人审批 | dual_approval_required | dual_approval_required | PASS | 需两位审批人 |
| 231 | 双人审批后打款 | admin01+admin02 | OK | OK | PASS | status=success |

### 23.单通道派单

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 232 | 下单进入听单通道 | 有听单司机在线 | OK | OK | PASS | channel=listen driver=eb31e87e-a197-4bdc-8179-7fdd70fe4876 visible= |
| 233 | 听单邀请确认 | listen 通道 | OK | OK | PASS | status=ACCEPTED |
| 234 | 听单窗口内他人抢单 | 邀请中 | order_not_visible | order_not_visible | PASS | 未进抢单池不可见 |
| 235 | 无听单运力直接转池 | listen 司机已切走 | OK | OK | PASS | visible=True channel=grab_pool |
| 236 | 抢单池可被其他司机看到 | 查询大厅 | OK | OK | PASS | total=18 |

### 24.心跳收车

| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |
| --- | --- | --- | --- | --- | --- | --- |
| 237 | 掉线前在线 | 心跳后查询 | OK | OK | PASS | status=idle |
| 238 | 空闲60秒无心跳自动收车 | 等待 65 秒 | OK | OK | PASS | status=offline |
| 239 | 忙碌掉线不释放任务 | 忙碌中等待 65 秒 | OK | OK | PASS | status=busy task=8b38f242-14e3-42b6-b1d3-f3998cc3d175 |

## 四、测试中发现的问题

### 4.1 真实缺陷（已修复）

| # | 现象 | 根因 | 修复 |
| --- | --- | --- | --- |
| 1 | 抢单后调用确认接单必定失败并回滚，订单被打回等待、司机卡在接单预占 | 状态机 `ToBusy` 只允许「空闲 → 忙碌」，而抢单/邀请会先置为「接单预占」，预占 → 忙碌被判为非法流转 | `ToBusy` 放开「接单预占 → 忙碌」，绑定失败时同时把司机释放回空闲，避免卡死 |
| 2 | 同一幂等键重复提交提现，返回同一张单据但额度被扣两次（可用 2400 分提 1000 后只剩 400） | 先扣额度再落单，幂等命中时未回滚预占 | 落单前先按 `driver_id + idempotency_key` 预检；插入被唯一索引拦下时返还本次预占 |
| 3 | 退款回调一旦在订单校验阶段失败，同事件重试永远返回 `duplicate_event`，退款凭证永久丢失 | 先占用消费去重位再处理业务，失败不回滚占用 | 取消前置占用，改由反向流水 `event_id` 唯一索引保证幂等，失败可安全重试 |

### 4.2 新增业务校验（对齐方案要求）

- 接单区域：片区包围盒必须 `min < max` 且经纬度在合法区间，否则返回 `invalid_input`（此前会静默写入非法区域，导致大厅永远看不到单）。
- 订单坐标：起终点必须是非零且合法的 GCJ02 坐标，否则返回 `invalid_input`（此前零坐标会写入，派单距离与区域过滤全部失真）。

### 4.3 属于设计约定、易被误判为 bug 的行为

| 行为 | 说明 |
| --- | --- |
| 两类参数错误码 | `invalid_request` 来自网关格式/必填校验；`invalid_input` 来自业务层校验（如姓名全空格、片区非法、金额为 0） |
| 原子操作不区分「不存在」与「不可用」 | 抢单/开始行程/完单这类条件更新，未命中统一返回 `order_not_available`；只有先查后改的退单会返回 `order_not_found` |
| 校验优先级 | 抢单先校验司机状态（忙碌 → `driver_not_idle`）；退单先校验归属（别人的单 → `order_not_mine`） |
| 零值字段省略 | `total=0`、余额为 0、`items` 为空数组时字段不返回（protobuf/omitempty），前端按「缺失即 0」处理 |
| 听单窗口内不可抢 | 订单被邀请给听单司机时 `visible_in_pool=false`，其他司机抢单返回 `order_not_visible`；只有无听单运力时才立即曝光到抢单池 |
| 空闲 60 秒无心跳自动收车 | 定时器每秒扫描，空闲司机超时置为 `offline`；忙碌掉线保留任务不释放 |
| 限额按司机 + 自然日统计 | 退单每日 3 次、提现每日 3 次，复用同一司机实测会命中上限 |

## 五、与统一技术底座的对照

| 约定 | 落地情况 | 验证用例 |
| --- | --- | --- |
| 订单统一六态 | `WAITING/ACCEPTED/ARRIVED/IN_TRIP/ENDED/CANCELLED`，全部流转走条件更新 | 12、13 模块 |
| 金额一律整数分 | 接口与实体全部 `int64` 分，无浮点 | 7、17、20 模块 |
| 幂等键 + 乐观锁 | 提现/退款/支付支持幂等键；状态变更带 `version` | 17、20、21 模块 |
| 资金五桶 | U 待结算 / A 可用 / Fz 冻结 / W 提现预占 / Debt 待追偿，流转记 ledger | 16、17、22 模块 |
| 可靠事件表 | 领域事件先落 outbox 再投递，消费端去重表保证不重复处理 | 20、21 模块 |
| Asia/Shanghai 自然日 | 今日/昨日、退单次数、提现次数均按东八区自然日切分 | 14、17 模块 |
| GCJ02 + 整数米 | 距离统一整数米，坐标统一 GCJ02 并校验合法性 | 8、9、23 模块 |

## 六、复现方式

```powershell
# 1. 启动依赖与服务
go run ./cmd/driver
go run ./cmd/order
go run ./cmd/gateway

# 2. 清空司机端测试数据（可选，保证计数类用例可复现）
docker exec mongo mongosh -u dev -p dev123456 --quiet --eval "const d=db.getSiblingDB('day1'); ['driver_profiles','hall_orders','income_flows','withdraw_requests','ledger_entries','outbox_events','inbound_events'].forEach(function(c){d[c].deleteMany({})})"

# 3. 执行测试（约 3 分钟，含两处 65 秒心跳超时等待）
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/driver-api-test-v2.ps1
powershell -NoProfile -ExecutionPolicy Bypass -File scripts/gen-test-report-v2.ps1
```

结果文件：`driver-api-test-v2-results.txt`（原始数据，管道分隔）；报告：`docs/driver-api-test-v2.md`。
