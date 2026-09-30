# 启动"沙箱 APPDATA + CUA 桥"的调试实例：真实配置只读、Mod 目录沿用真实库（只读扫描）。
#
# 前提：先构建带桥的 EXE：pwsh -File scripts/devtools/build-cua.ps1
#
# 用法：
#   pwsh -File scripts/devtools/launch-cua-sandbox.ps1
#   pwsh -File scripts/devtools/launch-cua-sandbox.ps1 -ExeName LytVPK-Community-Fork-cua.exe -Port 38999
#
# 启动后可用（桥只在本机回环、且只有带 cua 标签的构建才有）：
#   POST http://127.0.0.1:<Port>/eval-sync  {"js":"document.title"}
# 抓像素：pwsh -File scripts/devtools/capture-window.ps1 -OutFile .tmp-cua\shots\window.png

[CmdletBinding()]
param(
    [string]$ExeName = 'LytVPK-Community-Fork-cua.exe',
    [int]$Port = 38999,
    [string]$SandboxRoot = '',
    [switch]$UseRealAddonsDir
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$exePath = Join-Path $repoRoot (Join-Path 'build\bin' $ExeName)
if (-not (Test-Path -LiteralPath $exePath)) {
    throw "找不到带桥的 EXE：$exePath（先运行 scripts/devtools/build-cua.ps1）"
}

if ($SandboxRoot -eq '') {
    $SandboxRoot = Join-Path $repoRoot '.tmp-cua\zz-appdata'
}
$sandboxLocal = Join-Path $SandboxRoot 'LocalAppData'
$sandboxConfigDir = Join-Path $SandboxRoot 'LytVPK'
New-Item -ItemType Directory -Force -Path $sandboxConfigDir, $sandboxLocal | Out-Null

# 用真实配置的副本做沙箱配置：真实 %APPDATA% 一个字节都不动。
$realConfigPath = Join-Path $env:APPDATA 'LytVPK\config.json'
$sandboxConfigPath = Join-Path $sandboxConfigDir 'config.json'
if (Test-Path -LiteralPath $realConfigPath) {
    $config = Get-Content -LiteralPath $realConfigPath -Raw -Encoding UTF8 | ConvertFrom-Json
    if (-not $UseRealAddonsDir) {
        Write-Host "提示：沙箱将沿用真实配置里的 Mod 目录（只读扫描）。"
    }
    $config.mainWindowMaximised = $false
    $config.mainWindowWidth = 1400
    $config.mainWindowHeight = 900
    $config | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath $sandboxConfigPath -Encoding UTF8
}
else {
    Write-Host "未找到真实配置（$realConfigPath），沙箱将使用默认配置。"
}

$psi = New-Object System.Diagnostics.ProcessStartInfo
$psi.FileName = $exePath
$psi.UseShellExecute = $false
$psi.WorkingDirectory = Split-Path -Parent $exePath
$psi.Environment['APPDATA'] = $SandboxRoot
$psi.Environment['LOCALAPPDATA'] = $sandboxLocal
$psi.Environment['LYTVPK_CUA_BRIDGE'] = '1'
$psi.Environment['LYTVPK_CUA_PORT'] = [string]$Port
$psi.RedirectStandardOutput = $true
$psi.RedirectStandardError = $true

$process = [System.Diagnostics.Process]::Start($psi)
$process.BeginOutputReadLine()
$process.BeginErrorReadLine()
Write-Host "started pid=$($process.Id) exe=$ExeName port=$Port sandbox=$SandboxRoot"

$deadline = (Get-Date).AddSeconds(300)
$ready = $false
$attempt = 0
while ((Get-Date) -lt $deadline) {
    $attempt++
    Start-Sleep -Seconds 2
    try {
        $ping = (Invoke-WebRequest -UseBasicParsing -NoProxy -TimeoutSec 5 "http://127.0.0.1:$Port/ping").Content
        Write-Host "ping=$ping"
        $ready = $true
        break
    }
    catch {
        if ($attempt % 15 -eq 0) {
            Write-Host "  仍在等待桥（已 $([int]($attempt * 2)) 秒）…"
        }
    }
}

if (-not $ready) {
    throw "桥未就绪（端口 $Port）。可能原因：EXE 不是 -tags cua 构建、端口被占、或单例接管。"
}
