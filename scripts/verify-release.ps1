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
}
finally {
    $archive.Dispose()
}

Write-Host "Verified canonical release archive: $resolvedArchive"
