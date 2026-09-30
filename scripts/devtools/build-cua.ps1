# 构建"带 CUA 调试桥"的 EXE，专门用于 UI 实机断言与目视。
#
# 默认构建（`wails build`）与发布构建（`scripts/build-release.ps1`）**都不含**这个桥：
# 桥的实现在 internal/app/cua_bridge_cua.go（`//go:build cua`），
# 默认构建命中的是 cua_bridge_stub.go 里的空实现。
#
# 用法：
#   pwsh -File scripts/devtools/build-cua.ps1
#   pwsh -File scripts/devtools/build-cua.ps1 -OutName LytVPK-Community-Fork-cua.exe -Version 2.7.1-community.42-uicheck

[CmdletBinding()]
param(
    [string]$OutName = 'LytVPK-Community-Fork-cua.exe',
    [string]$Version = ''
)

$ErrorActionPreference = 'Stop'

$repoRoot = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
Push-Location $repoRoot
try {
    $arguments = @('build', '-m', '-tags', 'cua', '-o', $OutName)
    if ($Version -ne '') {
        $arguments += @('-ldflags', "-X main.AppVersion=$Version")
    }

    & wails @arguments
    if ($LASTEXITCODE -ne 0) {
        throw "wails build -tags cua failed with exit code $LASTEXITCODE"
    }

    $binary = Join-Path $repoRoot (Join-Path 'build\bin' $OutName)
    if (-not (Test-Path -LiteralPath $binary)) {
        throw "Expected executable was not produced: $binary"
    }

    # 自检：这个产物必须**能**被发布门禁识别出桥标记（否则标签没生效）。
    $text = [System.Text.Encoding]::ASCII.GetString([System.IO.File]::ReadAllBytes($binary))
    if (-not $text.Contains('LYTVPK_CUA_BRIDGE')) {
        throw "构建产物里没有桥标记，说明 -tags cua 没生效：$binary"
    }

    [pscustomobject]@{
        File        = $binary
        Size        = (Get-Item -LiteralPath $binary).Length
        HasBridge   = $true
        NextStep    = "pwsh -File scripts/devtools/launch-cua-sandbox.ps1 -ExeName $OutName"
    } | Format-List
}
finally {
    Pop-Location
}
