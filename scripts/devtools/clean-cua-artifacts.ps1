# 清理 CUA 调试留下的临时物（走回收站，可恢复）。
#
# 调试产物只允许落在这三处，本脚本就清这三处：
#   1) .tmp-cua/                              沙箱 APPDATA、副本库、探针脚本、报告 JSON
#   2) build\bin\*-cua.exe                    带桥调试 EXE（发布产物不含桥，随时可重建）
#   3) %TEMP%\lytvpk-cua-*.log 等             桥与沙箱的子进程日志
#
# 用法：
#   pwsh -File scripts/devtools/clean-cua-artifacts.ps1            # 真删（回收站）
#   pwsh -File scripts/devtools/clean-cua-artifacts.ps1 -DryRun    # 只看会删什么
#   pwsh -File scripts/devtools/clean-cua-artifacts.ps1 -KeepDebugExe
#
# 重建方式：
#   pwsh -File scripts/devtools/build-cua.ps1                 # 带桥调试 EXE
#   pwsh -File scripts/devtools/launch-cua-sandbox.ps1        # 只读沙箱（配置每次自动复制）
#   pwsh -File scripts/devtools/launch-cua-copy-sandbox.ps1   # 副本库沙箱（库不存在会自动准备）

[CmdletBinding()]
param(
    [switch]$DryRun,
    [switch]$KeepDebugExe
)

$ErrorActionPreference = 'Stop'
$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)

Add-Type -AssemblyName Microsoft.VisualBasic

function Remove-ToRecycleBin {
    param([string]$Path, [switch]$Directory)
    if (-not (Test-Path -LiteralPath $Path)) {
        return $false
    }
    if ($DryRun) {
        Write-Host "  [DryRun] 会删除：$Path"
        return $true
    }
    try {
        if ($Directory) {
            [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteDirectory(
                $Path, 'OnlyErrorDialogs', 'SendToRecycleBin')
        } else {
            [Microsoft.VisualBasic.FileIO.FileSystem]::DeleteFile(
                $Path, 'OnlyErrorDialogs', 'SendToRecycleBin')
        }
        Write-Host "  已移入回收站：$Path"
        return $true
    } catch {
        # 回收站 API 对"整棵目录"会失败（真机两种情况：路径过长 → "指定的路径无效"；
        # 里面有文件被别的进程（例如另一个并行会话的沙箱）占用 → "being used by another process"）。
        # 这里**只报告、不硬删**：跨盘 Move-Item 是"先复制再删除"，失败会留下半份副本，
        # 比不删更乱；而且并行会话可能正在用里面的夹具。
        Write-Warning "未能删除（$Path）：$($_.Exception.Message)"
        Write-Warning "  它可能被其它进程占用（例如并行会话的沙箱），或路径过长回收站放不下；请手动确认后再删。"
        return $false
    }
}

Write-Host "清理 CUA 调试临时物（仓库：$repoRoot）"

$tmpRoot = Join-Path $repoRoot '.tmp-cua'
Remove-ToRecycleBin -Path $tmpRoot -Directory | Out-Null

if (-not $KeepDebugExe) {
    $binDir = Join-Path $repoRoot 'build\bin'
    if (Test-Path -LiteralPath $binDir) {
        Get-ChildItem -LiteralPath $binDir -Filter '*-cua.exe' -File | ForEach-Object {
            Remove-ToRecycleBin -Path $_.FullName | Out-Null
        }
    }
}

$tempPatterns = @(
    'lytvpk-cua-bridge.log',
    'lytvpk-cua-sandbox-*.out.log',
    'lytvpk-cua-sandbox-*.err.log',
    'lytvpk-copy-sandbox-*.out.log',
    'lytvpk-copy-sandbox-*.err.log',
    'lytvpk-test-sandbox-*.out.log',
    'lytvpk-test-sandbox-*.err.log',
    'lytvpk-cdp-*.out.log',
    'lytvpk-cdp-*.err.log'
)
foreach ($pattern in $tempPatterns) {
    Get-ChildItem -LiteralPath ([System.IO.Path]::GetTempPath()) -Filter $pattern -File -ErrorAction SilentlyContinue |
        ForEach-Object { Remove-ToRecycleBin -Path $_.FullName | Out-Null }
}

Write-Host '完成。需要时用 build-cua.ps1 重建调试 EXE，沙箱目录会在下次启动时自动准备。'
