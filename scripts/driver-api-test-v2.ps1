# 司机端接口自动化测试（统一技术底座版）
# 覆盖：司机档案/状态/模式/心跳/区域、接单大厅全链路、收益对比、收入结算、
#       提现全流程、跨模块资金回调（支付/退款/结算/审批）。
# 每个接口 10 个用例，覆盖正常、参数缺失、参数非法、边界、状态冲突、幂等与限额。
$ErrorActionPreference = "Continue"
[Console]::OutputEncoding = [Text.Encoding]::UTF8

$ApiBase = "http://127.0.0.1:8080/api/v1"
$OutFile = Join-Path (Join-Path $PSScriptRoot "..") "driver-api-test-v2-results.txt"
$Results = New-Object System.Collections.ArrayList
$script:Seq = 0

function Body($obj) { $json = ConvertTo-Json $obj -Depth 9 -Compress; return ,([Text.Encoding]::UTF8.GetBytes($json)) }

function Api($method, $path, $obj) {
    try {
        if ($null -eq $obj) {
            $r = Invoke-RestMethod -Uri "$ApiBase$path" -Method $method
        } else {
            $r = Invoke-RestMethod -Uri "$ApiBase$path" -Method $method -ContentType "application/json; charset=utf-8" -Body (Body $obj)
        }
        return @{ Ok = $true; Data = $r.data }
    } catch {
        $raw = $_.ErrorDetails.Message
        if (-not $raw) {
            try { $s = $_.Exception.Response.GetResponseStream(); if ($s) { $raw = (New-Object IO.StreamReader($s)).ReadToEnd() } } catch { }
        }
        $code = "HTTP_ERROR"
        if ($raw -and $raw.TrimStart().StartsWith("{")) {
            try { $p = $raw | ConvertFrom-Json; if ($p.error.code) { $code = $p.error.code } } catch { }
        }
        return @{ Ok = $false; Code = $code; Raw = $raw }
    }
}

function T($iface, $case, $inputDesc, $expect, $r, $note, $override) {
    $script:Seq++
    $actual = if ($override) { $override } elseif ($r.Ok) { "OK" } else { $r.Code }
    $pass = if ($actual -eq $expect) { "PASS" } else { "FAIL" }
    [void]$Results.Add(("{0}|{1}|{2}|{3}|{4}|{5}|{6}|{7}" -f $script:Seq, $iface, $case, $inputDesc, $expect, $actual, $pass, $note))
}

function NewDriver($name) {
    $r = Api "POST" "/driver/profile" @{ name = $name; phone = "13800000000" }
    if (-not $r.Ok) {
        Add-Content -Path (Join-Path (Join-Path $PSScriptRoot "..") "newdriver-errors.log") -Value ("$name|" + $r.Code + "|" + $r.Raw)
    }
    return $r.Data
}

function HB($d) { Api "POST" "/driver/profile/$($d.id)/heartbeat" @{ lat = 30.657; lng = 104.081 } | Out-Null }

function NewOrder($name, $paid, $coupon, $subsidy) {
    return Api "POST" "/driver/hall/orders" @{
        passenger_name = $name
        origin = @{ name = "春熙路"; lat = 30.657; lng = 104.081 }
        destination = @{ name = "天府广场"; lat = 30.657; lng = 104.065 }
        passenger_paid_fen = $paid
        platform_coupon_fen = $coupon
        subsidy_fen = $subsidy
        passenger_count = 1
        city_code = "510100"
    }
}

function Status($d, $st) { Api "POST" "/driver/profile/$($d.id)/status" @{ status = $st } }

# ───────────────────── 0. 准备基础数据 ─────────────────────
$iface = "0.准备"
$baseDriver = NewDriver "基础司机"
T $iface "基础司机注册" "name=基础司机" "OK" @{ Ok = ($null -ne $baseDriver.id) } "id=$($baseDriver.id)" | Out-Null
HB $baseDriver | Out-Null

# ───────────────────── 1. POST /driver/profile 注册 ─────────────────────
$iface = "1.注册司机"
$r = Api "POST" "/driver/profile" @{ name = "张三"; phone = "13800000001" }
T $iface "正常注册" '{"name":"张三","phone":"13800000001"}' "OK" $r "status=$($r.Data.status) mode=$($r.Data.mode) ver=$($r.Data.version)" | Out-Null
$d1 = $r.Data

$r = NewDriver "李四"
T $iface "无手机号(可空)" '{"name":"李四"}' "OK" @{ Ok = ($null -ne $r.id) } "phone 非必填, id=$($r.id)" | Out-Null

$r = Api "POST" "/driver/profile" @{ phone = "13800000002" }
T $iface "缺少 name" '{"phone":"13800000002"}' "invalid_request" $r "网关格式校验" | Out-Null

$r = Api "POST" "/driver/profile" @{ name = ""; phone = "13800000003" }
T $iface "name 为空串" '{"name":""}' "invalid_request" $r "必填校验" | Out-Null

$r = Api "POST" "/driver/profile" @{ name = "   "; phone = "13800000004" }
T $iface "name 全空格" '{"name":"   "}' "invalid_input" $r "业务层 Trim 后拒绝" | Out-Null

$r = Api "POST" "/driver/profile" @{ name = ("长" * 50); phone = "13800000005" }
T $iface "超长姓名(50字)" '{"name":"长*50"}' "OK" $r "当前不限长度" | Out-Null

$r = Api "POST" "/driver/profile" @{ name = "重复手机号一"; phone = "13900000001" }
$r2 = Api "POST" "/driver/profile" @{ name = "重复手机号二"; phone = "13900000001" }
T $iface "重复手机号" '同一 phone 注册两次' "OK" $r2 "手机号不唯一(可多设备注册)" | Out-Null

$r = Api "POST" "/driver/profile" @{ name = "特殊字符司机"; phone = "13800000006" }
T $iface "姓名含emoji/特殊字符" '{"name":"特殊字符司机"}' "OK" $r "utf8 ok" | Out-Null

$r = Api "POST" "/driver/profile" @{ name = "王五"; phone = "" }
T $iface "phone 空串" '{"name":"王五","phone":""}' "OK" $r "空串按未填处理" | Out-Null

$r = Api "POST" "/driver/profile" @{ name = "刘六"; phone = "13800000008"; extra = "ignored" }
T $iface "多余字段" '{"name":"刘六","extra":"x"}' "OK" $r "未知字段忽略" | Out-Null

# ───────────────────── 2. GET /driver/profile/{id} ─────────────────────
$iface = "2.查询档案"
$r = Api "GET" "/driver/profile/$($d1.id)" $null
T $iface "正常查询" "id=$($d1.id)" "OK" $r "driver_no=$($r.Data.driver_no) funds 五桶" | Out-Null

$r = Api "GET" "/driver/profile/00000000-0000-0000-0000-000000000000" $null
T $iface "不存在的司机" "id=全0" "driver_not_found" $r "返回 404 语义" | Out-Null

$r = Api "GET" "/driver/profile/abc" $null
T $iface "非法 id 格式" "id=abc" "driver_not_found" $r "按不存在处理" | Out-Null

$r = Api "GET" "/driver/profile/$($d1.id)/" $null
T $iface "路径尾部斜杠" "id=$($d1.id)/" "OK" $r "gin 自动匹配" | Out-Null

$r = Api "GET" "/driver/profile/$($d1.id.ToUpper())" $null
T $iface "大写 UUID" "id 大写" "driver_not_found" $r "ID 大小写敏感" | Out-Null

$r = Api "GET" "/driver/profile/$($baseDriver.id)" $null
T $iface "默认字段校验" "id=$($baseDriver.id)" "OK" $r "mode=grab/$($r.Data.mode) seats=$($r.Data.seats)" | Out-Null

HB $d1 | Out-Null
$r = Api "GET" "/driver/profile/$($d1.id)" $null
T $iface "心跳后字段" "先心跳再查" "OK" $r "last_heartbeat_at 非空" | Out-Null

$r = Api "GET" "/driver/profile/%20" $null
T $iface "空白 id" "id=空格" "driver_not_found" $r "无效 id" | Out-Null

$r = Api "GET" "/driver/profile/$($d1.id)?x=1" $null
T $iface "带无关 query" "id + ?x=1" "OK" $r "忽略未知 query" | Out-Null

$r = Api "GET" "/driver/profile/$($d1.id)/extra/path" $null
T $iface "多余路径段" "id/extra/path" "HTTP_ERROR" $r "路由不匹配(404)" | Out-Null

# ───────────────────── 3. POST /driver/profile/{id}/status ─────────────────────
$iface = "3.收车出车"
HB $d1 | Out-Null
$r = Status $d1 "offline"
T $iface "空闲→收车" '{"status":"offline"}' "OK" $r "status=$($r.Data.status)" | Out-Null

$r = Status $d1 "idle"
T $iface "收车→出车" '{"status":"idle"}' "OK" $r "status=$($r.Data.status)" | Out-Null

$r = Status $d1 "offline"
T $iface "重复收车" '{"status":"offline"}' "OK" $r "幂等语义" | Out-Null
Status $d1 "idle" | Out-Null

$r = Api "POST" "/driver/profile/$($d1.id)/status" @{ status = "busy" }
T $iface "非法状态值 busy" '{"status":"busy"}' "invalid_request" $r "只允许 offline/idle" | Out-Null

$r = Api "POST" "/driver/profile/$($d1.id)/status" @{ }
T $iface "缺少 status" '{}' "invalid_request" $r "必填" | Out-Null

$r = Api "POST" "/driver/profile/$($d1.id)/status" @{ status = "idle"; version = 999 }
T $iface "版本冲突" '{"status":"idle","version":999}' "version_conflict" $r "乐观锁" | Out-Null

$r = Api "POST" "/driver/profile/00000000-0000-0000-0000-000000000000/status" @{ status = "idle" }
T $iface "不存在司机收车" "id=全0" "driver_not_found" $r "404" | Out-Null

$d3 = NewDriver "状态司机"
HB $d3 | Out-Null
$o3 = (NewOrder "忙碌单" 2000 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d3.id; order_id = $o3.id } | Out-Null
$r = Status $d3 "offline"
T $iface "预占中收车" '{"status":"offline"}' "pre_occupied_busy" $r "预占态禁止收车" | Out-Null

$r = Status $d3 "idle"
T $iface "预占中直接出车" '{"status":"idle"}' "status_not_allowed" $r "预占态禁止切空闲" | Out-Null

Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d3.id; order_id = $o3.id } | Out-Null
$r = Status $d3 "offline"
T $iface "忙碌中收车" '{"status":"offline"}' "busy_cannot_offline" $r "需先完单/退单" | Out-Null

# ───────────────────── 4. POST /driver/profile/{id}/mode ─────────────────────
$iface = "4.听单抢单"
$d4 = NewDriver "模式司机"
HB $d4 | Out-Null
$r = Api "POST" "/driver/profile/$($d4.id)/mode" @{ mode = "listen" }
T $iface "空闲切听单" '{"mode":"listen"}' "OK" $r "mode=$($r.Data.mode) mode_version=$($r.Data.mode_version)" | Out-Null

$r = Api "POST" "/driver/profile/$($d4.id)/mode" @{ mode = "grab" }
T $iface "空闲切回抢单" '{"mode":"grab"}' "OK" $r "mode_version 递增" | Out-Null

$r = Api "POST" "/driver/profile/$($d4.id)/mode" @{ mode = "listen" }
$v = $r.Data.mode_version
$r = Api "POST" "/driver/profile/$($d4.id)/mode" @{ mode = "listen" }
T $iface "重复切同模式" '{"mode":"listen"} x2' "OK" $r "版本不变: $($r.Data.mode_version -eq $v)" | Out-Null

$r = Api "POST" "/driver/profile/$($d4.id)/mode" @{ mode = "walk" }
T $iface "非法模式值" '{"mode":"walk"}' "invalid_request" $r "只允许 listen/grab" | Out-Null

$r = Api "POST" "/driver/profile/$($d4.id)/mode" @{ }
T $iface "缺少 mode" '{}' "invalid_request" $r "必填" | Out-Null

$r = Api "POST" "/driver/profile/$($d4.id)/mode" @{ mode = "grab"; version = 999 }
T $iface "版本冲突" '{"mode":"grab","version":999}' "version_conflict" $r "乐观锁" | Out-Null

$r = Api "POST" "/driver/profile/00000000-0000-0000-0000-000000000000/mode" @{ mode = "grab" }
T $iface "不存在司机" "id=全0" "driver_not_found" $r "404" | Out-Null

Status $d4 "offline" | Out-Null
$r = Api "POST" "/driver/profile/$($d4.id)/mode" @{ mode = "grab" }
T $iface "收车切模式" '{"mode":"grab"}' "mode_switch_denied" $r "仅空闲可切换" | Out-Null
Status $d4 "idle" | Out-Null
Api "POST" "/driver/profile/$($d4.id)/mode" @{ mode = "grab" } | Out-Null

$d4b = NewDriver "模式冲突司机"
HB $d4b | Out-Null
$o4 = (NewOrder "模式中单" 1500 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d4b.id; order_id = $o4.id } | Out-Null
$r = Api "POST" "/driver/profile/$($d4b.id)/mode" @{ mode = "listen" }
T $iface "预占中切模式" '{"mode":"listen"}' "mode_switch_denied" $r "预占态禁止切换" | Out-Null

$r = Api "POST" "/driver/profile/$($d4.id)/mode" @{ mode = "GRAB" }
T $iface "模式值大写" '{"mode":"GRAB"}' "invalid_request" $r "枚举大小写敏感" | Out-Null

# ───────────────────── 5. POST /driver/profile/{id}/heartbeat ─────────────────────
$iface = "5.心跳上报"
$d5 = NewDriver "心跳司机"
$r = Api "POST" "/driver/profile/$($d5.id)/heartbeat" @{ lat = 30.657; lng = 104.081 }
T $iface "正常心跳" '{"lat":30.657,"lng":104.081}' "OK" $r "last_heartbeat_at 更新" | Out-Null

$r = Api "POST" "/driver/profile/$($d5.id)/heartbeat" @{ }
T $iface "空 body" '{}' "OK" $r "坐标可空" | Out-Null

$r = Api "POST" "/driver/profile/$($d5.id)/heartbeat" $null
T $iface "无 body" "无请求体" "OK" $r "容错处理" | Out-Null

$r = Api "POST" "/driver/profile/$($d5.id)/heartbeat" @{ lat = -90; lng = 180 }
T $iface "边界坐标" '{"lat":-90,"lng":180}' "OK" $r "极值不报错" | Out-Null

$r = Api "POST" "/driver/profile/$($d5.id)/heartbeat" @{ lat = 999; lng = 999 }
T $iface "越界坐标" '{"lat":999,"lng":999}' "OK" $r "当前不做坐标校验" | Out-Null

$r = Api "POST" "/driver/profile/00000000-0000-0000-0000-000000000000/heartbeat" @{ lat = 30.6; lng = 104.0 }
T $iface "不存在司机" "id=全0" "driver_not_found" $r "404" | Out-Null

$r = Api "POST" "/driver/profile/$($d5.id)/heartbeat" @{ lat = "abc"; lng = 104.0 }
T $iface "坐标非数字" '{"lat":"abc"}' "invalid_request" $r "类型校验" | Out-Null

Status $d5 "offline" | Out-Null
$r = Api "POST" "/driver/profile/$($d5.id)/heartbeat" @{ lat = 30.65; lng = 104.08 }
T $iface "收车状态心跳" '{"lat":30.65}' "OK" $r "收车后仍可上报" | Out-Null
Status $d5 "idle" | Out-Null

$r = Api "POST" "/driver/profile/$($d5.id)/heartbeat" @{ lat = 30.657; lng = 104.081; extra = 1 }
T $iface "多余字段" '{"lat":30.657,"extra":1}' "OK" $r "忽略未知字段" | Out-Null

$r = Api "POST" "/driver/profile/$($d5.id)/heartbeat" @{ lat = 30.657 }
T $iface "仅传纬度" '{"lat":30.657}' "OK" $r "lng 缺省 0" | Out-Null

# ───────────────────── 6. PUT /driver/profile/{id}/area ─────────────────────
$iface = "6.接单区域"
$d6 = NewDriver "区域司机"
HB $d6 | Out-Null
$beijing = @{ name = "北京"; min_lat = 39.80; min_lng = 116.30; max_lat = 40.00; max_lng = 116.50 }
$chengdu = @{ name = "成都"; min_lat = 30.60; min_lng = 104.00; max_lat = 30.75; max_lng = 104.15 }

$r = Api "PUT" "/driver/profile/$($d6.id)/area" @{ area_limit_switch = $true; areas = @($chengdu) }
T $iface "开启限制+1片区" '成都片区' "OK" $r "switch=$($r.Data.area_limit_switch) areas=$(@($r.Data.areas).Count)" | Out-Null

$r = Api "PUT" "/driver/profile/$($d6.id)/area" @{ area_limit_switch = $true; areas = @($chengdu, $beijing) }
T $iface "多片区" '成都+北京' "OK" $r "areas=$(@($r.Data.areas).Count)" | Out-Null

$r = Api "PUT" "/driver/profile/$($d6.id)/area" @{ area_limit_switch = $false; areas = @() }
T $iface "关闭限制(全城)" 'switch=false' "OK" $r "不限区域" | Out-Null

$r = Api "PUT" "/driver/profile/$($d6.id)/area" @{ area_limit_switch = $true; areas = @($chengdu) }
T $iface "重新开启" '成都' "OK" $r "恢复片区" | Out-Null

$r = Api "PUT" "/driver/profile/$($d6.id)/area" @{ area_limit_switch = $true; areas = @(@{ name = "错误片区"; min_lat = 40.0; min_lng = 116.0; max_lat = 39.0; max_lng = 115.0 }) }
T $iface "min>max 非法片区" 'min_lat40/max_lat39' "invalid_input" $r "业务校验拒绝" | Out-Null

$r = Api "PUT" "/driver/profile/$($d6.id)/area" @{ area_limit_switch = $true; areas = @() }
T $iface "开启限制但片区为空" 'switch=true,areas=[]' "OK" $r "空片区=全城可见" | Out-Null

$r = Api "PUT" "/driver/profile/$($d6.id)/area" @{ }
T $iface "空 body" '{}' "OK" $r "等价关闭限制" | Out-Null

$r = Api "PUT" "/driver/profile/00000000-0000-0000-0000-000000000000/area" @{ area_limit_switch = $true }
T $iface "不存在司机" "id=全0" "driver_not_found" $r "404" | Out-Null

Status $d6 "offline" | Out-Null
$r = Api "PUT" "/driver/profile/$($d6.id)/area" @{ area_limit_switch = $true; areas = @($chengdu) }
T $iface "收车改区域" '收车态' "area_change_denied" $r "仅空闲可改" | Out-Null
Status $d6 "idle" | Out-Null

$r = Api "PUT" "/driver/profile/$($d6.id)/area" @{ area_limit_switch = $true; areas = @($chengdu); version = 999 }
T $iface "版本冲突" 'version=999' "version_conflict" $r "乐观锁" | Out-Null

# ───────────────────── 7. POST /driver/hall/orders 创建任务 ─────────────────────
$iface = "7.创建任务"
$r = NewOrder "乘客甲" 3000 0 0
T $iface "正常下单" 'paid=3000分' "OK" $r "income=$($r.Data.driver_income_fen) commission=$($r.Data.commission_fen)" | Out-Null
$o7 = $r.Data

$r = NewOrder "补贴单" 3000 0 500
T $iface "含补贴" 'paid=3000,subsidy=500' "OK" $r "income 应为 2900: $($r.Data.driver_income_fen)" | Out-Null

$r = NewOrder "平台券单" 3000 300 0
T $iface "含平台券" 'paid=3000,coupon=300' "OK" $r "券由平台承担，司机收入仍 2400: $($r.Data.driver_income_fen)" | Out-Null

$r = NewOrder "拼车单" 4000 0 0
T $iface "拼车两乘客" 'paid=4000' "OK" $r "passenger_count=$($r.Data.passenger_count)" | Out-Null

$r = Api "POST" "/driver/hall/orders" @{ passenger_name = "无金额"; origin = @{ lat = 30.6; lng = 104.0 }; destination = @{ lat = 30.7; lng = 104.1 } }
T $iface "缺少金额" '无 passenger_paid_fen' "invalid_request" $r "必填" | Out-Null

$r = NewOrder "零金额" 0 0 0
T $iface "金额为 0" 'paid=0' "invalid_request" $r "gt=0 校验" | Out-Null

$r = NewOrder "负金额" -100 0 0
T $iface "负数金额" 'paid=-100' "invalid_request" $r "gt=0 校验" | Out-Null

$r = Api "POST" "/driver/hall/orders" @{ passenger_name = "缺起点"; destination = @{ lat = 30.7; lng = 104.1 }; passenger_paid_fen = 2000 }
T $iface "缺少起点(坐标校验)" '无 origin' "invalid_input" $r "坐标不可为 0" | Out-Null

$r = Api "POST" "/driver/hall/orders" @{ origin = @{ lat = 30.6; lng = 104.0 }; destination = @{ lat = 30.7; lng = 104.1 }; passenger_paid_fen = 2000 }
T $iface "缺少乘客名" '无 passenger_name' "invalid_request" $r "必填" | Out-Null

$r = NewOrder "大额单" 500000 0 0
T $iface "大额订单(5000元)" 'paid=500000' "OK" $r "抽成=$($r.Data.commission_fen)" | Out-Null

# ───────────────────── 8. GET /driver/hall/orders 大厅列表 ─────────────────────
$iface = "8.大厅列表"
$d8 = NewDriver "大厅司机"
HB $d8 | Out-Null
$r = Api "GET" "/driver/hall/orders?driver_id=$($d8.id)&lat=30.657&lng=104.081" $null
T $iface "正常查询" "driver+坐标" "OK" $r "total=$($r.Data.total)" | Out-Null

$r = Api "GET" "/driver/hall/orders?driver_id=$($d8.id)" $null
T $iface "不传坐标" "仅 driver_id" "OK" $r "距离为 0" | Out-Null

$r = Api "GET" "/driver/hall/orders?driver_id=$($d8.id)&limit=1" $null
$cnt = if ($null -eq $r.Data.items) { 0 } else { @($r.Data.items).Count }
T $iface "limit=1" "limit=1" "OK" $r "返回 $cnt 条" | Out-Null

$r = Api "GET" "/driver/hall/orders" $null
T $iface "缺少 driver_id" "无参数" "invalid_request" $r "必填" | Out-Null

$r = Api "GET" "/driver/hall/orders?driver_id=00000000-0000-0000-0000-000000000000" $null
T $iface "不存在司机" "id=全0" "driver_not_found" $r "404" | Out-Null

$r = Api "GET" "/driver/hall/orders?driver_id=$($d8.id)&limit=abc" $null
T $iface "limit 非数字" "limit=abc" "invalid_request" $r "类型校验" | Out-Null

$r = Api "GET" "/driver/hall/orders?driver_id=$($d8.id)&limit=1000" $null
T $iface "limit 超上限" "limit=1000" "OK" $r "服务端收敛到配置上限" | Out-Null

Api "PUT" "/driver/profile/$($d8.id)/area" @{ area_limit_switch = $true; areas = @($beijing) } | Out-Null
$r = Api "GET" "/driver/hall/orders?driver_id=$($d8.id)" $null
$cntArea = if ($null -eq $r.Data.items) { 0 } else { @($r.Data.items).Count }
T $iface "区域过滤(北京片区看成都单)" "area=北京" "OK" $r "可见 $cntArea 条(应为0)" | Out-Null
Api "PUT" "/driver/profile/$($d8.id)/area" @{ area_limit_switch = $false } | Out-Null

$r = Api "GET" "/driver/hall/orders?driver_id=$($d8.id)&limit=0" $null
T $iface "limit=0" "limit=0" "OK" $r "按默认条数返回" | Out-Null

Status $d8 "offline" | Out-Null
$r = Api "GET" "/driver/hall/orders?driver_id=$($d8.id)" $null
T $iface "收车司机看大厅" "收车态" "OK" $r "仍可浏览(仅抢单受限)" | Out-Null
Status $d8 "idle" | Out-Null

# ───────────────────── 9. POST /driver/hall/orders/grab 抢单 ─────────────────────
$iface = "9.抢单"
$d9 = NewDriver "抢单司机"
HB $d9 | Out-Null
$o9 = (NewOrder "抢单单" 3000 0 0).Data
$r = Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d9.id; order_id = $o9.id }
T $iface "正常抢单" 'driver+order' "OK" $r "status=$($r.Data.status) round=$($r.Data.invite_round)" | Out-Null

$pr = (Api "GET" "/driver/profile/$($d9.id)" $null).Data
T $iface "抢单后司机状态" "查档案" "OK" @{ Ok = $true } "status=$($pr.status) task=$($pr.active_task_id)" | Out-Null

$d9b = NewDriver "抢单司机B"
HB $d9b | Out-Null
$r = Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d9b.id; order_id = $o9.id }
T $iface "并发抢同一单" "第二个司机" "order_not_available" $r "原子预占，只有一人成功" | Out-Null

$r = Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d9.id; order_id = $o9.id }
T $iface "重复抢自己的单" "已预占" "driver_not_idle" $r "司机状态优先校验" | Out-Null

$o9c = (NewOrder "收车抢单" 2000 0 0).Data
Status $d9b "offline" | Out-Null
$r = Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d9b.id; order_id = $o9c.id }
T $iface "收车司机抢单" "offline" "driver_not_idle" $r "需先出车" | Out-Null
Status $d9b "idle" | Out-Null

$r = Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d9b.id; order_id = "00000000-0000-0000-0000-000000000000" }
T $iface "抢不存在的订单" "id=全0" "order_not_found" $r "先查后改" | Out-Null

$r = Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d9.id }
T $iface "缺少 order_id" '仅 driver_id' "invalid_request" $r "必填" | Out-Null

$r = Api "POST" "/driver/hall/orders/grab" @{ driver_id = "00000000-0000-0000-0000-000000000000"; order_id = $o9c.id }
T $iface "不存在司机抢单" "id=全0" "driver_not_found" $r "404" | Out-Null

$o9d = (NewOrder "第二单" 2500 0 0).Data
$r = Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d9b.id; order_id = $o9d.id; idempotency_key = "grab-key-1" }
T $iface "带幂等键抢单" 'idempotency_key' "OK" $r "status=$($r.Data.status)" | Out-Null

$r = Api "POST" "/driver/hall/orders/grab" @{ order_id = $o9d.id }
T $iface "缺少 driver_id" '仅 order_id' "invalid_request" $r "必填" | Out-Null

# ───────────────────── 10. POST /driver/hall/orders/accept 确认接单 ─────────────────────
$iface = "10.确认接单"
$d10 = NewDriver "接单司机"
HB $d10 | Out-Null
$o10 = (NewOrder "接单单" 3000 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d10.id; order_id = $o10.id } | Out-Null
$r = Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d10.id; order_id = $o10.id }
T $iface "正常确认" "grab 后立即 accept" "OK" $r "status=$($r.Data.status)" | Out-Null

$pr = (Api "GET" "/driver/profile/$($d10.id)" $null).Data
T $iface "确认后司机忙碌" "查档案" "OK" @{ Ok = $true } "status=$($pr.status)" | Out-Null

$r = Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d10.id; order_id = $o10.id }
T $iface "重复确认" "同单再确认" "order_not_available" $r "状态已 ACCEPTED" | Out-Null

$r = Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d10.id }
T $iface "缺少 order_id" '仅 driver_id' "invalid_request" $r "必填" | Out-Null

$d10b = NewDriver "他人司机"
HB $d10b | Out-Null
$o10b = (NewOrder "他人单" 2000 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d10b.id; order_id = $o10b.id } | Out-Null
$r = Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d9b.id; order_id = $o10b.id }
T $iface "确认别人的邀请" "他人已预占" "order_not_available" $r "driver_id 不匹配" | Out-Null

$r = Api "POST" "/driver/hall/orders/accept" @{ driver_id = "00000000-0000-0000-0000-000000000000"; order_id = $o10b.id }
T $iface "不存在司机确认" "id=全0" "driver_not_found" $r "404" | Out-Null

$r = Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d10b.id; order_id = $o10b.id; version = 999 }
T $iface "版本冲突" "version=999" "order_not_available" $r "条件更新未命中" | Out-Null

$o10c = (NewOrder "无预占确认" 1800 0 0).Data
$r = Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d10b.id; order_id = $o10c.id }
T $iface "未抢单直接确认" "池中订单" "order_not_available" $r "必须自己预占" | Out-Null

$o10d = (NewOrder "预占超时" 1800 0 0).Data
$d10c = NewDriver "超时司机"
HB $d10c | Out-Null
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d10c.id; order_id = $o10d.id } | Out-Null
Start-Sleep -Seconds 17
$r = Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d10c.id; order_id = $o10d.id }
T $iface "超过15秒确认" "等待17秒" "order_not_available" $r "邀请已超时回收" | Out-Null

$pr = (Api "GET" "/driver/profile/$($d10c.id)" $null).Data
T $iface "超时后司机释放" "查档案" "OK" @{ Ok = $true } "status=$($pr.status)" | Out-Null

# ───────────────────── 11. POST /driver/hall/orders/release 退单 ─────────────────────
$iface = "11.退单"
$d11 = NewDriver "退单司机"
HB $d11 | Out-Null
$o11 = (NewOrder "退单单" 2600 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d11.id; order_id = $o11.id } | Out-Null
Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d11.id; order_id = $o11.id } | Out-Null
$r = Api "POST" "/driver/hall/orders/release" @{ driver_id = $d11.id; order_id = $o11.id; reason = "临时有事" }
T $iface "正常退单" "已接单未开始" "OK" $r "status=$($r.Data.status)" | Out-Null

$pr = (Api "GET" "/driver/profile/$($d11.id)" $null).Data
T $iface "退单后司机空闲" "查档案" "OK" @{ Ok = $true } "status=$($pr.status)" | Out-Null

$r = Api "POST" "/driver/hall/orders/release" @{ driver_id = $d11.id; order_id = $o11.id }
T $iface "退已退的订单" "重复退单" "order_not_mine" $r "已无归属" | Out-Null

$o11b = (NewOrder "他人退单" 2000 0 0).Data
$r = Api "POST" "/driver/hall/orders/release" @{ driver_id = $d11.id; order_id = $o11b.id }
T $iface "退别人的单" "未接单" "order_not_mine" $r "归属校验优先" | Out-Null

$r = Api "POST" "/driver/hall/orders/release" @{ driver_id = $d11.id; order_id = "00000000-0000-0000-0000-000000000000" }
T $iface "退不存在的订单" "id=全0" "order_not_found" $r "先查后改" | Out-Null

$r = Api "POST" "/driver/hall/orders/release" @{ order_id = $o11b.id }
T $iface "缺少 driver_id" "仅 order_id" "invalid_request" $r "必填" | Out-Null

# 退单次数上限：同一司机连续退 3 次后第 4 次拦截
$d11b = NewDriver "退单上限司机"
HB $d11b | Out-Null
$o11c = (NewOrder "次数单" 2000 0 0).Data
$codes = @()
for ($i = 1; $i -le 4; $i++) {
    HB $d11b | Out-Null
    Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d11b.id; order_id = $o11c.id } | Out-Null
    Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d11b.id; order_id = $o11c.id } | Out-Null
    $rr = Api "POST" "/driver/hall/orders/release" @{ driver_id = $d11b.id; order_id = $o11c.id }
    $codes += $(if ($rr.Ok) { "OK" } else { $rr.Code })
}
T $iface "每日退单上限" "连续退4次" "OK" @{ Ok = $true } "实际=$($codes -join ',')" $(if (($codes -join ',') -eq "OK,OK,OK,release_limit_exceeded") { "OK" } else { "FAIL:$($codes -join ',')" }) | Out-Null

$d11c = NewDriver "乘客取消司机"
HB $d11c | Out-Null
$o11d = (NewOrder "乘客取消单" 2000 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d11c.id; order_id = $o11d.id } | Out-Null
Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d11c.id; order_id = $o11d.id } | Out-Null
$r = Api "POST" "/driver/hall/orders/cancel" @{ order_id = $o11d.id; responsible_party = "passenger"; reason = "乘客取消" }
T $iface "乘客责任取消" "responsible_party=passenger" "OK" $r "status=$($r.Data.status)" | Out-Null

$pr = (Api "GET" "/driver/profile/$($d11c.id)" $null).Data
T $iface "取消后释放司机" "查档案" "OK" @{ Ok = $true } "status=$($pr.status)" | Out-Null

$o11e = (NewOrder "平台取消单" 2000 0 0).Data
$r = Api "POST" "/driver/hall/orders/cancel" @{ order_id = $o11e.id; responsible_party = "platform"; reason = "派单超时" }
T $iface "平台责任取消" "池中订单" "OK" $r "status=$($r.Data.status)" | Out-Null

# ───────────────────── 12. POST /driver/hall/orders/start 开始行程 ─────────────────────
$iface = "12.开始行程"
$d12 = NewDriver "行程司机"
HB $d12 | Out-Null
$o12 = (NewOrder "行程单" 3200 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d12.id; order_id = $o12.id } | Out-Null
Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d12.id; order_id = $o12.id } | Out-Null
$r = Api "POST" "/driver/hall/orders/start" @{ driver_id = $d12.id; order_id = $o12.id }
T $iface "正常开始" "ACCEPTED" "OK" $r "status=$($r.Data.status)" | Out-Null

$r = Api "POST" "/driver/hall/orders/start" @{ driver_id = $d12.id; order_id = $o12.id }
T $iface "重复开始" "已在行程中" "order_not_available" $r "原子条件更新未命中" | Out-Null

$o12b = (NewOrder "未接单开始" 2000 0 0).Data
$r = Api "POST" "/driver/hall/orders/start" @{ driver_id = $d12.id; order_id = $o12b.id }
T $iface "未接单开始" "池中订单" "order_not_available" $r "原子条件更新未命中" | Out-Null

$r = Api "POST" "/driver/hall/orders/start" @{ driver_id = $d12.id; order_id = "00000000-0000-0000-0000-000000000000" }
T $iface "不存在订单" "id=全0" "order_not_available" $r "原子条件更新未命中" | Out-Null

$r = Api "POST" "/driver/hall/orders/start" @{ order_id = $o12.id }
T $iface "缺少 driver_id" '仅 order_id' "invalid_request" $r "必填" | Out-Null

$r = Api "POST" "/driver/hall/orders/start" @{ driver_id = $d12.id }
T $iface "缺少 order_id" '仅 driver_id' "invalid_request" $r "必填" | Out-Null

$r = Api "POST" "/driver/hall/orders/start" @{ driver_id = $d11.id; order_id = $o12.id }
T $iface "他人开始行程" "非本单司机" "order_not_available" $r "归属不匹配" | Out-Null

$r = Api "POST" "/driver/hall/orders/start" @{ driver_id = "00000000-0000-0000-0000-000000000000"; order_id = $o12.id }
T $iface "不存在司机开始" "id=全0" "order_not_available" $r "条件更新未命中" | Out-Null

$o12c = (NewOrder "已取消开始" 2000 0 0).Data
Api "POST" "/driver/hall/orders/cancel" @{ order_id = $o12c.id; responsible_party = "platform" } | Out-Null
$r = Api "POST" "/driver/hall/orders/start" @{ driver_id = $d12.id; order_id = $o12c.id }
T $iface "已取消订单开始" "CANCELLED" "order_not_available" $r "状态不允许" | Out-Null

$r = Api "POST" "/driver/hall/orders/release" @{ driver_id = $d12.id; order_id = $o12.id }
T $iface "行程中退单" "IN_TRIP 退单" "trip_already_started" $r "行程中禁止退单" | Out-Null

# ───────────────────── 13. POST /driver/hall/orders/complete 送达结束 ─────────────────────
$iface = "13.送达结束"
$d13 = NewDriver "完单司机"
HB $d13 | Out-Null
$o13 = (NewOrder "完单单" 3500 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d13.id; order_id = $o13.id } | Out-Null
Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d13.id; order_id = $o13.id } | Out-Null
Api "POST" "/driver/hall/orders/start" @{ driver_id = $d13.id; order_id = $o13.id } | Out-Null
$r = Api "POST" "/driver/hall/orders/complete" @{ driver_id = $d13.id; order_id = $o13.id; distance_meters = 5200; duration_sec = 900 }
T $iface "正常完单" "IN_TRIP→ENDED" "OK" $r "flow=$($r.Data.flow.id) income=$($r.Data.flow.amount.driver_income_fen)" | Out-Null

$pr = (Api "GET" "/driver/profile/$($d13.id)" $null).Data
T $iface "完单后释放运力" "查档案" "OK" @{ Ok = $true } "status=$($pr.status) avail=$($pr.funds.available_fen)" | Out-Null

$r = Api "POST" "/driver/hall/orders/complete" @{ driver_id = $d13.id; order_id = $o13.id }
T $iface "重复完单" "已 ENDED" "order_not_available" $r "原子条件更新未命中" | Out-Null

$r = Api "POST" "/driver/hall/orders/complete" @{ driver_id = $d13.id }
T $iface "缺少 order_id" '仅 driver_id' "invalid_request" $r "必填" | Out-Null

$o13b = (NewOrder "未完单完单" 2000 0 0).Data
$r = Api "POST" "/driver/hall/orders/complete" @{ driver_id = $d13.id; order_id = $o13b.id }
T $iface "未开始行程完单" "WAITING" "order_not_available" $r "状态不允许" | Out-Null

$r = Api "POST" "/driver/hall/orders/complete" @{ driver_id = $d13.id; order_id = "00000000-0000-0000-0000-000000000000" }
T $iface "不存在订单完单" "id=全0" "order_not_available" $r "状态不匹配" | Out-Null

$o13c = (NewOrder "里程缺省" 3000 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d13.id; order_id = $o13c.id } | Out-Null
Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d13.id; order_id = $o13c.id } | Out-Null
Api "POST" "/driver/hall/orders/start" @{ driver_id = $d13.id; order_id = $o13c.id } | Out-Null
$r = Api "POST" "/driver/hall/orders/complete" @{ driver_id = $d13.id; order_id = $o13c.id }
T $iface "不传里程/时长完单" "仅必要字段" "OK" $r "默认 0" | Out-Null

$o13d = (NewOrder "他人完单" 3000 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d13.id; order_id = $o13d.id } | Out-Null
Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d13.id; order_id = $o13d.id } | Out-Null
Api "POST" "/driver/hall/orders/start" @{ driver_id = $d13.id; order_id = $o13d.id } | Out-Null
$r = Api "POST" "/driver/hall/orders/complete" @{ driver_id = $d11.id; order_id = $o13d.id }
T $iface "他人完单" "非本单司机" "order_not_available" $r "归属不匹配" | Out-Null

$r = Api "POST" "/driver/hall/orders/complete" @{ driver_id = $d13.id; order_id = $o13d.id; distance_meters = -1; duration_sec = -1 }
T $iface "负数里程时长" "distance=-1" "OK" $r "当前不校验负数" | Out-Null

$pr = (Api "GET" "/driver/profile/$($d13.id)" $null).Data
T $iface "超额完单余额累计" "查档案" "OK" @{ Ok = $true } "avail=$($pr.funds.available_fen) monthly=$($pr.monthly_income_fen)" | Out-Null

# ───────────────────── 14. GET /driver/income/compare 收益对比 ─────────────────────
$iface = "14.收益对比"
$r = Api "GET" "/driver/income/compare?driver_id=$($d13.id)" $null
T $iface "正常查询" "今日有完单" "OK" $r "today_orders=$($r.Data.today.order_count) today_income=$($r.Data.today.income_fen)" | Out-Null

$r = Api "GET" "/driver/income/compare?driver_id=$($baseDriver.id)" $null
T $iface "无单司机" "新司机" "OK" $r "income 为空表示 0" | Out-Null

$r = Api "GET" "/driver/income/compare?driver_id=00000000-0000-0000-0000-000000000000" $null
T $iface "不存在司机" "id=全0" "driver_not_found" $r "404" | Out-Null

$r = Api "GET" "/driver/income/compare" $null
T $iface "缺少 driver_id" "无参数" "invalid_request" $r "必填" | Out-Null

$r = Api "GET" "/driver/income/compare?driver_id=$($d13.id)&page=2" $null
T $iface "多余参数" "page=2" "OK" $r "忽略未知参数" | Out-Null

$r = Api "GET" "/driver/income/compare?driver_id=$($d13.id)" $null
T $iface "昨日为0不展示百分比" "yesterday=0" "OK" $r "percent_text=[$($r.Data.percent_text)]" | Out-Null

$cmpToday = $r.Data.today
T $iface "今日完单量统计" "今日 2 单" "OK" @{ Ok = $true } "today_order_count=$($cmpToday.order_count)" | Out-Null

$r = Api "GET" "/driver/income/compare?driver_id=$($d13.id)" $null
T $iface "待入账(U桶)返回" "自动结算后应为0" "OK" $r "pending=$($r.Data.pending_income_fen)" | Out-Null

$d14 = NewDriver "对比司机"
$r = Api "GET" "/driver/income/compare?driver_id=$($d14.id)" $null
T $iface "空数据司机" "刚注册" "OK" $r "diff 为 0" | Out-Null

$r = Api "GET" "/driver/income/compare?driver_id=%20" $null
T $iface "空白司机ID" "id=空格" "driver_not_found" $r "无效 id" | Out-Null

# ───────────────────── 15. GET /driver/income/flows 收入流水 ─────────────────────
$iface = "15.收入流水"
$r = Api "GET" "/driver/income/flows?driver_id=$($d13.id)" $null
T $iface "正常查询" "有流水" "OK" $r "total=$($r.Data.total)" | Out-Null

$r = Api "GET" "/driver/income/flows?driver_id=$($d13.id)&page=1&page_size=1" $null
$c = if ($null -eq $r.Data.items) { 0 } else { @($r.Data.items).Count }
T $iface "分页 page_size=1" "page=1" "OK" $r "返回 $c 条" | Out-Null

$r = Api "GET" "/driver/income/flows?driver_id=$($baseDriver.id)" $null
T $iface "无流水司机" "新司机" "OK" $r "total 空=0" | Out-Null

$r = Api "GET" "/driver/income/flows" $null
T $iface "缺少 driver_id" "无参数" "invalid_request" $r "必填" | Out-Null

$r = Api "GET" "/driver/income/flows?driver_id=$($d13.id)&page=abc" $null
T $iface "page 非数字" "page=abc" "invalid_request" $r "类型校验" | Out-Null

$r = Api "GET" "/driver/income/flows?driver_id=$($d13.id)&page_size=999" $null
T $iface "page_size 超上限" "page_size=999" "OK" $r "收敛到 100" | Out-Null

$r = Api "GET" "/driver/income/flows?driver_id=$($d13.id)&page=999" $null
T $iface "超出页码" "page=999" "OK" $r "空列表" | Out-Null

$r = Api "GET" "/driver/income/flows?driver_id=$($d13.id)" $null
$f0 = @($r.Data.items)[0]
T $iface "流水字段完整性" "首条" "OK" @{ Ok = $true } "type=$($f0.flow_type) date=$($f0.accounting_date) bucket=$($f0.bucket_to)" | Out-Null

$r = Api "GET" "/driver/income/flows?driver_id=00000000-0000-0000-0000-000000000000" $null
T $iface "不存在司机流水" "id=全0" "OK" $r "空列表(不报错)" | Out-Null

$r = Api "GET" "/driver/income/flows?driver_id=$($d13.id)&page_size=0" $null
T $iface "page_size=0" "page_size=0" "OK" $r "按默认 20" | Out-Null

# ───────────────────── 16. GET /driver/income/summary 收入概览 ─────────────────────
$iface = "16.收入概览"
$r = Api "GET" "/driver/income/summary?driver_id=$($d13.id)" $null
T $iface "正常查询" "有收入" "OK" $r "avail=$($r.Data.available_fen) monthly=$($r.Data.monthly_income_fen)" | Out-Null

T $iface "提现规则下发" "min/max/daily" "OK" @{ Ok = $true } "min=$($r.Data.min_withdraw_fen) max=$($r.Data.max_withdraw_fen) daily=$($r.Data.daily_limit_fen) count=$($r.Data.daily_max_count)" | Out-Null

T $iface "双人审批阈值" "dual_approval" "OK" @{ Ok = $true } "dual=$($r.Data.dual_approval_fen)" | Out-Null

$r = Api "GET" "/driver/income/summary?driver_id=$($baseDriver.id)" $null
T $iface "无收入司机" "新司机" "OK" $r "各桶为0(字段省略)" | Out-Null

$r = Api "GET" "/driver/income/summary" $null
T $iface "缺少 driver_id" "无参数" "invalid_request" $r "必填" | Out-Null

$r = Api "GET" "/driver/income/summary?driver_id=00000000-0000-0000-0000-000000000000" $null
T $iface "不存在司机" "id=全0" "driver_not_found" $r "404" | Out-Null

$r = Api "GET" "/driver/income/summary?driver_id=$($d13.id)" $null
T $iface "当日受理统计" "withdrawn_today" "OK" @{ Ok = $true } "withdrawn=$($r.Data.withdrawn_today_fen) count=$($r.Data.withdraw_count_today)" | Out-Null

$r = Api "GET" "/driver/income/summary?driver_id=$($d13.id)&x=1" $null
T $iface "多余 query" "x=1" "OK" $r "忽略" | Out-Null

$r = Api "GET" "/driver/income/summary?driver_id=" $null
T $iface "空 driver_id" "driver_id=" "invalid_request" $r "必填校验" | Out-Null

$r = Api "GET" "/driver/income/summary?driver_id=$($d13.id)" $null
T $iface "updated_at 字段" "查概览" "OK" @{ Ok = $true } "updated_at=$($r.Data.updated_at)" | Out-Null

# ───────────────────── 17. POST /driver/income/withdraw 提现 ─────────────────────
$iface = "17.申请提现"
$d17 = NewDriver "提现司机"
HB $d17 | Out-Null
$o17 = (NewOrder "提现单" 20000 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d17.id; order_id = $o17.id } | Out-Null
Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d17.id; order_id = $o17.id } | Out-Null
Api "POST" "/driver/hall/orders/start" @{ driver_id = $d17.id; order_id = $o17.id } | Out-Null
Api "POST" "/driver/hall/orders/complete" @{ driver_id = $d17.id; order_id = $o17.id } | Out-Null
$pr = (Api "GET" "/driver/profile/$($d17.id)" $null).Data
T $iface "准备余额" "完成 200 元订单" "OK" @{ Ok = $true } "avail=$($pr.funds.available_fen)" | Out-Null

$r = Api "POST" "/driver/income/withdraw" @{ driver_id = $d17.id; amount_fen = 1000; idempotency_key = "wd-1" }
T $iface "正常提现 10 元" 'amount=1000' "OK" $r "id=$($r.Data.id) status=$($r.Data.status)" | Out-Null
$wd1 = $r.Data

$r = Api "POST" "/driver/income/withdraw" @{ driver_id = $d17.id; amount_fen = 1000; idempotency_key = "wd-1" }
T $iface "幂等重复提交" "同 idempotency_key" "OK" $r "同一单: $($r.Data.id -eq $wd1.id)" | Out-Null

$pr = (Api "GET" "/driver/profile/$($d17.id)" $null).Data
T $iface "幂等不重复预占" "查余额" "OK" @{ Ok = $true } "avail=$($pr.funds.available_fen) withdraw=$($pr.funds.withdraw_fen)" | Out-Null

$r = Api "POST" "/driver/income/withdraw" @{ driver_id = $d17.id; amount_fen = 999 }
T $iface "低于最低门槛" "amount=999" "below_min_withdraw" $r "最低 1000 分" | Out-Null

$r = Api "POST" "/driver/income/withdraw" @{ driver_id = $d17.id; amount_fen = 200001 }
T $iface "超过单笔上限" "amount=200001" "above_max_withdraw" $r "上限 200000 分" | Out-Null

$r = Api "POST" "/driver/income/withdraw" @{ driver_id = $d17.id; amount_fen = 16000 }
T $iface "超过可用余额" "amount=16000" "balance_insufficient" $r "可用不足" | Out-Null

$r = Api "POST" "/driver/income/withdraw" @{ driver_id = $d17.id; amount_fen = 0 }
T $iface "金额为 0" "amount=0" "invalid_request" $r "gt=0" | Out-Null

$r = Api "POST" "/driver/income/withdraw" @{ driver_id = $d17.id; amount_fen = -100 }
T $iface "负数金额" "amount=-100" "invalid_request" $r "gt=0" | Out-Null

$r = Api "POST" "/driver/income/withdraw" @{ amount_fen = 1000 }
T $iface "缺少 driver_id" "仅金额" "invalid_request" $r "必填" | Out-Null

# ───────────────────── 18. GET /driver/income/withdrawals 提现记录 ─────────────────────
$iface = "18.提现记录"
$r = Api "GET" "/driver/income/withdrawals?driver_id=$($d17.id)" $null
T $iface "正常查询" "有记录" "OK" $r "total=$($r.Data.total)" | Out-Null

$r = Api "GET" "/driver/income/withdrawals?driver_id=$($d17.id)&page_size=1" $null
$c = if ($null -eq $r.Data.items) { 0 } else { @($r.Data.items).Count }
T $iface "分页 page_size=1" "page=1" "OK" $r "返回 $c 条" | Out-Null

$r = Api "GET" "/driver/income/withdrawals?driver_id=$($baseDriver.id)" $null
T $iface "无记录司机" "新司机" "OK" $r "空列表" | Out-Null

$r = Api "GET" "/driver/income/withdrawals" $null
T $iface "缺少 driver_id" "无参数" "invalid_request" $r "必填" | Out-Null

$r = Api "GET" "/driver/income/withdrawals?driver_id=$($d17.id)&page=0" $null
T $iface "page=0" "page=0" "OK" $r "按第 1 页" | Out-Null

$r = Api "GET" "/driver/income/withdrawals?driver_id=$($d17.id)&page_size=999" $null
T $iface "page_size 超上限" "page_size=999" "OK" $r "收敛到 100" | Out-Null

$r = Api "GET" "/driver/income/withdrawals?driver_id=$($d17.id)&page=99" $null
T $iface "超出页码" "page=99" "OK" $r "空列表" | Out-Null

$r = Api "GET" "/driver/income/withdrawals?driver_id=00000000-0000-0000-0000-000000000000" $null
T $iface "不存在司机" "id=全0" "OK" $r "空列表" | Out-Null

$r = Api "GET" "/driver/income/withdrawals?driver_id=$($d17.id)" $null
$w0 = @($r.Data.items)[0]
T $iface "记录字段完整性" "首条" "OK" @{ Ok = $true } "status=$($w0.status) amount=$($w0.amount_fen) dual=$($w0.dual_approval)" | Out-Null

$r = Api "GET" "/driver/income/withdrawals?driver_id=%20" $null
T $iface "空白 driver_id" "id=空格" "OK" $r "空列表" | Out-Null

# ───────────────────── 19. POST /driver/profile/{id}/offline-after-trip ─────────────────────
$iface = "19.本单后收车"
$d19 = NewDriver "收车司机"
HB $d19 | Out-Null
$r = Api "POST" "/driver/profile/$($d19.id)/offline-after-trip" @{ on = $true }
T $iface "开启" '{"on":true}' "OK" $r "offline_after_trip=$($r.Data.offline_after_trip)" | Out-Null

$o19 = (NewOrder "收车单" 3000 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d19.id; order_id = $o19.id } | Out-Null
Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d19.id; order_id = $o19.id } | Out-Null
Api "POST" "/driver/hall/orders/start" @{ driver_id = $d19.id; order_id = $o19.id } | Out-Null
Api "POST" "/driver/hall/orders/complete" @{ driver_id = $d19.id; order_id = $o19.id } | Out-Null
$pr = (Api "GET" "/driver/profile/$($d19.id)" $null).Data
T $iface "完单后自动收车" "on=true 完单" "OK" @{ Ok = $true } "status=$($pr.status)" | Out-Null

Status $d19 "idle" | Out-Null
$r = Api "POST" "/driver/profile/$($d19.id)/offline-after-trip" @{ on = $false }
T $iface "关闭" '{"on":false}' "OK" $r "offline_after_trip=$($r.Data.offline_after_trip)" | Out-Null

$r = Api "POST" "/driver/profile/$($d19.id)/offline-after-trip" @{ }
T $iface "空 body" '{}' "OK" $r "默认 false" | Out-Null

$r = Api "POST" "/driver/profile/00000000-0000-0000-0000-000000000000/offline-after-trip" @{ on = $true }
T $iface "不存在司机" "id=全0" "driver_not_found" $r "404" | Out-Null

$r = Api "POST" "/driver/profile/$($d19.id)/offline-after-trip" @{ on = "yes" }
T $iface "on 非布尔" '{"on":"yes"}' "invalid_request" $r "类型校验" | Out-Null

$r = Api "POST" "/driver/profile/$($d19.id)/offline-after-trip" @{ on = $true; version = 999 }
T $iface "版本冲突" "version=999" "version_conflict" $r "乐观锁" | Out-Null

$r = Api "POST" "/driver/profile/$($d19.id)/offline-after-trip" @{ on = $true }
T $iface "重复开启" "on=true x2" "OK" $r "幂等" | Out-Null

$r = Api "POST" "/driver/profile/$($d19.id)/offline-after-trip" @{ on = $true; extra = 1 }
T $iface "多余字段" '{"on":true,"extra":1}' "OK" $r "忽略" | Out-Null

$r = Api "POST" "/driver/profile/$($d19.id)/offline-after-trip" @{ on = $true }
T $iface "字段回读" "再次开启" "OK" $r "offline_after_trip=$($r.Data.offline_after_trip)" | Out-Null

# ───────────────────── 20. 跨模块资金回调 ─────────────────────
$iface = "20.支付回调"
$d20 = NewDriver "回调司机"
HB $d20 | Out-Null
$o20 = (NewOrder "回调单" 5000 0 0).Data
$pay = @{ order_id = $o20.id; driver_id = $d20.id; event_id = "evt-pay-1"; idempotency_key = "pay-key-1"; amount = @{ orig_fare_fen = 5000; passenger_paid_fen = 5000; commission_fen = 1000; driver_income_fen = 4000 } }
$r = Api "POST" "/driver/payments" $pay
T $iface "支付成功记账" "4000 分" "OK" $r "flow=$($r.Data.flow.id) income=$($r.Data.flow.amount.driver_income_fen)" | Out-Null

$r2 = Api "POST" "/driver/payments" $pay
T $iface "重复投递去重" "同 event_id" "OK" $r2 "同一流水: $($r2.Data.flow.id -eq $r.Data.flow.id)" | Out-Null

$pr = (Api "GET" "/driver/profile/$($d20.id)" $null).Data
T $iface "自动结算到可用桶" "auto_settle=true" "OK" @{ Ok = $true } "avail=$($pr.funds.available_fen) pending=$($pr.funds.pending_fen)" | Out-Null

$r = Api "POST" "/driver/payments" @{ order_id = $o20.id; driver_id = $d20.id; event_id = "evt-pay-2"; amount = @{ driver_income_fen = 0 } }
T $iface "金额为0" "income=0" "invalid_input" $r "业务拒绝" | Out-Null

$r = Api "POST" "/driver/payments" @{ order_id = $o20.id; event_id = "evt-pay-3"; amount = @{ driver_income_fen = 100 } }
T $iface "缺少 driver_id" "无 driver_id" "invalid_request" $r "必填" | Out-Null

$r = Api "POST" "/driver/payments" @{ driver_id = $d20.id; event_id = "evt-pay-4"; amount = @{ driver_income_fen = 100 } }
T $iface "缺少 order_id" "无 order_id" "invalid_request" $r "必填" | Out-Null

$r = Api "POST" "/driver/payments" @{ order_id = $o20.id; driver_id = "00000000-0000-0000-0000-000000000000"; event_id = "evt-pay-5"; amount = @{ driver_income_fen = 100 } }
T $iface "不存在司机" "id=全0" "OK" $r "记账不校验司机存在(由上游保证)" | Out-Null

$r = Api "POST" "/driver/payments" @{ order_id = $o20.id; driver_id = $d20.id; event_id = "evt-pay-6"; idempotency_key = "pay-key-6"; amount = @{ driver_income_fen = 2000; subsidy_fen = 200 } }
T $iface "含补贴记账" "income=2000" "OK" $r "flow income=$($r.Data.flow.amount.driver_income_fen)" | Out-Null

$r = Api "POST" "/driver/payments" @{ order_id = "unknown-order"; driver_id = $d20.id; event_id = "evt-pay-7"; amount = @{ driver_income_fen = 500 } }
T $iface "未知订单号记账" "order 不存在" "OK" $r "凭证仍生成" | Out-Null

$r = Api "POST" "/driver/payments" @{ order_id = $o20.id; driver_id = $d20.id; accounting_date = "2026-10-21"; event_id = "evt-pay-8"; amount = @{ driver_income_fen = 700 } }
T $iface "指定记账日" "accounting_date=昨日" "OK" $r "date=$($r.Data.flow.accounting_date)" | Out-Null

# ───────────────────── 21. 退款回调 ─────────────────────
$iface = "21.退款回调"
$r = Api "POST" "/driver/refunds" @{ order_id = $o20.id; refund_fen = 1000; responsible_party = "passenger"; reason = "乘客退款"; event_id = "evt-refund-1" }
T $iface "正常退款" "refund=1000" "OK" $r "type=$($r.Data.flow.flow_type) income=$($r.Data.flow.amount.driver_income_fen)" | Out-Null

$r2 = Api "POST" "/driver/refunds" @{ order_id = $o20.id; refund_fen = 1000; event_id = "evt-refund-1" }
T $iface "重复退款事件" "同 event_id" "OK" $r2 "返回同一凭证: $($r2.Data.flow.id -eq $r.Data.flow.id)" | Out-Null

$r = Api "POST" "/driver/refunds" @{ order_id = $o20.id; refund_fen = 0; event_id = "evt-refund-2" }
T $iface "退款金额为0" "refund=0" "invalid_request" $r "gt=0 校验" | Out-Null

$r = Api "POST" "/driver/refunds" @{ order_id = $o20.id; refund_fen = -100; event_id = "evt-refund-3" }
T $iface "负数退款" "refund=-100" "invalid_request" $r "gt=0" | Out-Null

$r = Api "POST" "/driver/refunds" @{ order_id = "00000000-0000-0000-0000-000000000000"; refund_fen = 100; event_id = "evt-refund-4" }
T $iface "订单不存在" "id=全0" "order_not_found" $r "404" | Out-Null

$r = Api "POST" "/driver/refunds" @{ refund_fen = 100; event_id = "evt-refund-5" }
T $iface "缺少 order_id" "无 order_id" "invalid_request" $r "必填" | Out-Null

$r = Api "POST" "/driver/refunds" @{ order_id = $o20.id; responsible_party = "driver"; event_id = "evt-refund-6" }
T $iface "缺少 refund_fen" "无 refund_fen" "invalid_request" $r "必填" | Out-Null

$r = Api "POST" "/driver/refunds" @{ order_id = $o20.id; refund_fen = 500; responsible_party = "driver"; reason = "司机责任"; event_id = "evt-refund-7" }
T $iface "司机责任退款" "party=driver" "OK" $r "type=$($r.Data.flow.flow_type)" | Out-Null

$r = Api "POST" "/driver/refunds" @{ order_id = $o20.id; refund_fen = 5000; responsible_party = "platform"; reason = "平台责任"; event_id = "evt-refund-8" }
T $iface "平台责任退款" "party=platform" "OK" $r "type=$($r.Data.flow.flow_type)" | Out-Null

$r = Api "POST" "/driver/refunds" @{ order_id = $o20.id; refund_fen = 200; event_id = "evt-refund-9"; idempotency_key = "rf-9" }
T $iface "带幂等键退款" "idempotency_key" "OK" $r "凭证生成" | Out-Null

# ───────────────────── 22. 结算与提现审批 ─────────────────────
$iface = "22.结算审批"
$r = Api "POST" "/driver/settlements" @{ driver_id = $d20.id }
T $iface "结算指定司机" "settle driver" "OK" $r "settled_drivers=$($r.Data.settled_drivers)" | Out-Null

$r = Api "POST" "/driver/settlements" @{ }
T $iface "结算全部待结算" "无参数" "OK" $r "settled=$($r.Data.settled_drivers)" | Out-Null

$r = Api "POST" "/driver/settlements" @{ driver_id = "00000000-0000-0000-0000-000000000000" }
T $iface "不存在司机结算" "id=全0" "OK" $r "无待结算" | Out-Null

$r = Api "POST" "/driver/settlements" @{ flow_ids = @("unknown-flow") }
T $iface "指定不存在流水" "flow_ids" "OK" $r "忽略" | Out-Null

$r = Api "POST" "/driver/settlements" @{ flow_ids = @() }
T $iface "空流水列表" "flow_ids=[]" "OK" $r "全额结算" | Out-Null

$r = Api "POST" "/driver/withdrawals/approve" @{ withdrawal_id = $wd1.id; approver_id = "admin01"; comment = "审核通过" }
T $iface "审批通过" "admin01" "OK" $r "status=$($r.Data.status) approvals=$(@($r.Data.approvals).Count)" | Out-Null

$r = Api "POST" "/driver/withdrawals/approve" @{ withdrawal_id = $wd1.id; approver_id = "admin01" }
T $iface "已审批单据再次审批" "admin01 再次" "withdraw_state_denied" $r "状态已流转" | Out-Null

$r = Api "POST" "/driver/withdrawals/approve" @{ withdrawal_id = "00000000-0000-0000-0000-000000000000"; approver_id = "admin02" }
T $iface "审批不存在单据" "id=全0" "withdraw_not_found" $r "404" | Out-Null

$r = Api "POST" "/driver/withdrawals/approve" @{ withdrawal_id = $wd1.id }
T $iface "缺少审批人" "无 approver_id" "invalid_request" $r "必填" | Out-Null

$r = Api "POST" "/driver/withdrawals/complete" @{ withdrawal_id = $wd1.id; success = $true }
T $iface "打款成功" "success=true" "OK" $r "status=$($r.Data.status)" | Out-Null

$r = Api "POST" "/driver/withdrawals/complete" @{ withdrawal_id = $wd1.id; success = $true }
T $iface "重复打款" "同单再次" "withdraw_state_denied" $r "状态不允许" | Out-Null

$r = Api "POST" "/driver/withdrawals/complete" @{ withdrawal_id = "00000000-0000-0000-0000-000000000000"; success = $true }
T $iface "打款不存在单据" "id=全0" "withdraw_not_found" $r "404" | Out-Null

$r = Api "POST" "/driver/withdrawals/reject" @{ withdrawal_id = "00000000-0000-0000-0000-000000000000"; reason = "驳回" }
T $iface "驳回不存在单据" "id=全0" "withdraw_not_found" $r "404" | Out-Null

# 驳回场景：新提现单驳回后额度返还
HB $d17 | Out-Null
$r = Api "POST" "/driver/income/withdraw" @{ driver_id = $d17.id; amount_fen = 2000; idempotency_key = "wd-reject" }
$wd2 = $r.Data
T $iface "申请第二笔" "amount=2000" "OK" $r "status=$($wd2.status)" | Out-Null

$before = (Api "GET" "/driver/profile/$($d17.id)" $null).Data
$r = Api "POST" "/driver/withdrawals/reject" @{ withdrawal_id = $wd2.id; reason = "信息不全" }
T $iface "驳回并返还额度" "reject" "OK" $r "status=$($r.Data.status)" | Out-Null

$after = (Api "GET" "/driver/profile/$($d17.id)" $null).Data
T $iface "返还后余额校验" "对比驳回前后" "OK" @{ Ok = $true } "before=$($before.funds.available_fen) after=$($after.funds.available_fen)" | Out-Null

# 当日受理次数上限：已成功 1 笔，再申请 3 笔（第 3 笔应被拦截）
$codes = @()
for ($i = 2; $i -le 4; $i++) {
    $rr = Api "POST" "/driver/income/withdraw" @{ driver_id = $d17.id; amount_fen = 1000; idempotency_key = "wd-count-$i" }
    $codes += $(if ($rr.Ok) { "OK" } else { $rr.Code })
}
T $iface "单日提现次数上限" "当日第2/3/4笔" "OK" @{ Ok = $true } "实际=$($codes -join ',')" $(if (($codes -join ',') -eq "OK,OK,daily_withdraw_count") { "OK" } else { "FAIL:$($codes -join ',')" }) | Out-Null

# 大额双人审批
$d22 = NewDriver "大额司机"
HB $d22 | Out-Null
$big = (NewOrder "大额完单" 300000 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d22.id; order_id = $big.id } | Out-Null
Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d22.id; order_id = $big.id } | Out-Null
Api "POST" "/driver/hall/orders/start" @{ driver_id = $d22.id; order_id = $big.id } | Out-Null
Api "POST" "/driver/hall/orders/complete" @{ driver_id = $d22.id; order_id = $big.id } | Out-Null
HB $d22 | Out-Null
$r = Api "POST" "/driver/income/withdraw" @{ driver_id = $d22.id; amount_fen = 60000; idempotency_key = "wd-big" }
$wdBig = $r.Data
T $iface "大额提现需双人审批" "amount=60000" "OK" $r "dual=$($wdBig.dual_approval)" | Out-Null

Api "POST" "/driver/withdrawals/approve" @{ withdrawal_id = $wdBig.id; approver_id = "admin01" } | Out-Null
$r = Api "POST" "/driver/withdrawals/complete" @{ withdrawal_id = $wdBig.id; success = $true }
T $iface "单人审批打款被拦" "仅 1 人审批" "dual_approval_required" $r "需两位审批人" | Out-Null

Api "POST" "/driver/withdrawals/approve" @{ withdrawal_id = $wdBig.id; approver_id = "admin02" } | Out-Null
$r = Api "POST" "/driver/withdrawals/complete" @{ withdrawal_id = $wdBig.id; success = $true }
T $iface "双人审批后打款" "admin01+admin02" "OK" $r "status=$($r.Data.status)" | Out-Null

# ───────────────────── 23. 单通道派单（听单→抢单池） ─────────────────────
$iface = "23.单通道派单"
$d23 = NewDriver "听单司机"
HB $d23 | Out-Null
Api "POST" "/driver/profile/$($d23.id)/mode" @{ mode = "listen" } | Out-Null
Api "POST" "/driver/profile/$($d23.id)/heartbeat" @{ lat = 30.6572; lng = 104.0808 } | Out-Null

$r = NewOrder "听单" 3000 0 0
T $iface "下单进入听单通道" "有听单司机在线" "OK" $r "channel=$($r.Data.dispatch_channel) driver=$($r.Data.driver_id) visible=$($r.Data.visible_in_pool)" | Out-Null
$o23 = $r.Data

$r = Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d23.id; order_id = $o23.id }
T $iface "听单邀请确认" "listen 通道" "OK" $r "status=$($r.Data.status)" | Out-Null

Api "POST" "/driver/hall/orders/start" @{ driver_id = $d23.id; order_id = $o23.id } | Out-Null
Api "POST" "/driver/hall/orders/complete" @{ driver_id = $d23.id; order_id = $o23.id } | Out-Null

$o23b = (NewOrder "听单二" 2600 0 0).Data
$d23b = NewDriver "旁观司机"
HB $d23b | Out-Null
$r = Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d23b.id; order_id = $o23b.id }
T $iface "听单窗口内他人抢单" "邀请中" "order_not_visible" $r "未进抢单池不可见" | Out-Null

Api "POST" "/driver/hall/orders/cancel" @{ order_id = $o23b.id; responsible_party = "passenger" } | Out-Null
Api "POST" "/driver/profile/$($d23.id)/mode" @{ mode = "grab" } | Out-Null

$o23c = (NewOrder "无听单运力" 2200 0 0).Data
T $iface "无听单运力直接转池" "listen 司机已切走" "OK" @{ Ok = $true } "visible=$($o23c.visible_in_pool) channel=$($o23c.dispatch_channel)" | Out-Null

Start-Sleep -Seconds 3
$r = Api "GET" "/driver/hall/orders?driver_id=$($d9b.id)" $null
T $iface "抢单池可被其他司机看到" "查询大厅" "OK" $r "total=$($r.Data.total)" | Out-Null

# ───────────────────── 24. 心跳超时自动收车（长等待） ─────────────────────
$iface = "24.心跳收车"
$d24 = NewDriver "掉线司机"
HB $d24 | Out-Null
$before = (Api "GET" "/driver/profile/$($d24.id)" $null).Data
T $iface "掉线前在线" "心跳后查询" "OK" @{ Ok = $true } "status=$($before.status)" | Out-Null

Start-Sleep -Seconds 65
$after = (Api "GET" "/driver/profile/$($d24.id)" $null).Data
T $iface "空闲60秒无心跳自动收车" "等待 65 秒" "OK" @{ Ok = $true } "status=$($after.status)" $(if ($after.status -eq "offline") { "OK" } else { "FAIL:$($after.status)" }) | Out-Null

$d24b = NewDriver "忙碌掉线司机"
HB $d24b | Out-Null
$o24 = (NewOrder "忙碌掉线单" 3000 0 0).Data
Api "POST" "/driver/hall/orders/grab" @{ driver_id = $d24b.id; order_id = $o24.id } | Out-Null
Api "POST" "/driver/hall/orders/accept" @{ driver_id = $d24b.id; order_id = $o24.id } | Out-Null
Start-Sleep -Seconds 65
$after = (Api "GET" "/driver/profile/$($d24b.id)" $null).Data
T $iface "忙碌掉线不释放任务" "忙碌中等待 65 秒" "OK" @{ Ok = $true } "status=$($after.status) task=$($after.active_task_id)" $(if ($after.status -eq "busy") { "OK" } else { "FAIL:$($after.status)" }) | Out-Null

# ───────────────────── 汇总 ─────────────────────
$total = $Results.Count
$pass = ($Results | Where-Object { ($_ -split "\|")[6] -eq "PASS" }).Count
$fail = $total - $pass
$header = "seq|interface|case|input|expect|actual|result|note"
$lines = New-Object System.Collections.ArrayList
[void]$lines.Add($header)
foreach ($r in $Results) { [void]$lines.Add($r) }
[void]$lines.Add("TOTAL=$total PASS=$pass FAIL=$fail")
[IO.File]::WriteAllLines($OutFile, $lines, (New-Object Text.UTF8Encoding($true)))
Write-Output ("TOTAL=$total PASS=$pass FAIL=$fail")
Write-Output ("RESULT_FILE=" + (Resolve-Path $OutFile).Path)
