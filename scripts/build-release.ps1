[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)]
    [ValidatePattern('^\d+\.\d+\.\d+(?:-[0-9A-Za-z.-]+)?$')]
    [string]$Version
)

$ErrorActionPreference = 'Stop'

$script:ExecutableName = 'LytVPK-Community-Fork.exe'
$script:AssetName = "LytVPK-Community-Fork_v$Version" + '_windows_amd64.zip'
$repoRoot = (Resolve-Path (Join-Path $PSScriptRoot '..')).Path
$releaseDir = Join-Path $repoRoot 'build\release'
$binaryPath = Join-Path $repoRoot (Join-Path 'build\bin' $script:ExecutableName)
$assetPath = Join-Path $releaseDir $script:AssetName
$stagePath = Join-Path ([System.IO.Path]::GetTempPath()) ("lytvpk-community-release-$Version-" + [guid]::NewGuid().ToString('N'))

if (Test-Path -LiteralPath $assetPath) {
    throw "Release asset already exists: $assetPath"
}

New-Item -ItemType Directory -Path $releaseDir -Force | Out-Null
New-Item -ItemType Directory -Path $stagePath | Out-Null

try {
    Push-Location $repoRoot
    try {
        wails build -m -o $script:ExecutableName -ldflags "-X main.AppVersion=$Version"
        if ($LASTEXITCODE -ne 0) {
            throw "Wails build failed with exit code $LASTEXITCODE"
        }
    }
    finally {
        Pop-Location
    }

    if (-not (Test-Path -LiteralPath $binaryPath)) {
        throw "Expected executable was not produced: $binaryPath"
    }

    Copy-Item -LiteralPath $binaryPath -Destination (Join-Path $stagePath $script:ExecutableName)
    Copy-Item -LiteralPath (Join-Path $repoRoot 'LICENSE') -Destination (Join-Path $stagePath 'LICENSE')
    Copy-Item -LiteralPath (Join-Path $repoRoot 'THIRD_PARTY_NOTICES.md') -Destination (Join-Path $stagePath 'THIRD_PARTY_NOTICES.md')
    Copy-Item -LiteralPath (Join-Path $repoRoot 'README.md') -Destination (Join-Path $stagePath 'README.md')
    Copy-Item -LiteralPath (Join-Path $repoRoot 'CHANGELOG.md') -Destination (Join-Path $stagePath 'CHANGELOG.md')

    $sourceNotice = @"
# Corresponding Source

This binary release is licensed as GPL-3.0-only. Corresponding source and build files are available from:

https://github.com/zombrain69/lytvpk/tree/v$Version

Build command:

pwsh -ExecutionPolicy Bypass -File .\scripts\build-release.ps1 -Version $Version
"@
    Set-Content -LiteralPath (Join-Path $stagePath 'SOURCE_CODE.md') -Value $sourceNotice -Encoding utf8NoBOM

    $archiveFiles = Get-ChildItem -LiteralPath $stagePath -File | Select-Object -ExpandProperty FullName
    Compress-Archive -LiteralPath $archiveFiles -DestinationPath $assetPath
    Get-FileHash -Algorithm SHA256 -LiteralPath $assetPath
    Write-Host "Release asset created: $assetPath"
}
finally {
    if (Test-Path -LiteralPath $stagePath) {
        Remove-Item -LiteralPath $stagePath -Recurse -Force
    }
}
