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

Compose 默认监听 `0.0.0.0:8081`，支持直接通过服务器 IP 访问。客户端使用 `http://HOST:8081`，`0.0.0.0` 是监听地址而非客户端目标。
Compose defaults to `0.0.0.0:8081` for direct access using the server IP. Clients use `http://HOST:8081`; `0.0.0.0` is a bind address, not the client destination.

```bash
# Optional SSH access. For loopback-only publishing:
# host: LTE_LISTEN=127.0.0.1:8081; bridge: LTE_BIND_ADDR=127.0.0.1
ssh -L 8081:127.0.0.1:8081 addx@TARGET
```

需要自定义监听、启用令牌或固定镜像时，在服务器私有 `.env` 内设置以下值，并使用 `--env-file /absolute/path/.env`；不要提交真实令牌。
To customize the listener, enable a token or pin the image, put these values in a private server `.env` and use `--env-file /absolute/path/.env`; never commit real tokens.

```dotenv
LTE_LISTEN=0.0.0.0:8081
LTE_API_TOKEN=TOKEN
LTE_IMAGE=ltesystem-dep:RELEASE
```

`TOKEN` 启用时必须替换为随机强令牌；启用后所有 API 请求均携带 `Authorization: Bearer TOKEN`。host 网络不受 Docker 端口映射限制；bridge 使用 `LTE_BIND_ADDR`/`LTE_API_PORT` 控制发布，容器内部保持 `0.0.0.0:8081`。使用主机防火墙限制管理网来源，跨不可信网络使用 TLS 代理或 SSH。
When enabling authentication, replace `TOKEN` with a strong random token. Every API request then needs `Authorization: Bearer TOKEN`. Host networking bypasses Docker port mappings; bridge publishing uses `LTE_BIND_ADDR`/`LTE_API_PORT`, with `0.0.0.0:8081` inside the container. Restrict management sources with the host firewall and use TLS or SSH across untrusted networks.

此监听变更保留已有鉴权配置，不自动增加令牌。空令牌表示 API 未启用鉴权；建议设置令牌并限制管理网来源。普通 `docker run` 需显式设置 `LTE_LISTEN` 以确保预期监听。
This listener change preserves existing authentication and does not automatically add a token. An empty token means authentication is disabled; a token and management-network restrictions are recommended. Set `LTE_LISTEN` explicitly with plain `docker run` to ensure the intended binding.

bridge 发布端口走 Docker 转发路径，旧 host 的 INPUT/ufw 限制不一定生效。迁移前验证发布地址和 Docker 转发路径访问控制；`LTE_LISTEN` 不能限制 bridge 的宿主发布地址。
Bridge published ports use Docker's forwarding path; existing host INPUT/ufw restrictions may not apply. Validate the published address and forwarding-path access controls before migration. `LTE_LISTEN` does not restrict bridge host publishing. [Docker firewall guidance](https://docs.docker.com/engine/network/packet-filtering-firewalls/#docker-and-ufw)

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

## 7. Bridge 迁移 / Bridge migration

本项目 EPC/eNB 在同一容器，内部 S1AP/GTP 无需对外发布。选择 **单独** 的 `deploy/docker/docker-compose.bridge.yml`；不要以多个 `-f` 将它和 host 文件合并。Compose >= 2.36.0 支持固定 `eth0`。保留原 host 编排与原镜像作为一起回滚的组合。
EPC/eNB share a container; internal S1AP/GTP need no published ports. Select **only** `deploy/docker/docker-compose.bridge.yml`, never merge it with the host file using multiple `-f` options. Compose >= 2.36.0 supports fixed `eth0`. Keep the original host orchestration and image together for rollback.

1. 按第 3 节备份原 profile、数据卷、镜像；保留原项目名及 `/data` 实际卷。先构建后停机，GSM 容器不变。
   Back up the profile, volume and image as in section 3; retain the project and actual `/data` volume. Build before downtime, leaving GSM unchanged.
2. 检查 `ip -4 route` 与 `docker network inspect`；默认 bridge 为 `172.30.8.0/24`，与 UE 池 `172.16.0.0/24`、LAN/VPN/其它 Docker 网络不得重叠。需要时设置 `LTE_BRIDGE_SUBNET`。
   Check `ip -4 route` and `docker network inspect`; the default bridge `172.30.8.0/24` must not overlap the UE pool `172.16.0.0/24`, LAN/VPN or other Docker networks. Set `LTE_BRIDGE_SUBNET` when needed.
3. 使用原项目名执行 bridge 文件的 `config --quiet`、`up -d --no-build --force-recreate`。不要 `down -v`。API 默认仍从 `http://HOST:8081` 访问；重建只启动 API，不启动小区。
   Use the original project with the bridge file for `config --quiet` and `up -d --no-build --force-recreate`. Never use `down -v`. The API remains at `http://HOST:8081`; recreation starts the API, not the cell.
4. 下一次明确启动时覆盖 `network: "auto"`，不要继承旧宿主网卡名。auto 持久化为策略，每次启动解析当前命名空间的默认出口；状态中的 `resolved_network` 显示实际网卡。其它频段、功率、SIM 和 FPGA 参数不变。
   On the next explicit cell start, override `network: "auto"` instead of inheriting a host interface name. Auto is persisted as policy and resolved for each start; `resolved_network` reports the actual interface. Preserve band, gain, SIM and FPGA settings.
5. DNS 仍是**单独配置项**：Docker 的容器 DNS 不等于下发给 UE 的 PCO DNS，不能把 `127.0.0.11`/`127.0.0.53` 下发给手机。使用经过服务器实际查询验证的 IPv4 resolver，并通过 `dns` 字段显式设置。更改需要重新接入才生效。
   DNS remains **separate**: Docker's resolver is not UE PCO DNS; never advertise `127.0.0.11`/`127.0.0.53` to handsets. Explicitly set `dns` to an IPv4 resolver validated from the server. Reattach after changes.

```bash
# SDR server, after backup/build; reuse the original private environment/token.
# Add --env-file /absolute/path/.env to BOTH compose commands if applicable.
COMPOSE=deploy/docker/docker-compose.bridge.yml
docker compose -p "$PROJECT" -f "$COMPOSE" config --quiet
docker compose -p "$PROJECT" -f "$COMPOSE" up -d --no-build --force-recreate
docker exec ltesystem ip -4 route
docker exec ltesystem cat /proc/sys/net/ipv4/ip_forward
BASE=${BASE:-http://127.0.0.1:8081}  # Set to the actual published IP/port.
AUTH=()
if [ -n "${LTE_API_TOKEN:-}" ]; then AUTH=(-H "Authorization: Bearer $LTE_API_TOKEN"); fi
curl -fsS "${AUTH[@]}" "$BASE/api/v1/diagnostics/connectivity"
```

转发在容器 namespace 中按 SGi、UE 子网、出口与 conntrack 精确放行；宿主继续由 Docker 管理 bridge NAT。不要清空宿主 iptables 或将全局 FORWARD 策略改为 ACCEPT。host 回滚时必须同时恢复原编排和原 profile 的网络/DNS字段；只换回旧镜像而保留 bridge 会失败。恢复原 profile 时不要覆盖最新订户/SQN数据。
Forwarding is narrowly scoped to SGi, the UE subnet, uplink and conntrack in the container namespace; Docker manages bridge NAT on the host. Do not flush host iptables or globally set FORWARD to ACCEPT. A host rollback must restore the original orchestration and profile network/DNS fields together; reverting only the image while retaining bridge will fail. Do not overwrite updated subscriber/SQN data when restoring the profile.

### 诊断解释 / Interpreting diagnostics

- 已分配 UE IP、NAT 命中或 TCP 下行，只能分别证明对应阶段，不能单独证明手机能正常打开网页。DNS 超时、上游代理、无线重传可独立存在。
  A UE IP, NAT hits or TCP downlink each proves only its own stage, not full handset Internet access. DNS timeouts, upstream proxy issues and radio retransmissions can coexist.
- `POST /api/v1/crack/jobs` 是专用有副作用接口，不是联网测试。没有 CHAP 不等于注册失败；先读 `GET /api/v1/diagnostics/connectivity`，无需停基站。
  `POST /api/v1/crack/jobs` is a specialized side-effecting endpoint, not a connectivity test. Missing CHAP does not mean registration failed; first read `GET /api/v1/diagnostics/connectivity`, without stopping the cell.

参考 / References: [Docker bridge](https://docs.docker.com/engine/network/drivers/bridge/), [Compose interface_name](https://docs.docker.com/reference/compose-file/services/#interface_name).

---
**导航 Navigation:** [文档索引 Docs](README.md) · [API](API.md) · [Docker](../deploy/docker/README.md) · [MIGRATION](MIGRATION.md)
