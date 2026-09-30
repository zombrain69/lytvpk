[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$')]
    [string]$Version,

    [Parameter(Mandatory = $true)]
    [string]$ArchivePath
)

$ErrorActionPreference = 'Stop'

$expectedExecutable = 'LytVPK-Community-Fork.exe'
$expectedArchive = "LytVPK-Community-Fork_v$Version" + '_windows_amd64.zip'
$requiredEntries = @(
    $expectedExecutable,
    'LICENSE',
    'THIRD_PARTY_NOTICES.md',
    'README.md',
    'CHANGELOG.md',
    'SOURCE_CODE.md'
)

if (-not (Test-Path -LiteralPath $ArchivePath -PathType Leaf)) {
    throw "Release archive was not found: $ArchivePath"
}

$resolvedArchive = (Resolve-Path -LiteralPath $ArchivePath).Path
if ([System.IO.Path]::GetFileName($resolvedArchive) -cne $expectedArchive) {
    throw "Release archive must be named $expectedArchive"
}

Add-Type -AssemblyName System.IO.Compression.FileSystem
$archive = [System.IO.Compression.ZipFile]::OpenRead($resolvedArchive)
try {
    $entries = @(
        $archive.Entries |
            Where-Object { -not $_.FullName.EndsWith('/') } |
            ForEach-Object { $_.FullName.Replace('\', '/') }
    )

    $missing = @($requiredEntries | Where-Object { $_ -notin $entries })
    $unexpected = @($entries | Where-Object { $_ -notin $requiredEntries })
    $executables = @($entries | Where-Object { [System.IO.Path]::GetExtension($_) -ieq '.exe' })

    if ($missing.Count -gt 0) {
        throw "Release archive is missing required entries: $($missing -join ', ')"
    }
    if ($unexpected.Count -gt 0) {
        throw "Release archive contains unexpected entries: $($unexpected -join ', ')"
    }
    if ($executables.Count -ne 1 -or $executables[0] -cne $expectedExecutable) {
        throw "Release archive must contain exactly one executable named $expectedExecutable"
    }

    # CUA 调试桥（internal/app/cua_bridge_cua.go，只在 `-tags cua` 构建里存在）绝不能进发布产物：
    # 它是一个"能在应用里执行任意 JS"的本地 HTTP 端点，属于不该分发的攻击面。
    # 这里做最后一道保险：直接扫归档内 EXE 的字节。
    $cuaMarkers = @('LYTVPK_CUA_BRIDGE', 'cua bridge listening', 'cua_bridge_cua.go')
    $entryStream = $archive.GetEntry($expectedExecutable).Open()
    try {
        $reader = New-Object System.IO.StreamReader($entryStream, [System.Text.Encoding]::ASCII, $false)
        try {
            $executableText = $reader.ReadToEnd()
        }
        finally {
            $reader.Dispose()
        }
    }
    finally {
        $entryStream.Dispose()
    }
    foreach ($marker in $cuaMarkers) {
        if ($executableText.Contains($marker)) {
            throw "发布产物里含 CUA 调试桥标记 '$marker'：请用不带 -tags cua 的构建重新打包。"
        }
    }
}
finally {
    $archive.Dispose()
}

Write-Host "Verified canonical release archive: $resolvedArchive"
