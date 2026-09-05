#Requires -Version 7
<# deploy_from_windows.ps1 — Windows 本地把代码同步到 Ubuntu 服务器并构建镜像。
用法: .\scripts\deploy_from_windows.ps1 [-HostAlias vm-sdr] [-Build]
  默认只同步；加 -Build 则在远端执行 compose build。
前置: ~/.ssh/config 里有 Host vm-sdr（密钥登录），见 AGENTS.md/DEPLOY.md。
注意: 不重建容器（避免中断在线小区）；要重建在服务器跑 compose up -d --force-recreate。
#>
param([string]$HostAlias = "vm-sdr", [switch]$Build)

$ErrorActionPreference = "Stop"
$root = Split-Path -Parent (Split-Path -Parent $PSScriptRoot)
$tgz = Join-Path ([System.IO.Path]::GetTempPath()) "lte-system.tgz"

Push-Location $root
try {
  tar --exclude=.git --exclude=bin --exclude=var -czf $tgz . | Out-Null
  scp -o BatchMode=yes $tgz "${HostAlias}:~/lte-system.tgz"
  ssh -o BatchMode=yes $HostAlias "mkdir -p ~/lte-system && tar -xzf ~/lte-system.tgz -C ~/lte-system && echo SYNCED"
  if ($Build) {
    ssh -o BatchMode=yes $HostAlias "cd ~/lte-system && docker compose -f deploy/docker/docker-compose.yml build 2>&1 | tail -1"
  }
} finally {
  Pop-Location
  Remove-Item $tgz -ErrorAction SilentlyContinue
}
