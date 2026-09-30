# 启动「副本库 + CUA 桥」的沙箱实例：**写盘类验证用这个**。
#
# 与 launch-cua-sandbox.ps1 的区别：
#   - launch-cua-sandbox.ps1 沿用**真实** Mod 目录（只读扫描）。一旦点下会写盘的按钮
#     （游戏内开关、批量启用/禁用、策略组联动、按分层应用…），改的就是你真实的
#     addonlist.txt / Mod 文件 —— 只适合做"读 DOM、量几何、抓像素"这类只读验证。
#   - 本脚本把 Mod 目录指到 .tmp-cua 下的**副本**，并且**拒绝**指向真实库的配置，
#     所以可以放心点写盘按钮。
#
# 用法：
#   pwsh -File scripts/devtools/launch-cua-copy-sandbox.ps1
#   pwsh -File scripts/devtools/launch-cua-copy-sandbox.ps1 -Port 38998 -WaitSeconds 90
#
# 副本库不存在时会自动准备：从真实库复制 addonlist.txt + 10 个最小的 VPK
# （够跑 UI 与写盘流程；要更多就自己往 -LibraryPath 里拷）。

[CmdletBinding()]
param(
    [string]$ExeName = 'LytVPK-Community-Fork-cua.exe',
    [int]$Port = 38998,
    [string]$LibraryPath = '',
    [string]$SandboxRoot = '',
    [int]$WaitSeconds = 60,
    [switch]$NoPrepare
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)

$exePath = Join-Path $repoRoot (Join-Path 'build\bin' $ExeName)
if (-not (Test-Path -LiteralPath $exePath)) {
    throw "找不到带桥的 EXE：$exePath（先运行 scripts/devtools/build-cua.ps1）"
}

$realConfigDir = Join-Path $env:APPDATA 'LytVPK'
$realConfigPath = Join-Path $realConfigDir 'config.json'
$realAddonsDir = ''
if (Test-Path -LiteralPath $realConfigPath) {
    $realConfig = Get-Content -LiteralPath $realConfigPath -Raw -Encoding UTF8 | ConvertFrom-Json
    if ($realConfig.lastActiveDirectory) { $realAddonsDir = $realConfig.lastActiveDirectory }
}

if ($SandboxRoot -eq '') {
    $SandboxRoot = Join-Path $repoRoot '.tmp-cua\zz-copy-appdata'
}
if ($LibraryPath -eq '') {
    $LibraryPath = Join-Path $repoRoot '.tmp-cua\zz-copy-lib\left4dead2\addons'
}
$LibraryPath = [System.IO.Path]::GetFullPath($LibraryPath)

# 安全闸：副本库必须落在仓库的 .tmp-cua 下，绝不能是真实 addons 目录。
$tmpRoot = [System.IO.Path]::GetFullPath((Join-Path $repoRoot '.tmp-cua'))
if (-not $LibraryPath.StartsWith($tmpRoot, [System.StringComparison]::OrdinalIgnoreCase)) {
    throw "拒绝启动：-LibraryPath 必须位于 $tmpRoot 之下（写盘验证只允许改副本），当前是 $LibraryPath"
}
if ($realAddonsDir -ne '' -and
    $LibraryPath.TrimEnd('\') -ieq ([System.IO.Path]::GetFullPath($realAddonsDir)).TrimEnd('\')) {
    throw "拒绝启动：-LibraryPath 指向真实 Mod 目录（$realAddonsDir）。写盘验证必须用副本。"
}

function Initialize-CopyLibrary {
    param([string]$Target, [string]$RealAddons)
    New-Item -ItemType Directory -Force -Path $Target | Out-Null
    if ($RealAddons -eq '' -or -not (Test-Path -LiteralPath $RealAddons)) {
        throw "副本库不存在（$Target），且拿不到真实 Mod 目录来自动准备。请手动往这个目录放几个 .vpk 与 addonlist.txt。"
    }
    # addonlist.txt 在**游戏目录**（addons 的上一级），不是在 addons 里。
    # 放错位置的表现很有迷惑性：应用找不到那份文件就新建一份空的，
    # 于是"写盘成功"但副本里原本的条目全没了 —— 看着像应用丢数据。
    $gameDir = Split-Path -Parent $RealAddons
    $realAddonList = Join-Path $gameDir 'addonlist.txt'
    $copyGameDir = Split-Path -Parent $Target
    New-Item -ItemType Directory -Force -Path $copyGameDir | Out-Null
    if (Test-Path -LiteralPath $realAddonList) {
        Copy-Item -LiteralPath $realAddonList -Destination (Join-Path $copyGameDir 'addonlist.txt') -Force
    }
    $seeds = Get-ChildItem -LiteralPath $RealAddons -Filter '*.vpk' -File |
        Sort-Object Length | Select-Object -First 10
    foreach ($seed in $seeds) {
        Copy-Item -LiteralPath $seed.FullName -Destination (Join-Path $Target $seed.Name) -Force
    }
    Write-Host "已准备副本库：$Target（$copyGameDir\addonlist.txt + $($seeds.Count) 个最小 VPK）"
}

$existingVpk = @(Get-ChildItem -LiteralPath $LibraryPath -Filter '*.vpk' -File -ErrorAction SilentlyContinue).Count
if ($existingVpk -eq 0) {
    if ($NoPrepare) {
        throw "副本库为空：$LibraryPath（去掉 -NoPrepare，或自己放入 .vpk）"
    }
    Initialize-CopyLibrary -Target $LibraryPath -RealAddons $realAddonsDir
}

# 沙箱配置：以真实配置为底，只把 Mod 目录改成副本；groups/priority 一并复制，
# 这样"策略组自动联动""按分层应用"这些会写盘的行为和真实环境一致。
$sandboxConfigDir = Join-Path $SandboxRoot 'LytVPK'
New-Item -ItemType Directory -Force -Path $sandboxConfigDir, (Join-Path $SandboxRoot 'LocalAppData') | Out-Null
if (Test-Path -LiteralPath $realConfigPath) {
    $config = Get-Content -LiteralPath $realConfigPath -Raw -Encoding UTF8 | ConvertFrom-Json
} else {
    $config = [pscustomobject]@{}
    Write-Host "未找到真实配置（$realConfigPath），用最小配置启动。"
}
$config.lastActiveDirectory = $LibraryPath
$config.defaultDirectory = $LibraryPath
$config.mainWindowMaximised = $false
$config.mainWindowWidth = 1400
$config.mainWindowHeight = 900
# 沙箱里关掉 addonlist 守卫：否则它会按真实快照"恢复"副本库的文件，干扰写盘结论。
if ($config.PSObject.Properties.Name -contains 'addonListGuardEnabled') {
    $config.addonListGuardEnabled = $false
}
$config | ConvertTo-Json -Depth 20 | Set-Content -LiteralPath (Join-Path $sandboxConfigDir 'config.json') -Encoding UTF8
foreach ($name in @('groups.json', 'priority.json')) {
    $source = Join-Path $realConfigDir $name
    if (Test-Path -LiteralPath $source) {
        Copy-Item -LiteralPath $source -Destination (Join-Path $sandboxConfigDir $name) -Force
    }
}

$stdoutLog = Join-Path ([System.IO.Path]::GetTempPath()) "lytvpk-copy-sandbox-$Port.out.log"
$stderrLog = Join-Path ([System.IO.Path]::GetTempPath()) "lytvpk-copy-sandbox-$Port.err.log"

$savedEnv = @{
    APPDATA           = $env:APPDATA
    LOCALAPPDATA      = $env:LOCALAPPDATA
    LYTVPK_CUA_BRIDGE = $env:LYTVPK_CUA_BRIDGE
    LYTVPK_CUA_PORT   = $env:LYTVPK_CUA_PORT
}
try {
    $env:APPDATA = $SandboxRoot
    $env:LOCALAPPDATA = Join-Path $SandboxRoot 'LocalAppData'
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

Write-Host "started pid=$($process.Id) port=$Port"
Write-Host "副本库   ：$LibraryPath"
Write-Host "沙箱配置 ：$sandboxConfigDir"
Write-Host "子进程日志：$stderrLog"

$deadline = (Get-Date).AddSeconds($WaitSeconds)
$ready = $false
while ((Get-Date) -lt $deadline) {
    Start-Sleep -Seconds 2
    try {
        $ping = (Invoke-WebRequest -UseBasicParsing -NoProxy -TimeoutSec 5 "http://127.0.0.1:$Port/ping").Content
        Write-Host "ping=$ping"
        $ready = $true
        break
    }
    catch {
        # 桥起来之前一直重试
    }
}
if (-not $ready) {
    if ($process.HasExited) { Write-Host "子进程已退出，ExitCode=$($process.ExitCode)" }
    Get-Content -LiteralPath $stderrLog -Encoding UTF8 -Tail 15 -ErrorAction SilentlyContinue |
        ForEach-Object { Write-Host "    $_" }
    throw "桥未就绪（端口 $Port）。最常见原因是单例接管：先关掉其它 LytVPK 实例。"
}
