# 花小猪主项目本地一键启动：8 个微服务 + 乘客端/司机端页面(8081)。
# 数据库/中间件连接走 configs/config.yaml（当前指向线上实例）。
#
# 用法（在仓库任意目录的 PowerShell 终端执行）：
#   powershell -ExecutionPolicy Bypass -File scripts\dev-up.ps1            # 用已编译二进制启动
#   powershell -ExecutionPolicy Bypass -File scripts\dev-up.ps1 -Build     # 先重新编译再启动（改过 Go 代码时）
#   powershell -ExecutionPolicy Bypass -File scripts\dev-up.ps1 -Stop      # 只停止全部服务
param(
  [switch]$Build,
  [switch]$Stop
)

$ErrorActionPreference = 'Continue'
$root   = Split-Path -Parent $PSScriptRoot
$binDir = Join-Path $root '.build'
$logDir = Join-Path $binDir 'logs'
New-Item -ItemType Directory -Force -Path $binDir, $logDir | Out-Null

$services = @('user', 'order', 'shop', 'payment', 'settlement', 'passenger', 'driver', 'gateway')

# ── 停止旧进程（避免端口占用）──
Write-Output '== 清理旧进程 =='
foreach ($svc in $services) {
  $exe = Join-Path $binDir "$svc.exe"
  if (Test-Path $exe) { Get-Process | Where-Object { $_.Path -eq $exe } | Stop-Process -Force -ErrorAction SilentlyContinue }
}
# 8081 静态页服务（web/server.mjs）
Get-CimInstance Win32_Process | Where-Object {
  $_.Name -eq 'node.exe' -and $_.CommandLine -like '*server.mjs*'
} | ForEach-Object { Stop-Process -Id $_.ProcessId -Force -ErrorAction SilentlyContinue }
Start-Sleep 2

if ($Stop) { Write-Output '已全部停止。'; return }

# ── 本地开发密钥（与 configs/config.yaml 的 security 约定一致；真值上线时必须更换）──
$env:SECURITY_DATA_KEY      = '0123456789abcdef0123456789abcdef'
$env:PASSENGER_PHONE_KEY    = '0123456789abcdef0123456789abcdef'
$env:PASSENGER_TOKEN_SECRET = 'local-dev-secret-change-me'

# ── 编译：-Build 强制编译；没传 -Build 但二进制缺失时自动编译（首次/清理后）──
$needBuild = $Build
if (-not $needBuild) {
  foreach ($svc in $services) {
    if (-not (Test-Path (Join-Path $binDir "$svc.exe"))) { $needBuild = $true; break }
  }
}
if ($needBuild) {
  Write-Output '== 编译 Go 服务 =='
  Push-Location $root
  foreach ($svc in $services) {
    go build -o (Join-Path $binDir "$svc.exe") "./cmd/$svc"
    if ($LASTEXITCODE -ne 0) { Write-Warning "$svc 编译失败" }
  }
  Pop-Location
}

# ── 启动微服务（日志写入 .build/logs/<svc>.log）──
Write-Output '== 启动微服务 =='
foreach ($svc in $services) {
  $exe = Join-Path $binDir "$svc.exe"
  if (-not (Test-Path $exe)) { Write-Warning "  缺少 $exe，请加 -Build 先编译"; continue }
  $log = Join-Path $logDir "$svc.log"
  Start-Process -FilePath $exe -WorkingDirectory $root -WindowStyle Hidden `
    -RedirectStandardOutput $log -RedirectStandardError "$log.err"
  Start-Sleep -Milliseconds 800
}

# ── 乘客端/司机端页面(8081) ──
Write-Output '== 启动前端(8081) =='
$webDir = Join-Path $root 'web'
if (Test-Path (Join-Path $webDir 'server.mjs')) {
  Start-Process -FilePath 'node' -ArgumentList 'server.mjs' -WorkingDirectory $webDir -WindowStyle Hidden `
    -RedirectStandardOutput (Join-Path $logDir 'web8081.log') -RedirectStandardError (Join-Path $logDir 'web8081.log.err')
} else { Write-Warning "  未找到 $webDir\server.mjs" }

# ── 汇报状态 ──
Write-Output ''
Write-Output '== 等待服务就绪 =='
Start-Sleep 8
foreach ($svc in $services) {
  $exe = Join-Path $binDir "$svc.exe"
  $running = Get-Process | Where-Object { $_.Path -eq $exe }
  Write-Output ("  {0,-12} {1}" -f $svc, $(if ($running) { 'running' } else { "DOWN(看 .build/logs/$svc.log.err)" }))
}

$ports = @(8080, 8081, 9001, 9005, 9100, 9101, 9102, 9103, 9104)
$down = @()
foreach ($p in $ports) {
  if (-not (Get-NetTCPConnection -LocalPort $p -State Listen -ErrorAction SilentlyContinue)) { $down += $p }
}
if ($down.Count -eq 0) { Write-Output '  端口 8080/8081/9001/9005/9100-9104 全部就绪' }
else { Write-Warning ("  未监听端口: " + ($down -join ', ') + "（线上库连接较慢，可稍后重跑本脚本确认）") }

try {
  $health = (Invoke-WebRequest -Uri 'http://127.0.0.1:8080/healthz' -UseBasicParsing -TimeoutSec 8).Content
  Write-Output "  gateway healthz: $health"
} catch { Write-Output '  gateway healthz: 暂未响应，稍等几秒再试' }

Write-Output ''
Write-Output '访问地址：'
Write-Output '  乘客端   http://localhost:8081/'
Write-Output '  司机端   http://localhost:8081/driver/'
Write-Output '日志目录：.build/logs/（每个服务一个 .log / .log.err）'
