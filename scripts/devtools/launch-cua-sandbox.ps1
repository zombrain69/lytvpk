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

# 单例监听端口是**固定**的（internal/app/singleton.go: SingletonPort = 19527），跟 -Port 无关：
# 只要已经有任何一个实例在跑（哪怕是你自己开的正式版），新进程会连上它、转发参数、然后自己退出，
# 桥永远不会起来。以前这里只会抛一句"可能原因"，白等 5 分钟；现在先说清、失败时把日志打出来。
$singletonPort = 19527
$singletonBusy = $false
try {
    $probe = New-Object System.Net.Sockets.TcpClient
    $singletonBusy = $probe.ConnectAsync('127.0.0.1', $singletonPort).Wait(500)
    $probe.Close()
} catch {
    $singletonBusy = $false
}
if ($singletonBusy) {
    # 这种情况下新进程会 os.Exit(0)（internal/app/singleton.go: EnsureSingleton），
    # 等下去只是白等 5 分钟，直接说清楚。
    throw "127.0.0.1:$singletonPort 已经有 LytVPK 实例在跑：沙箱实例会被单例接管、立刻退出，桥永远起不来。先关掉那个实例（Get-Process *LytVPK* | Stop-Process）再跑本脚本。"
}

$stdoutLog = Join-Path ([System.IO.Path]::GetTempPath()) "lytvpk-cua-sandbox-$Port.out.log"
$stderrLog = Join-Path ([System.IO.Path]::GetTempPath()) "lytvpk-cua-sandbox-$Port.err.log"

# 用 Start-Process 而不是 ProcessStartInfo：它能把子进程 stdout/stderr **边跑边写**进文件，
# 失败时能看到它到底卡在哪一步（ProcessStartInfo + ReadToEndAsync 在进程不退出时读不到任何东西）。
# 代价是环境变量只能改当前会话的，所以改完立刻在 finally 里还原。
$savedEnv = @{
    APPDATA           = $env:APPDATA
    LOCALAPPDATA      = $env:LOCALAPPDATA
    LYTVPK_CUA_BRIDGE = $env:LYTVPK_CUA_BRIDGE
    LYTVPK_CUA_PORT   = $env:LYTVPK_CUA_PORT
}
try {
    $env:APPDATA = $SandboxRoot
    $env:LOCALAPPDATA = $sandboxLocal
    $env:LYTVPK_CUA_BRIDGE = '1'
    $env:LYTVPK_CUA_PORT = [string]$Port
    $process = Start-Process -FilePath $exePath -PassThru -NoNewWindow `
        -RedirectStandardOutput $stdoutLog -RedirectStandardError $stderrLog
}
finally {
    $env:APPDATA = $savedEnv.APPDATA
    $env:LOCALAPPDATA = $savedEnv.LOCALAPPDATA
    $env:LYTVPK_CUA_BRIDGE = $savedEnv.LYTVPK_CUA_BRIDGE
    $env:LYTVPK_CUA_PORT = $savedEnv.LYTVPK_CUA_PORT
}
Write-Host "started pid=$($process.Id) exe=$ExeName port=$Port sandbox=$SandboxRoot"
Write-Host "日志：$stderrLog"

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
    Write-Host "--- 子进程是否已退出 ---"
    if ($process.HasExited) {
        Write-Host "已退出，ExitCode=$($process.ExitCode)"
    } else {
        Write-Host "仍在运行 pid=$($process.Id)"
    }
    foreach ($log in @($stderrLog, $stdoutLog)) {
        if (Test-Path -LiteralPath $log) {
            Write-Host "--- $log（最后 15 行）---"
            Get-Content -LiteralPath $log -Encoding UTF8 -Tail 15 | ForEach-Object { Write-Host "    $_" }
        }
    }
    $hint = if ($singletonBusy) {
        "127.0.0.1:$singletonPort 上有别的实例（单例接管）：先关掉它。"
    } else {
        "EXE 不是 -tags cua 构建（用 scripts/devtools/build-cua.ps1）、或端口 $Port 被占。"
    }
    throw "桥未就绪（端口 $Port）。$hint"
}
