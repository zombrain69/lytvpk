# 只读备份当前 Mod 列表状态（addonlist.txt + 应用备份 + 本地记录 + Mod 清单）。
# 不复制、不移动、不修改任何 Mod 文件；用于调试 / 验证前留一份可恢复快照。
#
# 用法：
#   pwsh -File scripts/devtools/backup-modlist.ps1
#   pwsh -File scripts/devtools/backup-modlist.ps1 -GameDir 'D:\Steam\...\left4dead2' -BackupRoot 'D:\backups'

param(
    [string]$GameDir = 'E:\SteamLibrary\steamapps\common\Left 4 Dead 2\left4dead2',
    [string]$BackupRoot = 'E:\SteamLibrary\steamapps\common\Left 4 Dead 2\program\modlist-backups',
    [string]$ConfigDir = "$env:APPDATA\LytVPK"
)

$ErrorActionPreference = 'Stop'
$stamp = Get-Date -Format 'yyyyMMdd-HHmmss'
$target = Join-Path $BackupRoot $stamp
New-Item -ItemType Directory -Force -Path $target | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $target 'config') | Out-Null
New-Item -ItemType Directory -Force -Path (Join-Path $target 'app-addonlist-backups') | Out-Null

$addonList = Join-Path $GameDir 'addonlist.txt'
if (-not (Test-Path -LiteralPath $addonList)) { throw "addonlist.txt not found: $addonList" }
Copy-Item -LiteralPath $addonList -Destination (Join-Path $target 'addonlist.txt')

$appBackupDir = "$addonList.lytvpk-backups"
if (Test-Path -LiteralPath $appBackupDir) {
    Get-ChildItem -LiteralPath $appBackupDir -File -Force |
        ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $target 'app-addonlist-backups') }
}

if (Test-Path -LiteralPath $ConfigDir) {
    Get-ChildItem -LiteralPath $ConfigDir -File -Force -Filter *.json |
        ForEach-Object { Copy-Item -LiteralPath $_.FullName -Destination (Join-Path $target 'config') }
}

# Mod 清单快照（只读列举，不复制、不修改任何 Mod 文件）
$inventory = Join-Path $target 'addons-inventory.txt'
$addonsRoot = Join-Path $GameDir 'addons'
"# generated $stamp`n# root: $addonsRoot`n# relative_path`tsize_bytes`tlast_write`n" | Set-Content -LiteralPath $inventory -Encoding utf8
Get-ChildItem -LiteralPath $addonsRoot -Recurse -File -Force -Filter *.vpk |
    ForEach-Object {
        $rel = $_.FullName.Substring($addonsRoot.Length).TrimStart('\')
        "$rel`t$($_.Length)`t$($_.LastWriteTime.ToString('s'))"
    } | Add-Content -LiteralPath $inventory -Encoding utf8

# 校验用清单：源文件与副本的 sha256
$manifest = Join-Path $target 'manifest.sha256.txt'
$pairs = @()
$pairs += ,@((Join-Path $target 'addonlist.txt'), $addonList)
Get-ChildItem -LiteralPath (Join-Path $target 'app-addonlist-backups') -File -Force |
    ForEach-Object { $pairs += ,@($_.FullName, (Join-Path $appBackupDir $_.Name)) }
Get-ChildItem -LiteralPath (Join-Path $target 'config') -File -Force |
    ForEach-Object { $pairs += ,@($_.FullName, (Join-Path $ConfigDir $_.Name)) }

$mismatch = 0
$manifestLines = foreach ($pair in $pairs) {
    $copyHash = (Get-FileHash -LiteralPath $pair[0] -Algorithm SHA256).Hash
    $srcHash = (Get-FileHash -LiteralPath $pair[1] -Algorithm SHA256).Hash
    $ok = $copyHash -eq $srcHash
    if (-not $ok) { $mismatch++ }
    $rel = $pair[0].Substring($target.Length).TrimStart('\')
    "$($copyHash.ToLowerInvariant())  $rel  src_ok=$ok"
}
$manifestLines | Set-Content -LiteralPath $manifest -Encoding ascii

$addonListHash = (Get-FileHash -LiteralPath $addonList -Algorithm SHA256).Hash.ToLowerInvariant()
$lineCount = (Get-Content -LiteralPath $addonList).Count
@(
    "backup of the current mod list (read-only copy)"
    "created: $stamp"
    "source addonlist: $addonList"
    "sha256: $addonListHash"
    "lines: $lineCount"
    "copied: addonlist.txt, app-addonlist-backups/* ($((Get-ChildItem -LiteralPath (Join-Path $target 'app-addonlist-backups') -File).Count) files), config/*.json"
    "inventory: addons-inventory.txt (relative path, size, mtime of every .vpk under addons/)"
    "no mod file was created, moved, modified or deleted"
)
| Set-Content -LiteralPath (Join-Path $target 'README.txt') -Encoding utf8

"backup dir: $target"
"addonlist sha256: $addonListHash"
"addonlist lines: $lineCount"
"hash mismatches: $mismatch"
"files: $((Get-ChildItem -LiteralPath $target -Recurse -File).Count)"
