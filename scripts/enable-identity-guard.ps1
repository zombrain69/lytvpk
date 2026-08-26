[CmdletBinding()]
param()

$ErrorActionPreference = 'Stop'

$repoRoot = (& git rev-parse --show-toplevel).Trim()
if ($LASTEXITCODE -ne 0) {
    throw 'Run this script from inside the LytVPK repository.'
}

Push-Location $repoRoot
try {
    git config --local user.name 'zombrain69'
    git config --local user.email '321279816+zombrain69@users.noreply.github.com'
    git config --local user.useConfigOnly true
    git config --local core.hooksPath .githooks

    & (Join-Path $repoRoot 'scripts\verify-publish-identity.ps1') -Mode Commit
    if ($LASTEXITCODE -ne 0) {
        throw 'Identity guard verification failed.'
    }

    Write-Host 'Identity guard is enabled for this clone.'
}
finally {
    Pop-Location
}
