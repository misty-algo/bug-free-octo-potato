# 到达域端到端验收脚本（按网关真实路由编写）
#
# 覆盖：D05-D07 行程服务、D08 服务规范、D14/D15 车辆资料与审核、D17/D18 安全中心，
# 以及任务分配这个行程前置环节。路由依据 internal/gateway/router/ 下 arrival、
# vehicle、safety 三个路由文件。
#
# 旧契约脚本（/ride-tasks 生命周期）保留为 scripts/arrival-e2e-contract.ps1，
# 那组端点尚未实现，本文件才是当前可运行的验收版本。
#
# 用法（仓库根目录，需已启动 gateway/driver/order）：
#   powershell -ExecutionPolicy Bypass -File scripts/arrival-e2e.ps1
# MongoContainer 用于准备行程前置数据：行程闭环依赖 drivers / orders 两个集合里
# 已有司机与行程订单，而网关目前没有创建这两类文档的接口（对应 docs/arrival/
# openapi.yaml 里尚未实现的 ride-tasks 生命周期），因此脚本直连 Mongo 补齐。
param(
    [string]$BaseURL = 'http://127.0.0.1:8080',
    [string]$ConfigPath = 'configs/config.yaml',
    [string]$OutputPath = 'arrival-evidence.json',
    [string]$MongoContainer = 'mongo'
)
$ErrorActionPreference = 'Continue'
[Console]::OutputEncoding = [Text.Encoding]::UTF8

# ---------- 令牌：与服务端共用同一份 JWT 配置 ----------
function Get-JwtSetting([string]$name, [string]$defaultValue) {
    if (Test-Path $ConfigPath) {
        $lines = Get-Content -LiteralPath $ConfigPath
        for ($i = 0; $i -lt $lines.Count; $i++) {
            if ($lines[$i] -match '^\s*jwt:\s*$') {
                for ($k = $i + 1; $k -lt [Math]::Min($i + 8, $lines.Count); $k++) {
                    if ($lines[$k] -match ('^\s*' + $name + ':\s*"?([^"#]*)"?\s*$')) {
                        $v = $Matches[1].Trim()
                        if ($v) { return $v }
                    }
                }
            }
        }
    }
    return $defaultValue
}

$jwtSecret = if ($env:ARRIVAL_JWT_SECRET) { $env:ARRIVAL_JWT_SECRET } else { Get-JwtSetting 'secret' '' }
$jwtIssuer = if ($env:ARRIVAL_JWT_ISSUER) { $env:ARRIVAL_JWT_ISSUER } else { Get-JwtSetting 'issuer' 'xiaolanshu' }
if (-not $jwtSecret) {
    Write-Error ("JWT secret is empty: fill middleware.jwt.secret in {0} or set ARRIVAL_JWT_SECRET." -f $ConfigPath)
    exit 2
}

function Get-MongoSetting([string]$name, [string]$defaultValue) {
    if (Test-Path $ConfigPath) {
        $lines = Get-Content -LiteralPath $ConfigPath
        for ($i = 0; $i -lt $lines.Count; $i++) {
            if ($lines[$i] -match '^mongodb:\s*$') {
                for ($k = $i + 1; $k -lt [Math]::Min($i + 14, $lines.Count); $k++) {
                    if ($lines[$k] -match ('^\s*' + $name + ':\s*"?([^"#]*)"?\s*$')) {
                        $v = $Matches[1].Trim()
                        if ($v) { return $v }
                    }
                }
            }
        }
    }
    return $defaultValue
}

$mongoUser = Get-MongoSetting 'username' ''
$mongoPass = Get-MongoSetting 'password' ''
$mongoDb = Get-MongoSetting 'database' 'day1'

function Invoke-MongoJs([string]$js) {
    $dockerArgs = @('exec', $MongoContainer, 'mongosh', '--quiet')
    if ($mongoUser) { $dockerArgs += @('--username', $mongoUser, '--password', $mongoPass, '--authenticationDatabase', 'admin') }
    $dockerArgs += @($mongoDb, '--eval', $js)
    try { return ((& docker @dockerArgs 2>$null) -join "`n") } catch { return '' }
}

# New-TripFixture 补齐行程所需的前置文档：司机、已接单的行程订单，
# 并把订单挂到刚创建的任务上。缺这块数据时到达/开始/送达都会拒绝执行。
function New-TripFixture([string]$taskId, [string]$drvId, [string]$ordId, [string]$psgId, [string]$ordNo, [string]$phone) {
    # 这里一律用单引号：PowerShell 调用原生命令时会剥掉参数里的双引号，
    # 写双引号会把 ObjectId("...") 变成 ObjectId(...) 而让 mongosh 报错。
    $tpl = @'
const drv = ObjectId('__DRV__');
const task = ObjectId('__TASK__');
const ord = ObjectId('__ORD__');
db.getCollection('drivers').updateOne({_id: drv}, {$setOnInsert: {createdAt: new Date()}, $set: {updatedAt: new Date()}}, {upsert: true});
db.getCollection('orders').updateOne({_id: ord}, {$set: {
  orderNo: '__ORDNO__',
  passengerId: ObjectId('__PSG__'),
  state: 'ACCEPTED',
  version: 1,
  driverId: drv,
  dispatchId: task,
  startLocation: {type: 'Point', coordinates: [121.4737, 31.2304]},
  verified: false,
  verifyFailures: 0,
  timeline: [],
  priceSnap: {T: 1800, Q: 500, H: 200, F: 1200, K: 0, fixed: false},
  cfgVersion: NumberInt(1)
}}, {upsert: true});
db.getCollection('accounts').updateOne({_id: ObjectId('__PSG__')}, {$set: {phone: '__PHONE__', name: 'e2e-passenger'}}, {upsert: true});
db.getCollection('dispatch_tasks').updateOne({_id: task}, {$set: {orderIds: [ord]}});
print('fixture-ready');
'@
    $js = $tpl.Replace('__DRV__', $drvId).Replace('__TASK__', $taskId).Replace('__ORD__', $ordId).
        Replace('__ORDNO__', $ordNo).Replace('__PSG__', $psgId).Replace('__PHONE__', $phone)
    return Invoke-MongoJs $js
}

function ConvertTo-Base64Url([string]$text) {
    $b64 = [Convert]::ToBase64String([Text.Encoding]::UTF8.GetBytes($text))
    return ($b64 -replace '\+', '-' -replace '/', '_' -replace '=', '')
}

# New-AccessToken 直接签发令牌，跳过尚未实现的登录链路（与 cmd/mint-token 同源自一套声明）。
function New-AccessToken([string]$userId, [string]$role) {
    $now = [DateTimeOffset]::UtcNow.ToUnixTimeSeconds()
    # 这里用拼接而非 -f：JSON 的 {} 会与格式化占位符冲突。
    $payload = '{"user_id":"' + $userId + '","role":"' + $role + '","iss":"' + $jwtIssuer +
        '","nbf":' + [string]($now - 60) + ',"iat":' + [string]$now + ',"exp":' + [string]($now + 7200) + '}'
    $signingInput = (ConvertTo-Base64Url '{"alg":"HS256","typ":"JWT"}') + '.' + (ConvertTo-Base64Url $payload)
    $hmac = New-Object System.Security.Cryptography.HMACSHA256
    $hmac.Key = [Text.Encoding]::UTF8.GetBytes($jwtSecret)
    $sig = [Convert]::ToBase64String($hmac.ComputeHash([Text.Encoding]::UTF8.GetBytes($signingInput)))
    return $signingInput + '.' + ($sig -replace '\+', '-' -replace '/', '_' -replace '=', '')
}

# ---------- 测试基座 ----------
$api = $BaseURL.TrimEnd('/') + '/api'
$results = @()

function Invoke-Case {
    param([string]$Id, [string]$Desc, [string]$Method, [string]$Path, [string]$Role, $Body, [int[]]$Expect = @(200), [switch]$Raw)
    $role = $Role
    # safety_handler 在网关守卫里的取值就是 safety_handler
    $token = if ($role) { New-AccessToken $script:UserId[$role] $role } else { '' }
    $code = 0; $resp = ''
    try {
        $a = @{ Method = $Method; Uri = ($api + $Path); TimeoutSec = 15; UseBasicParsing = $true }
        if ($token) { $a['Headers'] = @{ Authorization = 'Bearer ' + $token } }
        if ($null -ne $Body) {
            if ($Raw) { $a['InFile'] = $Body } else {
                $a['Body'] = [Text.Encoding]::UTF8.GetBytes((ConvertTo-Json $Body -Depth 10 -Compress))
                $a['ContentType'] = 'application/json; charset=utf-8'
            }
        }
        $r = Invoke-WebRequest @a
        $code = [int]$r.StatusCode; $resp = $r.Content
    } catch {
        if ($_.Exception.Response) { $code = [int]$_.Exception.Response.StatusCode }
        $resp = $_.ErrorDetails.Message
        if (-not $resp) { try { $resp = (New-Object IO.StreamReader($_.Exception.Response.GetResponseStream())).ReadToEnd() } catch { } }
    }
    $ok = $Expect -contains $code
    $errMsg = ''
    try { $j = $resp | ConvertFrom-Json; if ($j.error) { $errMsg = $j.error.code + ': ' + $j.error.message } } catch { }
    if ($ok) { $script:results += [pscustomobject]@{ Case = $Id; Desc = $Desc; Code = $code; Error = $errMsg; Ok = $true } } `
    else { $script:results += [pscustomobject]@{ Case = $Id; Desc = $Desc; Code = $code; Error = $errMsg; Ok = $false } }
    Write-Host ('{0,-6} {1,-32} {2,3} {3} {4}' -f $Id, $Desc, $code, $(if ($ok) { 'PASS' } else { 'FAIL' }), $errMsg)
    return $resp
}

function Assert-Equal([string]$Id, [string]$Desc, $actual, $expected) {
    $ok = ($actual -eq $expected)
    $script:results += [pscustomobject]@{ Case = $Id; Desc = $Desc; Code = 0; Error = $(if ($ok) { '' } else { "expected $expected" }); Ok = $ok }
    Write-Host ('{0,-6} {1,-32}     {2}' -f $Id, $Desc, $(if ($ok) { 'PASS' } else { 'FAIL' }))
}

# Get-BusinessBody 取出响应里的 data 部分：整包含每次不同的 request_id，
# 幂等一致性只能比业务体。
function Get-BusinessBody([string]$json) {
    try { return (($json | ConvertFrom-Json).data | ConvertTo-Json -Depth 12 -Compress) } catch { return $json }
}

function Get-Data([string]$json, [string]$field) {
    try { return ($json | ConvertFrom-Json).data.$field } catch { return $null }
}

# 司机号取 ObjectId 形态：行程域与派单域以 Mongo ObjectId 作为司机主键。
$driverId = [Guid]::NewGuid().ToString('N').Substring(0, 24)
$orderId = [Guid]::NewGuid().ToString('N').Substring(0, 24)
$passengerId = [Guid]::NewGuid().ToString('N').Substring(0, 24)
# 上车核验取手机号末 4 位（entity.VerifyCodeLength），因此这里直接生成 4 位尾号。
$passengerPhone = '1380000' + (Get-Random -Minimum 1000 -Maximum 9999)
$script:VerifyCode = $passengerPhone.Substring($passengerPhone.Length - 4)
$script:UserId = @{
    driver          = $driverId
    passenger       = 'psg-' + $driverId.Substring(0, 8)
    dispatcher      = 'dsp-' + $driverId.Substring(0, 8)
    reviewer        = 'rev-' + $driverId.Substring(0, 8)
    auditor         = 'aud-' + $driverId.Substring(0, 8)
    safety_handler  = 'sfh-' + $driverId.Substring(0, 8)
}

Write-Host '=== D14/D15 车辆资料与审核 ==='
$pngPath = Join-Path $env:TEMP ('material-' + [guid]::NewGuid().ToString('N') + '.png')
[IO.File]::WriteAllBytes($pngPath, [Convert]::FromBase64String('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='))

$fileId = Get-Data (Invoke-Case 'V1' '创建材料上传会话' POST '/drv/file/upload-session' 'driver' @{ purpose = 'material' }) 'file_id'
if ($fileId) {
    Invoke-Case 'V2' '上传材料内容(PNG)' POST ("/drv/file/upload?file_id=$fileId&filename=material.png") 'driver' $pngPath -Raw | Out-Null
}
Remove-Item $pngPath -Force -ErrorAction SilentlyContinue

$nowSec = [int64](Get-Date -UFormat %s)
$future = $nowSec + 31536000
$draft = Invoke-Case 'V3' '保存车辆草稿' POST '/drv/vehicle/draft' 'driver' @{
    vin = 'LSVAM4187E2100001'; plate = ('沪A' + (Get-Random -Minimum 10000 -Maximum 99999))
    city_code = 'shanghai'; model_code = 'sedan'; seats = 4; color = 'white'; file_ids = @($fileId)
}
$ver = Get-Data $draft 'version'; if (-not $ver) { $ver = 1 }

foreach ($d in @('driving_license', 'vehicle_license', 'insurance', 'ride_hailing_permit')) {
    Invoke-Case ("V4-" + $d) ("保存证件:" + $d) POST '/drv/vehicle/document' 'driver' @{
        doc_type = $d; doc_no = ($d.Substring(0, 3).ToUpper() + '90001'); issued_at = $nowSec; valid_until = $future
    } | Out-Null
}

$appId = 'app-' + [guid]::NewGuid().ToString('N')
Invoke-Case 'V5' '提交车辆申请' POST '/drv/vehicle/application' 'driver' @{ application_id = $appId; expected_draft_version = $ver } | Out-Null
Invoke-Case 'V6' '查询申请详情' GET '/drv/vehicle/application' 'driver' $null | Out-Null
Invoke-Case 'A1' '待审核列表(auditor)' GET '/admin/driver-audit/pending' 'auditor' $null | Out-Null
Invoke-Case 'A2' '领取申请(auditor)' POST '/admin/driver-audit/claim' 'auditor' @{ application_id = $appId; revision = 1 } | Out-Null
Invoke-Case 'A3' '批准申请(auditor)' POST '/admin/driver-audit/approve' 'auditor' @{ application_id = $appId; revision = 1; expected_review_version = 1 } | Out-Null
Invoke-Case 'A4' '查询当前绑定' GET '/drv/vehicle/binding' 'driver' $null | Out-Null
Invoke-Case 'A5' '实时准入自查' GET '/drv/eligibility' 'driver' $null | Out-Null

Write-Host ''
Write-Host '=== D05-D07 行程服务 ==='
$taskNo = 'T' + (Get-Random -Minimum 100000 -Maximum 999999)
$assign = Invoke-Case 'T1' '任务分配(auditor)' POST '/admin/dispatch/assign' 'auditor' @{ driver_id = $driverId; order_ids = @($orderId); task_no = $taskNo }
$taskId = Get-Data $assign 'task_id'

if ($taskId) {
    $fx = New-TripFixture $taskId $driverId $orderId $passengerId $taskNo $passengerPhone
    Write-Host ('     fixture(drivers/orders): ' + (($fx -replace '\s+', ' ').Trim()))
    Invoke-Case 'T2' '位置上报' POST '/drv/position' 'driver' @{ task_id = $taskId; lng = 121.4737; lat = 31.2304 } | Out-Null
    $arrKey = 'arrive-' + [guid]::NewGuid().ToString('N')
    $first = Invoke-Case 'T3' '到达确认' POST '/drv/trip/arrive' 'driver' @{ task_id = $taskId; idempotency_key = $arrKey }
    $second = Invoke-Case 'T4' '到达幂等重放' POST '/drv/trip/arrive' 'driver' @{ task_id = $taskId; idempotency_key = $arrKey }
    # 只比业务体：整包里 request_id 每次都不同，不能作为一致性依据。
    Assert-Equal 'T5' '幂等重放业务结果一致' (Get-BusinessBody $first) (Get-BusinessBody $second)
    Invoke-Case 'T6' '异常到达(填写原因)' POST '/drv/trip/arrive' 'driver' @{ task_id = $taskId; idempotency_key = ('arrive-x-' + [guid]::NewGuid().ToString('N')); reason = '定位失败，司机报已到达' } | Out-Null
    # 出发前必须先完成上车核验：这是服务端的业务要求，不是缺陷。
    Invoke-Case 'T6b' '上车核验(手机号后四位)' POST '/drv/trip/verify' 'driver' @{
        order_id = $orderId; code = $script:VerifyCode
        idempotency_key = ('verify-' + [guid]::NewGuid().ToString('N'))
    } | Out-Null
    Invoke-Case 'T7' '开始行程' POST '/drv/trip/start' 'driver' @{ task_id = $taskId; idempotency_key = ('start-' + [guid]::NewGuid().ToString('N')) } | Out-Null
    Invoke-Case 'T8' '轨迹采样' POST '/drv/trip/sample' 'driver' @{ task_id = $taskId; session_id = 's1'; seq = 1; lng = 121.4740; lat = 31.2308; crs = 'GCJ02'; accuracy = 5; collected_at = $nowSec } | Out-Null
    Invoke-Case 'T9' '送达结束' POST '/drv/trip/end' 'driver' @{ order_id = $orderId; actual_dist = 5200; actual_dur = 900; end_reason = 'NORMAL'; idempotency_key = ('end-' + [guid]::NewGuid().ToString('N')) } | Out-Null
    # 任务号与任务 ObjectId 都应能命中同一个任务
    Invoke-Case 'T10' '按任务号也能到达' POST '/drv/trip/arrive' 'driver' @{ task_id = $taskNo; idempotency_key = ('arrive-no-' + [guid]::NewGuid().ToString('N')) } -Expect @(200, 409) | Out-Null
} else {
    Write-Host '     skip trip cases: task was not created'
}

Write-Host ''
Write-Host '=== D17/D18 安全中心 ==='
$sReq = 'sos-' + [guid]::NewGuid().ToString('N')
$sBody = @{ client_request_id = $sReq; event_type = 'accident'; description = '路面事故'; has_position = $true; lng = 121.4737; lat = 31.2304; captured_at = $nowSec }
$s1 = Invoke-Case 'S1' '紧急求助上报' POST '/drv/safety/event' 'driver' $sBody
$s2 = Invoke-Case 'S2' '重复上报同一请求号' POST '/drv/safety/event' 'driver' $sBody
$eid1 = Get-Data $s1 'event_id'; $eid2 = Get-Data $s2 'event_id'
Assert-Equal 'S3' '重复上报返回同一事件' ($eid1 -eq $eid2 -and $eid1) $true
Invoke-Case 'S4' '按请求号查询' GET ("/drv/safety/event/by-request?client_request_id=$sReq") 'driver' $null | Out-Null
Invoke-Case 'S5' '无定位也能上报' POST '/drv/safety/event' 'driver' @{ client_request_id = ('snp-' + [guid]::NewGuid().ToString('N')); event_type = 'medical'; description = '乘客不适'; has_position = $false } | Out-Null
if ($eid1) {
    Invoke-Case 'S6' '追加补充说明' POST '/drv/safety/event/supplement' 'driver' @{ event_id = $eid1; kind = 'note'; note = '已联系乘客' } | Out-Null
}
Invoke-Case 'S7' '后台安全队列' GET '/admin/safety/events' 'safety_handler' $null | Out-Null
Invoke-Case 'S8' '热线指引' GET '/drv/safety/hotline' 'driver' $null | Out-Null

Write-Host ''
Write-Host '=== D08 服务规范（依赖 A18 字典，未配置时标记为已知缺口）==='
$p = Invoke-Case 'P1' '服务规范内容' GET '/drv/service-spec' 'driver' $null -Expect @(200, 404)
if (($p | ConvertFrom-Json).error.code -eq 'SPEC_NOT_PUBLISHED') {
    Write-Host '     NOTE: A18 config_dictionary 未发布服务规范，非本域缺陷'
}

Write-Host ''
Write-Host '=== 汇总 ==='
$total = $results.Count
$pass = @($results | Where-Object { $_.Ok }).Count
$fail = $total - $pass
Write-Host ("TOTAL={0} PASS={1} FAIL={2}" -f $total, $pass, $fail)
if ($fail -gt 0) {
    Write-Host '--- 失败明细 ---'
    $results | Where-Object { -not $_.Ok } | ForEach-Object { Write-Host ("  {0} {1}: {2}" -f $_.Case, $_.Desc, $_.Error) }
}
$results | ConvertTo-Json -Depth 8 | Set-Content -LiteralPath $OutputPath -Encoding UTF8
Write-Host ("evidence saved: {0}" -f $OutputPath)
