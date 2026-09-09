# 部署与回滚 / Deployment and rollback

> 当前源码默认前端 **18081**、后端 **8081**，后端仍需显式启用。已发布的不可变 Hub `2.1` 镜像内置旧 UI 默认值/`EXPOSE 8080`，本次没有覆盖该镜像；使用它时显式设置 `LTE_UI_LISTEN=0.0.0.0:18081` 并发布 `18081:18081`。当前服务器已采用此配置。裸 `8080/tcp` 元数据不等于宿主发布或实际监听。详见 [端口统一记录](PORTS_2026-09-09.md)。
>
> Current source defaults are UI **18081**, API **8081**, with API exposure still opt-in. The immutable published Hub 2.1 image retains its old baked-in UI default/EXPOSE 8080; it was not overwritten. Run it with explicit LTE_UI_LISTEN=0.0.0.0:18081 and mapping 18081:18081, as on the current server. Bare EXPOSE metadata is neither host publishing nor an active listener.

开发机只编辑、测试与管理 Git；Docker 构建、容器替换和 SDR 验证均在服务器执行。版本保持 **2.1**。容器启动管理服务，但不会自动启动 LTE 小区。

Development machines only edit, test and manage Git. Build images, replace containers and validate SDR hardware on the server. The version remains **2.1**. Container creation starts management services but never starts the LTE cell automatically.

> **部署默认值有破坏性变化 / Breaking deployment default:** 旧客户端默认使用的独立 `HOST:8081` API 不再监听或发布。默认入口改为 `HOST:18081` Web 控制台及其有限同源管理网关。旧客户端或完整 API 测试必须显式设置 `LTE_EXPOSE_API=true`；bridge 还必须叠加 `docker-compose.test.yml` 发布 8081。
>
> The direct `HOST:8081` API used by older clients is no longer listening or published by default. The default entry is the Web console on `HOST:18081` and its limited same-origin management gateway. Legacy clients or full-API tests must explicitly set `LTE_EXPOSE_API=true`; bridge deployments must also add `docker-compose.test.yml` to publish 8081.

## 1. 文件、端口与范围 / Files, ports and scope

| 模式 Mode | Compose 文件 Compose files | 宿主入口 Host entry | 独立完整 API Direct full API |
|---|---|---|---|
| bridge 默认 / default | `deploy/docker/docker-compose.bridge.yml` | `${LTE_UI_PORT:-18081}:18081` | 关闭且不发布 / disabled, not published |
| bridge 测试 / test | bridge + `deploy/docker/docker-compose.test.yml` | `${LTE_UI_PORT:-18081}:18081` | `${LTE_API_PORT:-8081}:8081`，已启用 |
| host 默认 / default | `deploy/docker/docker-compose.yml` | `${LTE_UI_LISTEN:-0.0.0.0:18081}` | 关闭；无 `ports` / disabled; no port map |
| host 测试 / test | host base + `LTE_EXPOSE_API=true` | `LTE_UI_LISTEN` | `LTE_LISTEN`（默认 `0.0.0.0:8081`）；仍无 `ports` |

bridge 和 host 两个**基础文件绝不叠加**。`docker-compose.test.yml` 只是 bridge 测试 override。测试时 `LTE_UI_PORT` 和 `LTE_API_PORT` 必须不同。应用只在 `expose_api=true` 时拒绝相同的内部 UI/API 监听端口。Dockerfile 的 `EXPOSE` 是元数据，不构成网络隔离。

Never combine the bridge and host **base files**. `docker-compose.test.yml` is only a bridge test override. `LTE_UI_PORT` and `LTE_API_PORT` must differ during tests. The application rejects equal internal UI/API listen ports only when `expose_api=true`. Dockerfile `EXPOSE` metadata is not network isolation.

控制台同源 `/api/v1` 只包含管理只读路径和 `GET/POST/DELETE /api/v1/cell`；仍经过原 Bearer Token 门禁。它不代理 crack、SIM、upload、capture 或订户写操作。完整 API 只能从显式启用的独立监听访问。路由明细见 [Web 控制台](WEB_UI.md)。

The console's same-origin `/api/v1` includes only management reads plus `GET/POST/DELETE /api/v1/cell`, all behind the original Bearer-token gate. It does not proxy crack, SIM, upload, capture or subscriber writes. Access the full API only through the explicitly enabled direct listener. See [Web UI](WEB_UI.md) for the exact route list.

## 2. 同步源码与本地验证 / Synchronize source and local validation

部署脚本打包 **Git 已跟踪文件的工作区内容**。新文件必须先 `git add`；未暂存的已跟踪修改会包含。脚本排除真实卡库、字典、私有 `configs/app.yaml`、`.env*`、运行日志与抓包。同步/构建脚本不重建容器、不启动小区。

Deployment scripts package **working-tree contents of Git-tracked files**. Stage new files with `git add`; tracked unstaged edits are included. Real subscriber databases, wordlists, private `configs/app.yaml`, `.env*`, runtime logs and captures are excluded. Sync/build scripts neither recreate containers nor start a cell.

```powershell
# Windows development host: sync; -Build builds remotely without replacement.
./scripts/deploy_from_windows.ps1 -HostAlias TARGET -Build
```

```bash
# Linux development host: sync and remote build, no container replacement.
bash scripts/deploy_to_ubuntu.sh addx@TARGET
```

提交或服务器构建前运行：

Run before handoff or server build:

```bash
go test ./...
go vet ./...
python -m unittest discover -s scripts/tests -v
```

开发机不运行 `docker build/run`、无线探测或常驻服务。

Do not run Docker builds/containers, radio probes or long-lived services on the development machine.

## 3. 配置、Token、Host 与 TLS / Configuration, token, Host and TLS

私有服务器 `.env` 示例；不要提交真实 Token：

Private server `.env` example; never commit a real token:

```dotenv
LTE_IMAGE=ltesystem-dep:WEB_TEST_TAG
LTE_UI_PORT=18081
LTE_API_PORT=8081
LTE_EXPOSE_API=false
LTE_API_TOKEN=TOKEN
```

bridge 容器内地址固定为 `0.0.0.0:18081` 和 `0.0.0.0:8081`；host 可用 `LTE_UI_LISTEN`/`LTE_LISTEN` 覆盖。`LTE_EXPOSE_API` 是严格布尔值，只接受 `true`/`false`。非空 `LTE_API_TOKEN` 优先于 YAML `api_token`；空环境变量不清除 YAML Token。Token 不写入镜像、URL、Git 或 `/ui-config.json`。

Bridge container addresses are fixed at `0.0.0.0:18081` and `0.0.0.0:8081`; host mode can override them with `LTE_UI_LISTEN`/`LTE_LISTEN`. `LTE_EXPOSE_API` is a strict boolean accepting only `true`/`false`. A nonempty `LTE_API_TOKEN` overrides YAML `api_token`; an empty environment value does not erase the YAML token. Never put the token in an image, URL, Git or `/ui-config.json`.

需要私有 YAML 时，以只读 bind mount 显式选择，保持实际文件在镜像/Git 外：

For a private YAML, select it through a read-only bind mount and keep it outside the image/Git:

```yaml
services:
  lte-system:
    environment:
      LTE_CONFIG: /run/lte-system/app.yaml
    volumes:
      - type: bind
        source: /ABSOLUTE/PATH/app.yaml
        target: /run/lte-system/app.yaml
        read_only: true
        bind:
          create_host_path: false
```

自定义 DNS 名称必须在 YAML `ui_allowed_hosts` 中列出，只写 hostname，不写 scheme/port。空列表默认接受 literal IP 与 localhost。匿名控制台仍允许任何可达用户调用网关范围内的小区启停；Host allowlist 不替代 Token、防火墙或可信管理网。

List every custom console DNS name in YAML `ui_allowed_hosts` as a hostname only, without scheme/port. An empty list accepts literal IPs and localhost by default. An anonymous console still lets any reachable user invoke gateway-permitted cell start/stop; the Host allowlist does not replace a token, firewall or trusted management network.

Origin 校验使用 Go 收到的直接 `r.TLS`，当前不信任 `X-Forwarded-Proto`。普通 TLS 终止反代会造成浏览器 `https` Origin 与后端所见 HTTP 不一致，状态变更返回 403。引入反代前必须验证同源和直接 TLS 行为，不要通过伪造 Origin/X-Forwarded-Proto 放宽检查。

Origin validation uses direct `r.TLS` as seen by Go and currently does not trust `X-Forwarded-Proto`. Ordinary TLS termination can make the browser's `https` Origin disagree with backend HTTP, causing 403 on mutations. Validate same-origin/direct-TLS behavior before adding a proxy; do not weaken checks by spoofing Origin or forwarded-protocol headers.

## 4. 识别并备份实际数据卷 / Resolve and back up the actual data volume

`lte-data` 是逻辑名，实际名称取决于 Compose 项目。替换前从现有容器读取，停机前确认没有在途写卡/上传；不要假设裸 `lte-data`，也不要执行 `down -v`。

`lte-data` is a logical name; the actual volume depends on the Compose project. Resolve it from the existing container, ensure no SIM/upload operation is in flight, never assume a bare `lte-data`, and never run `down -v`.

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
# Optional only when rollback-image retention is requested.
# 仅在要求保留回滚镜像时执行；当前测试部署按用户要求不保留。
# docker tag "$OLD_IMAGE" "ltesystem-dep:rollback-$RELEASE"
docker stop --time 210 ltesystem
docker run --rm --network none --read-only --entrypoint tar \
  --mount "type=volume,src=$DATA_VOLUME,dst=/data,readonly" \
  --mount "type=bind,src=$BACKUP_DIR,dst=/backup" \
  "$OLD_IMAGE" czf /backup/data.tgz -C /data .
tar tzf "$BACKUP_DIR/data.tgz" >/dev/null
sha256sum "$BACKUP_DIR/data.tgz" > "$BACKUP_DIR/data.tgz.sha256"
```

备份失败立即 `docker start ltesystem`，不替换容器。备份含私有卡库，限制权限且不上传。保留同一卷也保留最新 SQN。

If backup fails, immediately `docker start ltesystem` and do not replace it. The backup contains private subscriber data; restrict permissions and never upload it. Retaining the same volume also retains the newest SQN.

## 5. 两种构建 / Two build paths

### 5.1 完整规范构建 / Canonical full build

`deploy/docker/Dockerfile` 是规范发布路径：重建固定 srsRAN core、运行 CPU CTest、构建 Go+embed 控制台并组装完整运行镜像。发布或 core/UHD/依赖变化必须使用它。

`deploy/docker/Dockerfile` is the canonical release path: it rebuilds the pinned srsRAN core, runs CPU CTests, builds the Go+embed console and assembles the full runtime. Use it for releases or any core/UHD/dependency change.

```bash
export LTE_IMAGE="ltesystem-dep:$RELEASE"
docker compose -p "$PROJECT" -f deploy/docker/docker-compose.bridge.yml config --quiet
docker compose -p "$PROJECT" -f deploy/docker/docker-compose.bridge.yml build
```

### 5.2 已验证 2.1 core 的 Web 增量 / Web upgrade on a verified 2.1 core

`deploy/docker/Dockerfile.web-upgrade` 只适用于已确认 core 与当前 2.1 完全相同的现有镜像。`RUNTIME_BASE` 必填，显式传 `ltesystem-dep:2.1` 或不可变 image ID/digest；它使用服务器已有的 `golang:1.26.8-bookworm` 编译全新 Go+embed 前端，然后在 base 上只替换 `/usr/local/bin/lte-system` 和由非敏感 `configs/app.yaml.example` 生成的 `/app/configs/app.yaml`。srsRAN、UHD、pySIM、系统库与 entrypoint 保持 base 内容。

`deploy/docker/Dockerfile.web-upgrade` is only for an existing image whose core is confirmed identical to the current 2.1 core. `RUNTIME_BASE` is mandatory: pass `ltesystem-dep:2.1` or an immutable image ID/digest explicitly. It uses the server's cached `golang:1.26.8-bookworm` to compile a fresh Go+embed frontend, then replaces only `/usr/local/bin/lte-system` and `/app/configs/app.yaml` derived from non-secret `configs/app.yaml.example`. srsRAN, UHD, pySIM, system libraries and entrypoint remain from the base.

```bash
BASE_IMAGE=ltesystem-dep:2.1       # or a verified immutable sha256 image reference
REVISION=$(git rev-parse HEAD)
WEB_IMAGE="ltesystem-dep:web-test-${REVISION:0:12}"
docker build -f deploy/docker/Dockerfile.web-upgrade \
  --build-arg RUNTIME_BASE="$BASE_IMAGE" \
  --build-arg OCI_VERSION=2.1 \
  --build-arg OCI_REVISION="$REVISION" \
  -t "$WEB_IMAGE" .
```

该构建没有 Token build arg，不会烘焙真实 Token。OCI `version`/`revision` 标签由 build args 覆盖。由于父 2.1 镜像可能继承旧 `EXPOSE 8081` 元数据，增量镜像 inspect 可能同时看到 18081/8081；这不会发布端口，安全边界仍由 `LTE_EXPOSE_API=false` 与 Compose 仅发布 18081 实现。规范完整镜像只声明 18081。

This build has no token build argument and never bakes a real token. OCI `version`/`revision` labels are overridden by build args. Because the parent 2.1 image may carry legacy `EXPOSE 8081` metadata, inspection of the incremental image may show both 18081/8081; that publishes nothing. The boundary remains `LTE_EXPOSE_API=false` plus Compose publishing only 18081. The canonical full image declares only 18081.

增量构建不是通用升级：base core 未逐项确认、VERSION/core/依赖变化或正式发布时回到完整 Dockerfile。

The incremental build is not a general upgrade path. Use the canonical Dockerfile whenever the base core is not explicitly matched, VERSION/core/dependencies change, or a formal release is produced.

本测试候选已对实际 2.1 base（`c18151d`）到当前源码做只读比较：C++、固件和静态无线配置无差异；差异仅含应用 example 配置与第三方说明文档，因此满足本次增量 core 前提。该结论只覆盖此 base/候选组合，不自动适用于其它标签。

For this test candidate, a read-only comparison from the actual 2.1 base (`c18151d`) to current source found no C++, firmware or static-radio-configuration changes; differences were limited to the application example config and third-party documentation. This establishes the incremental-core prerequisite only for this base/candidate pair, not for other tags.

## 6. 测试版替换与验证 / Test-image replacement and validation

先构建并完成第 4 节备份，再重建默认 bridge。使用原 `PROJECT`、同一实际卷与私有 env/override；不要改变其它容器。以下命令不会启动小区。

Build first and complete the section 4 backup, then recreate the default bridge deployment. Reuse the original `PROJECT`, actual volume and private env/override; do not modify other containers. These commands do not start a cell.

```bash
export LTE_IMAGE="$WEB_IMAGE"
COMPOSE=deploy/docker/docker-compose.bridge.yml
docker compose -p "$PROJECT" -f "$COMPOSE" config --quiet
docker compose -p "$PROJECT" -f "$COMPOSE" up -d --no-build --force-recreate
ACTUAL_VOLUME=$(docker inspect ltesystem --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}')
test "$ACTUAL_VOLUME" = "$DATA_VOLUME"
curl -fsS http://127.0.0.1:${LTE_UI_PORT:-18081}/ui-config.json
docker logs --tail 50 ltesystem
```

验证：18081 首页/静态资源与五字段 `/ui-config.json`；有 Token 时未授权网关请求为 401、授权请求成功；默认 8081 没有监听/发布；控制台受限路由工作且 crack/SIM/upload/capture 返回网关 404。浏览器测试确认 Token 只在页面内存，刷新后清除。不要在基础 smoke 中点击“启动小区”。

Validate the 18081 page/assets and five-field `/ui-config.json`; with a token, verify unauthorized gateway requests return 401 and authorized requests succeed; confirm 8081 is not listening/published by default; verify permitted console routes and gateway 404 for crack/SIM/upload/capture. Confirm in a browser that the token is page-memory only and clears on refresh. Do not select “Start cell” during basic smoke validation.

### 双端口完整 API 测试 / Dual-port full API test

```bash
COMPOSE_BASE=deploy/docker/docker-compose.bridge.yml
COMPOSE_TEST=deploy/docker/docker-compose.test.yml
test "${LTE_UI_PORT:-18081}" != "${LTE_API_PORT:-8081}"
docker compose -p "$PROJECT" -f "$COMPOSE_BASE" -f "$COMPOSE_TEST" config --quiet
docker compose -p "$PROJECT" -f "$COMPOSE_BASE" -f "$COMPOSE_TEST" up -d --no-build --force-recreate
BASE=http://127.0.0.1:${LTE_API_PORT:-8081} bash scripts/smoke.sh
```

测试 override 显式设置 `LTE_EXPOSE_API=true`，同时保留 18081。host 测试只设置该环境值，不叠加 test override、不使用 `ports`。smoke 只读 `/api/v1/cell` 和 `/api/v1/profile`，不会启停小区、写卡、上传、抓包或破解。

The test override explicitly sets `LTE_EXPOSE_API=true` while retaining 18081. In host-mode tests, set only that environment value: do not add the test override or use `ports`. Smoke reads only `/api/v1/cell` and `/api/v1/profile`; it never starts/stops a cell, programs a SIM, uploads, captures or cracks.

## 7. 跳过硬件探测的管理模式 / Management mode without hardware probes

正常 `/entrypoint.sh` 保持完整 LTE 语义：选择 FPGA、刷新/播种 `/data`、启动 `pcscd`、探测 UHD/bladeRF，最后执行 Go 服务。若管理测试必须避免探测另一任务正在使用的 SDR，可创建私有 override：

The normal `/entrypoint.sh` retains full LTE semantics: select FPGA, refresh/seed `/data`, start `pcscd`, probe UHD/bladeRF, then execute Go. If a management test must avoid probing an SDR used by another task, create a private override:

```yaml
services:
  lte-system:
    entrypoint: ["/usr/local/bin/lte-system"]
```

只在已确认 `docker_lte-data`（或从现有容器解析出的实际卷）已播种并将继续原样挂载时使用。该 override 跳过 FPGA 选择、`sib.conf`/`rb.conf` 刷新、`wordlist.list` 首次播种、PC/SC 与无线探测；空卷/首次部署不使用。直接 Go 启动不会自动启动小区。需要完整 LTE 或写卡行为时恢复正常 entrypoint。

Use it only after confirming `docker_lte-data` (or the actual volume resolved from the existing container) is already seeded and will be mounted unchanged. This skips FPGA selection, `sib.conf`/`rb.conf` refresh, first-run `wordlist.list` seeding, PC/SC and radio probes; do not use it for an empty volume or first deployment. Direct Go startup does not start a cell automatically. Restore the normal entrypoint for full LTE or SIM-programming behavior.

## 8. 回滚 / Rollback

本节仅适用于已选择保留回滚镜像的部署。当前测试部署按用户要求不保留旧镜像或多余 tag：验证新容器后定向删除旧镜像，保留数据备份与源快照；没有旧镜像时需从源重建，不能使用下面的快速切换。/ This section applies only when image rollback was selected. The current test deployment retains data backups and source snapshots, not obsolete images or tags. Without an old image, recovery requires a rebuild rather than the fast switch below.

```bash
export LTE_IMAGE="ltesystem-dep:rollback-$RELEASE"
docker compose -p "$PROJECT" -f deploy/docker/docker-compose.bridge.yml up -d --no-build --force-recreate
curl -fsS http://127.0.0.1:${LTE_UI_PORT:-18081}/ui-config.json
```

回滚镜像时保持同一数据卷。只在服务停止、确有需要时校验 SHA-256 后恢复 `data.tgz`；恢复会覆盖同名文件，不能覆盖更新后的 SQN。保留旧镜像与备份，稳定验收前不清理 build cache。host 回滚要同时恢复原 host compose 与 profile 的网络/DNS；不要只换镜像。

Keep the same data volume during image rollback. Restore `data.tgz` only when necessary, with the service stopped and SHA-256 verified; restoration overwrites names and must not roll back newer SQNs. Retain the old image and backup; do not clean build cache before acceptance. A host rollback must restore the host compose and profile network/DNS together, not only the image.

## 9. Bridge 与运行边界 / Bridge and runtime boundaries

- bridge 需要 Compose >= 2.36.0，以 `interface_name: eth0` 固定容器出口；默认 `172.30.8.0/24` 不得与 UE 池 `172.16.0.0/24`、LAN、VPN 或其它 bridge 重叠。
- bridge needs Compose >= 2.36.0 for fixed container `eth0`; default `172.30.8.0/24` must not overlap the `172.16.0.0/24` UE pool, LAN, VPN or another bridge.
- `/dev/bus/usb`、`lte-data:/data`、`privileged`、`UHD_FPGA=compat` 与 `stop_grace_period: 210s` 在 host/bridge 保持一致。
- `/dev/bus/usb`, `lte-data:/data`, `privileged`, `UHD_FPGA=compat` and `stop_grace_period: 210s` remain identical across host/bridge.
- 容器创建仅启动管理服务；小区启动仍需明确操作，且使用 bridge 时 profile 网络应为 `auto`。容器 DNS 与 UE PCO DNS 是不同配置。
- Container creation starts management only; cell start remains explicit, and bridge profiles should use network `auto`. Container DNS and UE PCO DNS are separate settings.
- 不清空宿主 iptables，不把全局 FORWARD 改为 ACCEPT，不改动共享 GSM 容器。
- Do not flush host iptables, globally set FORWARD to ACCEPT, or modify a shared GSM container.

---
**导航 Navigation:** [文档索引 Docs](README.md) · [Web 控制台](WEB_UI.md) · [API](API.md) · [Docker](../deploy/docker/README.md) · [MIGRATION](MIGRATION.md)
