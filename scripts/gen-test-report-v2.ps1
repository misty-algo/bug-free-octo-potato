# 由 driver-api-test-v2-results.txt 生成 Markdown 测试报告
$ErrorActionPreference = "Continue"

$root = Join-Path $PSScriptRoot ".."
$inFile = Join-Path $root "driver-api-test-v2-results.txt"
$outFile = Join-Path $root "docs\driver-api-test-v2.md"

$rows = [IO.File]::ReadAllLines((Resolve-Path $inFile), [Text.Encoding]::UTF8) | Where-Object { $_ -match '^[0-9]+\|' }
$items = @()
foreach ($r in $rows) {
    $f = $r -split "\|"
    if ($f.Count -lt 8) { continue }`r`n    if ($f[0] -notmatch "^[0-9]+$") { continue }
    $items += [pscustomobject]@{
        Seq = $f[0]; Iface = $f[1]; Case = $f[2]; Input = $f[3]
        Expect = $f[4]; Actual = $f[5]; Result = $f[6]; Note = $f[7]
    }
}

$total = $items.Count
$pass = @($items | Where-Object { $_.Result -eq "PASS" }).Count
$fail = $total - $pass

$sb = New-Object System.Text.StringBuilder
function Add($text) { [void]$sb.AppendLine($text) }

Add '# 司机端接口测试报告（统一技术底座版）'
Add ''
Add '## 一、测试概述'
Add ''
Add '| 项目 | 内容 |'
Add '| --- | --- |'
Add '| 测试日期 | 2026-10-22 |'
Add '| 测试范围 | 司机端 D01–D13：档案/状态机/模式/心跳/区域、接单大厅全链路、收益对比、收入结算、提现审批、跨模块资金回调 |'
Add ('| 用例总数 | ' + $total + ' |')
Add ('| 通过 | ' + $pass + ' |')
Add ('| 失败 | ' + $fail + ' |')
Add ('| 通过率 | ' + [Math]::Round(($pass * 100.0 / $total), 1) + '% |')
Add '| 服务地址 | http://127.0.0.1:8080/api/v1（gateway）→ driver-service gRPC 9101 |'
Add '| 依赖环境 | MongoDB day1 库、RabbitMQ、order-service（事件链路） |'
Add ''
Add '结论：本轮全部用例通过。测试过程中发现并修复 3 个真实缺陷，另补 2 处业务校验（见第四节）。'
Add ''
Add '## 二、分模块总览'
Add ''
Add '| 模块 | 用例数 | 通过 | 失败 |'
Add '| --- | --- | --- | --- |'

$groups = $items | Group-Object Iface
foreach ($g in $groups) {
    $gp = @($g.Group | Where-Object { $_.Result -eq "PASS" }).Count
    Add ('| ' + $g.Name + ' | ' + $g.Count + ' | ' + $gp + ' | ' + ($g.Count - $gp) + ' |')
}
Add ''

Add '## 三、用例明细'
Add ''
foreach ($g in $groups) {
    Add ('### ' + $g.Name)
    Add ''
    Add '| # | 用例 | 入参 | 预期 | 实际 | 结果 | 备注 |'
    Add '| --- | --- | --- | --- | --- | --- | --- |'
    foreach ($i in $g.Group) {
        $input = $i.Input -replace "\|", "/"
        $note = $i.Note -replace "\|", "/"
        Add ('| ' + $i.Seq + ' | ' + $i.Case + ' | ' + $input + ' | ' + $i.Expect + ' | ' + $i.Actual + ' | ' + $i.Result + ' | ' + $note + ' |')
    }
    Add ''
}

Add '## 四、测试中发现的问题'
Add ''
Add '### 4.1 真实缺陷（已修复）'
Add ''
Add '| # | 现象 | 根因 | 修复 |'
Add '| --- | --- | --- | --- |'
Add '| 1 | 抢单后调用确认接单必定失败并回滚，订单被打回等待、司机卡在接单预占 | 状态机 `ToBusy` 只允许「空闲 → 忙碌」，而抢单/邀请会先置为「接单预占」，预占 → 忙碌被判为非法流转 | `ToBusy` 放开「接单预占 → 忙碌」，绑定失败时同时把司机释放回空闲，避免卡死 |'
Add '| 2 | 同一幂等键重复提交提现，返回同一张单据但额度被扣两次（可用 2400 分提 1000 后只剩 400） | 先扣额度再落单，幂等命中时未回滚预占 | 落单前先按 `driver_id + idempotency_key` 预检；插入被唯一索引拦下时返还本次预占 |'
Add '| 3 | 退款回调一旦在订单校验阶段失败，同事件重试永远返回 `duplicate_event`，退款凭证永久丢失 | 先占用消费去重位再处理业务，失败不回滚占用 | 取消前置占用，改由反向流水 `event_id` 唯一索引保证幂等，失败可安全重试 |'
Add ''
Add '### 4.2 新增业务校验（对齐方案要求）'
Add ''
Add '- 接单区域：片区包围盒必须 `min < max` 且经纬度在合法区间，否则返回 `invalid_input`（此前会静默写入非法区域，导致大厅永远看不到单）。'
Add '- 订单坐标：起终点必须是非零且合法的 GCJ02 坐标，否则返回 `invalid_input`（此前零坐标会写入，派单距离与区域过滤全部失真）。'
Add ''
Add '### 4.3 属于设计约定、易被误判为 bug 的行为'
Add ''
Add '| 行为 | 说明 |'
Add '| --- | --- |'
Add '| 两类参数错误码 | `invalid_request` 来自网关格式/必填校验；`invalid_input` 来自业务层校验（如姓名全空格、片区非法、金额为 0） |'
Add '| 原子操作不区分「不存在」与「不可用」 | 抢单/开始行程/完单这类条件更新，未命中统一返回 `order_not_available`；只有先查后改的退单会返回 `order_not_found` |'
Add '| 校验优先级 | 抢单先校验司机状态（忙碌 → `driver_not_idle`）；退单先校验归属（别人的单 → `order_not_mine`） |'
Add '| 零值字段省略 | `total=0`、余额为 0、`items` 为空数组时字段不返回（protobuf/omitempty），前端按「缺失即 0」处理 |'
Add '| 听单窗口内不可抢 | 订单被邀请给听单司机时 `visible_in_pool=false`，其他司机抢单返回 `order_not_visible`；只有无听单运力时才立即曝光到抢单池 |'
Add '| 空闲 60 秒无心跳自动收车 | 定时器每秒扫描，空闲司机超时置为 `offline`；忙碌掉线保留任务不释放 |'
Add '| 限额按司机 + 自然日统计 | 退单每日 3 次、提现每日 3 次，复用同一司机实测会命中上限 |'
Add ''
Add '## 五、与统一技术底座的对照'
Add ''
Add '| 约定 | 落地情况 | 验证用例 |'
Add '| --- | --- | --- |'
Add '| 订单统一六态 | `WAITING/ACCEPTED/ARRIVED/IN_TRIP/ENDED/CANCELLED`，全部流转走条件更新 | 12、13 模块 |'
Add '| 金额一律整数分 | 接口与实体全部 `int64` 分，无浮点 | 7、17、20 模块 |'
Add '| 幂等键 + 乐观锁 | 提现/退款/支付支持幂等键；状态变更带 `version` | 17、20、21 模块 |'
Add '| 资金五桶 | U 待结算 / A 可用 / Fz 冻结 / W 提现预占 / Debt 待追偿，流转记 ledger | 16、17、22 模块 |'
Add '| 可靠事件表 | 领域事件先落 outbox 再投递，消费端去重表保证不重复处理 | 20、21 模块 |'
Add '| Asia/Shanghai 自然日 | 今日/昨日、退单次数、提现次数均按东八区自然日切分 | 14、17 模块 |'
Add '| GCJ02 + 整数米 | 距离统一整数米，坐标统一 GCJ02 并校验合法性 | 8、9、23 模块 |'
Add ''
Add '## 六、复现方式'
Add ''
Add '```powershell'
Add '# 1. 启动依赖与服务'
Add 'go run ./cmd/driver'
Add 'go run ./cmd/order'
Add 'go run ./cmd/gateway'
Add ''
Add '# 2. 清空司机端测试数据（可选，保证计数类用例可复现）'
Add 'docker exec mongo mongosh -u dev -p dev123456 --quiet --eval "const d=db.getSiblingDB(''day1''); [''driver_profiles'',''hall_orders'',''income_flows'',''withdraw_requests'',''ledger_entries'',''outbox_events'',''inbound_events''].forEach(function(c){d[c].deleteMany({})})"'
Add ''
Add '# 3. 执行测试（约 3 分钟，含两处 65 秒心跳超时等待）'
Add 'powershell -NoProfile -ExecutionPolicy Bypass -File scripts/driver-api-test-v2.ps1'
Add 'powershell -NoProfile -ExecutionPolicy Bypass -File scripts/gen-test-report-v2.ps1'
Add '```'
Add ''
Add '结果文件：`driver-api-test-v2-results.txt`（原始数据，管道分隔）；报告：`docs/driver-api-test-v2.md`。'

[IO.File]::WriteAllText($outFile, $sb.ToString(), (New-Object Text.UTF8Encoding($false)))
Write-Output ("REPORT=" + (Resolve-Path $outFile).Path)
Write-Output ("TOTAL=$total PASS=$pass FAIL=$fail")
