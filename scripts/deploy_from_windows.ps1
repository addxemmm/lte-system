#Requires -Version 7
<# Export tracked working-tree source, sync without deleting private data, optionally build.
   New source files must be git-added first. Never restarts the running container.
   Usage: ./scripts/deploy_from_windows.ps1 -HostAlias TARGET [-Build]
#>
param([string]$HostAlias = "vm-sdr", [switch]$Build)
$ErrorActionPreference = "Stop"
$root = Split-Path -Parent $PSScriptRoot
$tgz = Join-Path ([IO.Path]::GetTempPath()) ("lte-source-" + [guid]::NewGuid().ToString("N") + ".tgz")
$remoteName = [IO.Path]::GetFileName($tgz)
$releaseDir = "lte-releases/" + [IO.Path]::GetFileNameWithoutExtension($tgz)
function Invoke-Checked([string]$Command, [string[]]$Arguments) {
    & $Command @Arguments
    if ($LASTEXITCODE -ne 0) { throw "$Command failed (exit $LASTEXITCODE)" }
}
try {
    Invoke-Checked python @("$PSScriptRoot/package_source.py", "--root", $root, "--output", $tgz)
    $sha = (Get-FileHash -LiteralPath $tgz -Algorithm SHA256).Hash.ToLowerInvariant()
    Invoke-Checked scp @("-o", "BatchMode=yes", $tgz, "${HostAlias}:$remoteName")
    $command = "set -eu; cd; printf '%s  %s\n' '$sha' '$remoteName' | sha256sum -c -; mkdir -p lte-releases; mkdir '$releaseDir'; tar --no-same-owner --no-same-permissions -xzf '$remoteName' -C '$releaseDir'; rm -f -- '$remoteName'; echo RELEASE_DIR=`$HOME/$releaseDir"
    Invoke-Checked ssh @("-o", "BatchMode=yes", $HostAlias, $command)
    if ($Build) {
        Invoke-Checked ssh @("-o", "BatchMode=yes", $HostAlias, "set -eu; cd ~/$releaseDir; docker compose -p docker -f deploy/docker/docker-compose.yml build")
    }
} finally {
    if (Test-Path -LiteralPath $tgz) { Remove-Item -LiteralPath $tgz }
}
