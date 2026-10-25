[CmdletBinding()]
param(
    [string]$CCompiler = '',
    [string]$OutputDirectory = 'tmp/arrival-verification',
    [switch]$Integration
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent $PSScriptRoot
$previousLocation = Get-Location
$previousCGO = [Environment]::GetEnvironmentVariable('CGO_ENABLED', 'Process')
$previousCC = [Environment]::GetEnvironmentVariable('CC', 'Process')
$previousPath = $env:PATH

function Invoke-GoCheck([string]$Name, [string[]]$GoArguments) {
    $logPath = Join-Path $verificationDirectory ($Name + '.log')
    & go @GoArguments *> $logPath
    $exitCode = $LASTEXITCODE
    if ($exitCode -ne 0) {
        Get-Content -LiteralPath $logPath -Tail 60 | Write-Output
        throw ('{0} failed (exit {1}); log: {2}' -f $Name, $exitCode, $logPath)
    }
    Write-Output ('PASS {0}' -f $Name)
}

try {
    Set-Location -LiteralPath $repoRoot
    $branch = & git branch --show-current
    if ($LASTEXITCODE -ne 0 -or $branch -eq 'main' -or $branch -eq 'dev' -or [string]::IsNullOrWhiteSpace($branch)) {
        throw 'Run arrival development verification on a named development branch, not main/dev.'
    }
    if ($Integration) {
        if (-not $env:ARRIVAL_TEST_MONGO_URI -or
            $env:ARRIVAL_TEST_MONGO_DATABASE -notlike 'arrival_test_*' -or
            $env:ARRIVAL_TEST_ALLOW_WRITES -ne '1') {
            throw 'Integration requires an explicit isolated MongoDB replica-set URI, arrival_test_* database and ARRIVAL_TEST_ALLOW_WRITES=1. Existing business databases must not be used.'
        }
        if (-not $env:ARRIVAL_E2E_URL -or -not $env:ARRIVAL_JWT_SECRET) {
            throw 'Integration also requires ARRIVAL_E2E_URL and an isolated ARRIVAL_JWT_SECRET for the running HTTP/RPC services.'
        }
    }
    if (-not $CCompiler) {
        if ($previousCC -and (Test-Path -LiteralPath $previousCC)) {
            $CCompiler = $previousCC
        } else {
            $gcc = Get-Command gcc -ErrorAction SilentlyContinue
            if ($gcc) { $CCompiler = $gcc.Source }
            elseif (Test-Path -LiteralPath 'C:/msys64/ucrt64/bin/gcc.exe') { $CCompiler = 'C:/msys64/ucrt64/bin/gcc.exe' }
        }
    }
    if (-not $CCompiler -or -not (Test-Path -LiteralPath $CCompiler -PathType Leaf)) {
        throw 'CGO/race requires a C compiler. Pass -CCompiler with the installed gcc executable path.'
    }
    $env:CGO_ENABLED = '1'
    $env:CC = (Resolve-Path -LiteralPath $CCompiler).Path
    $env:PATH = (Split-Path -Parent $env:CC) + [IO.Path]::PathSeparator + $previousPath
    $verificationDirectory = [IO.Path]::GetFullPath((Join-Path $repoRoot $OutputDirectory))
    New-Item -ItemType Directory -Force -Path $verificationDirectory | Out-Null
    Write-Output ('Branch: {0}; logs: {1}' -f $branch, $verificationDirectory)

    Invoke-GoCheck 'build' @('build', '-mod=readonly', './...')
    Invoke-GoCheck 'vet' @('vet', '-mod=readonly', './...')
    $coveragePath = Join-Path $verificationDirectory 'coverage.out'
    Invoke-GoCheck 'tests' @('test', '-mod=readonly', '-count=1', ('-coverprofile=' + $coveragePath), './...')
    Invoke-GoCheck 'race' @('test', '-mod=readonly', '-count=1', '-race', './...')
    if ($Integration) {
        Invoke-GoCheck 'integration' @('test', '-mod=readonly', '-count=1', '-race', '-tags=integration', './test/arrival', '-v')
    }
    & go tool cover ('-func=' + $coveragePath) | Select-Object -Last 1
    if ($LASTEXITCODE -ne 0) { throw 'Coverage report failed.' }
    if (-not $Integration) { Write-Output 'Database/HTTP E2E acceptance: NOT RUN (use -Integration only after isolated environment preparation).' }
}
finally {
    if ($null -eq $previousCGO) {
        Remove-Item Env:CGO_ENABLED -ErrorAction SilentlyContinue
    } else {
        [Environment]::SetEnvironmentVariable('CGO_ENABLED', $previousCGO, 'Process')
    }
    if ($null -eq $previousCC) {
        Remove-Item Env:CC -ErrorAction SilentlyContinue
    } else {
        [Environment]::SetEnvironmentVariable('CC', $previousCC, 'Process')
    }
    $env:PATH = $previousPath
    Set-Location -LiteralPath $previousLocation
}
