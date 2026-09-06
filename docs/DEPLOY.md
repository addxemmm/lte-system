# 部署与回滚 / Deployment and rollback

开发机仅编辑、测试与 Git 操作；所有 Docker 构建和运行均在 SDR 服务器执行。
Development machines only edit, test and manage Git. Run all Docker builds and containers on the SDR server.

## 1. 同步源代码 / Synchronize source

部署脚本只打包 **Git 已跟踪文件的工作区内容**；新文件须先 `git add`，未暂存的已跟踪修改会包含在内。
Deployment scripts export **working-tree contents of Git-tracked files**. Stage new files with `git add`; unstaged changes to tracked files are included.

- 排除兄弟项目、未跟踪文件、真实卡库/字典、私有 `configs/app.yaml`、`.env*` 和运行产物；保留 `.example` 和固件。
  Exclude sibling projects, untracked files, live subscriber databases/wordlists, private `configs/app.yaml`, `.env*` and runtime output; retain examples and firmware.
- 源码在服务器 `~/lte-releases/` 内解压到全新目录，脚本输出 `RELEASE_DIR`；SHA-256 验证后解压，不覆盖旧目录或保留已删除源码。
  Source is extracted into a fresh directory under `~/lte-releases/`; scripts print `RELEASE_DIR`. SHA-256 is verified first. Old directories stay untouched, and deleted source cannot linger.
- Python 3.9+、Git、SSH/SCP 为开发机前置；远端用户须有 Docker 权限。脚本任何命令失败均停止。
  The development host needs Python 3.9+, Git and SSH/SCP; the remote user needs Docker access. Every command failure stops deployment.

```powershell
# Windows: only sync by default; -Build also builds on the server.
./scripts/deploy_from_windows.ps1 -HostAlias TARGET -Build
```
```bash
# Linux: sync and build on the server, without restarting the running container.
bash scripts/deploy_to_ubuntu.sh addx@TARGET
```

两个脚本都不重建容器、不启动小区、不停止现有小区。替换容器请执行下面的备份/发布流程。
Neither script recreates containers, starts radio or stops an existing cell. Use the backup/release procedure below to replace a container.

## 2. API 监听与令牌 / API listener and token

Compose 默认监听 `127.0.0.1:8081`，只允许服务器本地或 SSH 隧道访问；这是相对于旧版全网卡监听的有意变更。
Compose now defaults to `127.0.0.1:8081`, reachable locally or through SSH tunneling. This intentionally changes the previous all-interface default.

```bash
# Run on your client; then use http://127.0.0.1:8081 locally.
ssh -L 8081:127.0.0.1:8081 addx@TARGET
```

需要局域网 API 时，在服务器私有 `.env` 内设置以下值，并使用 `--env-file /absolute/path/.env`；不要提交真实令牌。
For LAN API access, put these values in a private server `.env` and use `--env-file /absolute/path/.env`; never commit real tokens.

```dotenv
LTE_LISTEN=0.0.0.0:8081
LTE_API_TOKEN=TOKEN
LTE_IMAGE=ltesystem-dep:RELEASE
```

`TOKEN` 必须替换为随机强令牌；所有 API 请求均携带 `Authorization: Bearer TOKEN`。host 网络不受 Docker 端口映射限制；使用主机防火墙限制管理网来源，跨不可信网络使用 TLS 代理或 SSH。
Replace `TOKEN` with a strong random token. Every API request needs `Authorization: Bearer TOKEN`. Host networking bypasses Docker port mappings; restrict management sources with the host firewall and use TLS or SSH across untrusted networks.

默认 Compose 未设置令牌时仍支持回环访问；不要将空令牌和全网卡监听组合使用。普通 `docker run` 不读取 Compose 的回环默认值，必须显式设置 `LTE_LISTEN`。
An empty token is supported for default loopback access. Do not combine an empty token with an all-interface listener. Plain `docker run` does not inherit Compose defaults: explicitly set `LTE_LISTEN`.

## 3. 识别现有卷并备份 / Identify and back up the existing volume

`lte-data` 是 Compose 的逻辑名，实际名称通常为 `docker_lte-data`，但会随项目名变化。必须从现有容器读取名称；直接挂载裸 `lte-data` 可能创建一个空卷。
`lte-data` is a Compose logical name. The actual name is often `docker_lte-data`, but depends on the project name. Read it from the existing container; mounting bare `lte-data` can silently create an empty volume.

以下命令在脚本输出的服务器源码快照根目录执行；停机前确认没有在途写卡/上传。新部署没有旧容器时跳过旧实例备份。
Run these commands at the server source snapshot root printed by the script. Ensure there are no in-flight SIM/upload operations before stopping. Skip old-instance backup on a first deployment.

```bash
set -euo pipefail
umask 077
RELEASE=$(date -u +%Y%m%dT%H%M%SZ)
OLD_IMAGE=$(docker inspect ltesystem --format '{{.Image}}')
PROJECT=$(docker inspect ltesystem --format '{{index .Config.Labels "com.docker.compose.project"}}')
DATA_VOLUME=$(docker inspect ltesystem --format '{{range .Mounts}}{{if and (eq .Destination "/data") (eq .Type "volume")}}{{.Name}}{{end}}{{end}}')
test -n "$PROJECT" && test -n "$DATA_VOLUME"
docker volume inspect "$DATA_VOLUME" >/dev/null
BACKUP_DIR="$HOME/lte-backups/$RELEASE"
mkdir -p "$BACKUP_DIR"; chmod 700 "$BACKUP_DIR"
printf '%s\n' "$OLD_IMAGE" > "$BACKUP_DIR/image-id.txt"
printf '%s\n' "$PROJECT" > "$BACKUP_DIR/project.txt"
printf '%s\n' "$DATA_VOLUME" > "$BACKUP_DIR/volume.txt"
docker tag "$OLD_IMAGE" "ltesystem-dep:rollback-$RELEASE"
docker stop --time 210 ltesystem
docker run --rm --network none --read-only --entrypoint tar \
  --mount "type=volume,src=$DATA_VOLUME,dst=/data,readonly" \
  --mount "type=bind,src=$BACKUP_DIR,dst=/backup" \
  "$OLD_IMAGE" czf /backup/data.tgz -C /data .
tar tzf "$BACKUP_DIR/data.tgz" >/dev/null
sha256sum "$BACKUP_DIR/data.tgz" > "$BACKUP_DIR/data.tgz.sha256"
```

备份失败则不替换容器，使用 `docker start ltesystem` 恢复 API。备份包含私有卡库，保持权限受限，不上传 GitHub。
If backup fails, do not replace the container; restore API availability with `docker start ltesystem`. Backups contain private subscriber data: keep restricted permissions and never upload them to GitHub.

## 4. 构建、替换与验证 / Build, replace and verify

为减少停机，可先构建、再执行第 3 节停机备份。使用唯一镜像标签，并保持之前的 Compose 项目名和卷名。
To minimize downtime, build before the stopped backup in section 3. Use a unique image tag and retain the original Compose project and volume names.

```bash
# If this is a Git checkout, update only with: git pull --ff-only
# Snapshot deployments do not require a remote Git checkout.
export LTE_IMAGE="ltesystem-dep:$RELEASE"
docker compose -p "$PROJECT" -f deploy/docker/docker-compose.yml config --quiet
docker compose -p "$PROJECT" -f deploy/docker/docker-compose.yml build
# Add --env-file .env to each compose command when using private configuration.
docker compose -p "$PROJECT" -f deploy/docker/docker-compose.yml up -d --no-build --force-recreate
ACTUAL_VOLUME=$(docker inspect ltesystem --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}')
test "$ACTUAL_VOLUME" = "$DATA_VOLUME"
BASE=http://127.0.0.1:8081 bash scripts/smoke.sh
docker logs --tail 30 ltesystem
```

冒烟仅读取 `/api/v1/cell`、`/api/v1/health` 并断言 HTTP 成功、`code=0` 和请求 ID。设置鉴权时同时给 smoke 进程设置 `LTE_API_TOKEN`。它不调用 start/stop、写卡、上传、抓包或破解。
Smoke checks only read `/api/v1/cell` and `/api/v1/health`, asserting HTTP success, `code=0` and request IDs. Pass `LTE_API_TOKEN` to the smoke process when authentication is enabled. It never starts/stops radio, programs SIMs, uploads, captures or cracks.

容器启动只启动 API，不自动启动 LTE 小区；硬件入网测试另行执行。SIGTERM 时 API 最多排空 200 秒，Compose 等待 210 秒，覆盖 180 秒写卡请求。
Container startup starts the API only, not the LTE cell. Hardware attach validation is separate. API SIGTERM draining allows 200 seconds and Compose waits 210 seconds, covering 180-second SIM requests.

## 5. 回滚 / Rollback

```bash
export LTE_IMAGE="ltesystem-dep:rollback-$RELEASE"
docker compose -p "$PROJECT" -f deploy/docker/docker-compose.yml up -d --no-build --force-recreate
BASE=http://127.0.0.1:8081 bash scripts/smoke.sh
```

回滚镜像时保持同一个数据卷。只有确需恢复数据且服务已停止时才解包备份；先校验 SHA-256，恢复会覆盖同名文件。不要 `docker compose down -v` 或删除备份/旧镜像。
Keep the same data volume on image rollback. Restore data only when necessary and while the service is stopped; verify SHA-256 first, and note extraction overwrites matching files. Do not run `docker compose down -v` or delete backups/old images.

## 6. 配置与已知边界 / Configuration and known limits

- `UHD_FPGA=compat` 用于 BlackSDR 兼容板，正版 B210 使用 `stock`。固件选择不等于硬件已验证。
  Use `UHD_FPGA=compat` for BlackSDR clones and `stock` for genuine B210. Firmware selection is not hardware validation.
- `sib.conf`/`rb.conf` 启动时随镜像覆盖；卡库与字典仅缺失时播种；`rr.conf` 在启动小区时渲染。备份包含这些文件。
  `sib.conf`/`rb.conf` follow the image at startup; database/wordlist seeding only fills missing files; `rr.conf` is rendered at cell start. Backups contain these files.
- 用户库修改后需要重启小区才被 EPC 读取；应在小区停止时修改，进程内互斥不覆盖外部 EPC 的文件写回。
  Subscriber changes require a cell restart for EPC to load them. Edit while the cell is stopped: process-local locks do not coordinate external EPC file writes.
- Ubuntu 基础镜像、系统/Python 依赖尚非完全内容寻址锁定；本轮保留原 srsRAN 与 pySIM 版本，完整供应链升级应另做兼容性验证。
  Base images and system/Python dependencies are not fully content-pinned. This audit retains the existing srsRAN and pySIM versions; a complete supply-chain upgrade needs separate compatibility validation.

---
**导航 Navigation:** [文档索引 Docs](README.md) · [API](API.md) · [Docker](../deploy/docker/README.md) · [MIGRATION](MIGRATION.md)
