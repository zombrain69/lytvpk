[CmdletBinding()]
param(
    [ValidateSet('Commit', 'Push', 'Release')]
    [string]$Mode = 'Release',

    [string]$RemoteName,

    [string]$RemoteUrl,

    [string]$PushRefsFile
)

$ErrorActionPreference = 'Stop'

$expectedName = 'zombrain69'
$expectedEmail = '321279816+zombrain69@users.noreply.github.com'
$expectedOrigin = 'https://github.com/zombrain69/lytvpk.git'
$expectedUpstream = 'https://github.com/LaoYutang/lytvpk.git'

function Get-GitValue {
    param(
        [Parameter(Mandatory = $true)]
        [string[]]$Arguments,

        [Parameter(Mandatory = $true)]
        [string]$Description
    )

    $value = & git @Arguments 2>$null
    if ($LASTEXITCODE -ne 0) {
        throw "Unable to read $Description."
    }
    return ([string]$value).Trim()
}

function Assert-ExactValue {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Actual,

        [Parameter(Mandatory = $true)]
        [string]$Expected,

        [Parameter(Mandatory = $true)]
        [string]$Description
    )

    if ($Actual -cne $Expected) {
        throw "$Description must be '$Expected', but is '$Actual'. Refusing to continue."
    }
}

function Assert-CommitIdentity {
    param(
        [Parameter(Mandatory = $true)]
        [string]$Commit
    )

    $identity = Get-GitValue -Arguments @('show', '-s', '--format=%an|%ae|%cn|%ce', $Commit) -Description "commit identity for $Commit"
    $parts = $identity.Split('|')
    if ($parts.Count -ne 4 -or $parts[0] -cne $expectedName -or $parts[1] -cne $expectedEmail -or $parts[2] -cne $expectedName -or $parts[3] -cne $expectedEmail) {
        throw "Commit $Commit is not authored and committed by $expectedName <$expectedEmail>. Refusing to publish it."
    }
}

$repoRoot = Get-GitValue -Arguments @('rev-parse', '--show-toplevel') -Description 'repository root'
Push-Location $repoRoot
try {
    Assert-ExactValue -Actual (Get-GitValue -Arguments @('config', '--local', '--get', 'user.name') -Description 'local Git user.name') -Expected $expectedName -Description 'Local Git user.name'
    Assert-ExactValue -Actual (Get-GitValue -Arguments @('config', '--local', '--get', 'user.email') -Description 'local Git user.email') -Expected $expectedEmail -Description 'Local Git user.email'
    Assert-ExactValue -Actual (Get-GitValue -Arguments @('config', '--local', '--get', 'user.useConfigOnly') -Description 'local Git user.useConfigOnly') -Expected 'true' -Description 'Local Git user.useConfigOnly'
    Assert-ExactValue -Actual (Get-GitValue -Arguments @('remote', 'get-url', 'origin') -Description 'origin URL') -Expected $expectedOrigin -Description 'origin URL'
    Assert-ExactValue -Actual (Get-GitValue -Arguments @('remote', 'get-url', 'upstream') -Description 'upstream URL') -Expected $expectedUpstream -Description 'upstream URL'

    $authorIdentity = Get-GitValue -Arguments @('var', 'GIT_AUTHOR_IDENT') -Description 'effective Git author identity'
    $committerIdentity = Get-GitValue -Arguments @('var', 'GIT_COMMITTER_IDENT') -Description 'effective Git committer identity'
    $expectedIdentityPrefix = "$expectedName <$expectedEmail>"
    if (-not $authorIdentity.StartsWith($expectedIdentityPrefix, [System.StringComparison]::Ordinal) -or -not $committerIdentity.StartsWith($expectedIdentityPrefix, [System.StringComparison]::Ordinal)) {
        throw "The effective Git author/committer identity is not $expectedIdentityPrefix. Refusing to continue."
    }

    if ($Mode -eq 'Push') {
        Assert-ExactValue -Actual $RemoteName -Expected 'origin' -Description 'push remote'
        Assert-ExactValue -Actual $RemoteUrl -Expected $expectedOrigin -Description 'push remote URL'

        if ($PushRefsFile -and (Test-Path -LiteralPath $PushRefsFile)) {
            foreach ($line in Get-Content -LiteralPath $PushRefsFile -Encoding utf8) {
                if ([string]::IsNullOrWhiteSpace($line)) {
                    continue
                }

                $parts = $line -split ' '
                if ($parts.Count -lt 4 -or $parts[0] -eq '(delete)') {
                    continue
                }

                $localRef = $parts[0]
                $localObject = $parts[1]
                $remoteObject = $parts[3]

                if ($localRef -like 'refs/tags/v*') {
                    $tagIdentity = Get-GitValue -Arguments @('for-each-ref', '--format=%(objecttype)|%(taggername)|%(taggeremail)', $localRef) -Description "tag identity for $localRef"
                    Assert-ExactValue -Actual $tagIdentity -Expected "tag|$expectedName|<$expectedEmail>" -Description "annotated release tag $localRef"
                    $taggedCommit = Get-GitValue -Arguments @('rev-parse', "$localObject^{commit}") -Description "tagged commit for $localRef"
                    Assert-CommitIdentity -Commit $taggedCommit
                }

                $objectType = Get-GitValue -Arguments @('cat-file', '-t', $localObject) -Description "object type for $localRef"
                if ($objectType -eq 'tag') {
                    $localObject = Get-GitValue -Arguments @('rev-parse', "$localObject^{commit}") -Description "tagged commit for $localRef"
                    $objectType = 'commit'
                }
                if ($objectType -ne 'commit') {
                    continue
                }

                $commits = if ($remoteObject -match '^0+$') {
                    & git rev-list $localObject --not --remotes=origin
                }
                else {
                    & git rev-list "$remoteObject..$localObject"
                }
                if ($LASTEXITCODE -ne 0) {
                    throw "Unable to inspect commits that would be pushed for $localRef."
                }

                foreach ($commit in $commits) {
                    Assert-CommitIdentity -Commit $commit
                }
            }
        }
    }

    Write-Host "Identity guard passed for ${Mode}: $expectedName <$expectedEmail>"
}
finally {
    Pop-Location
}
