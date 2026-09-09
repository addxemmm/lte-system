# Docker 指南 / Docker Guide

Docker 只在 SDR 服务器运行；开发机仅编辑、执行 Go/Python 测试与 Git 操作。本轮版本仍为 **2.1**。构建或创建容器不会自动启动 LTE 小区。

Run Docker only on the SDR server; development hosts only edit, run Go/Python tests and manage Git. This work remains version **2.1**. Building or creating a container never starts the LTE cell automatically.

## 文件 / Files

| 文件 File | 用途 Purpose |
|---|---|
| `Dockerfile` | 规范三阶段完整构建：srsRAN + CPU CTest、Go+embed、runtime / canonical full build |
| `Dockerfile.web-upgrade` | 只在相同且已确认的 2.1 core 上替换 Go+embed 与 example 配置 / fast app-layer upgrade on a verified matching 2.1 core |
| `docker-compose.bridge.yml` | 推荐 bridge 基础部署，只发布 Web `18081` / recommended base, Web only |
| `docker-compose.test.yml` | bridge 测试 override，显式开启并发布完整 API `8081` |
| `docker-compose.yml` | host-network 兼容/回滚基础部署，无 `ports` |
| `entrypoint.sh` | FPGA 选择、数据播种、PC/SC、无线探测、启动 Go / full LTE boot path |
| `select-uhd-fpga.sh` | `stock` / `compat` / `auto` FPGA 选择 |

bridge 与 host 两个基础 Compose 文件不能合并。只有 `docker-compose.test.yml` 可作为 bridge 的第二个文件。

Never merge the bridge and host base Compose files. Only `docker-compose.test.yml` may be added as the second file for bridge testing.

## 架构 / Architecture

规范 Dockerfile：

Canonical Dockerfile:

```text
ubuntu:22.04 srs-builder
  └─ pinned srsRAN_4G + local patches + seven CPU CTests
golang:1.22-bookworm go-builder
  └─ cmd/ + internal/ (including internal/webui/static go:embed assets)
ubuntu:22.04 runtime
  └─ srsRAN/UHD/pySIM/tools + one /usr/local/bin/lte-system
```

前端位于 `internal/webui/static`，`COPY internal/ ./internal/` 已覆盖全部 embed 输入；没有 Node/npm/nginx 阶段。规范镜像 `EXPOSE 18081`。8081 是仅在显式测试/legacy 模式启用的独立完整 API；`EXPOSE` 本身不发布端口，也不是安全屏障。

Frontend assets live under `internal/webui/static`; `COPY internal/ ./internal/` includes every embed input. There is no Node/npm/nginx stage. The canonical image declares `EXPOSE 18081`. Port 8081 is the direct full API enabled only for explicit test/legacy use. `EXPOSE` neither publishes a port nor creates a security boundary.

## 监听与 Compose / Listeners and Compose

| Base/override | `LTE_UI_LISTEN` | `LTE_EXPOSE_API` | `LTE_LISTEN` | 宿主发布 Host publishing |
|---|---|---|---|---|
| bridge base | `0.0.0.0:18081` | `false` | `0.0.0.0:8081`（不绑定） | `${LTE_UI_PORT:-18081}:18081` |
| bridge + test | 同上 / same | `true` | `0.0.0.0:8081` | UI + `${LTE_API_PORT:-8081}:8081` |
| host base | `${LTE_UI_LISTEN:-0.0.0.0:18081}` | `${LTE_EXPOSE_API:-false}` | `${LTE_LISTEN:-0.0.0.0:8081}` | host namespace，无 `ports` |

`LTE_EXPOSE_API` 只接受 `true`/`false`。仅当它为 true 时，应用拒绝 UI 与 API 内部端口相同。bridge 测试的 `LTE_UI_PORT`/`LTE_API_PORT` 也必须不同，否则 Docker 端口发布失败。

`LTE_EXPOSE_API` accepts only `true`/`false`. The application rejects equal internal UI/API ports only when it is true. Bridge test values `LTE_UI_PORT`/`LTE_API_PORT` must also differ or Docker publishing fails.

默认 18081 同时提供静态控制台、五字段非敏感 `/ui-config.json` 和有限同源管理网关。该网关仍经过原 Bearer Token 门禁，只包括管理只读与 cell start/stop；它不代理 crack、SIM、upload、capture。完整 API 只在独立监听显式开启后可用。这个变化会打断默认访问 `HOST:8081` 的旧客户端。

Default 18081 serves static console assets, a five-field non-sensitive `/ui-config.json`, and a limited same-origin management gateway. The gateway retains the original Bearer-token gate and includes only management reads and cell start/stop; it does not proxy crack, SIM, upload or capture. The full API is available only after explicitly enabling its direct listener. This breaks legacy clients that assumed `HOST:8081` was available by default.

## Token、Host 与 TLS / Token, Host and TLS

`LTE_API_TOKEN` 的非空值覆盖 YAML `api_token`；空环境值不清除文件值。浏览器只在当前页面内存保留 Token并为同源请求设置 Bearer header；刷新后重新输入。真实 Token 不进入 build arg、镜像、Git 或 URL。

A nonempty `LTE_API_TOKEN` overrides YAML `api_token`; an empty environment value does not erase the file value. The browser keeps the token only in current-page memory and sets the Bearer header on same-origin requests; re-enter it after refresh. Never put a real token in build args, images, Git or URLs.

YAML `ui_allowed_hosts: []` 默认只允许 literal IP/localhost；自定义 DNS 名称必须逐项列出 hostname，不写 scheme/port。匿名模式下也应限制可信网络并优先启用 Token，以降低 DNS rebinding 与未认证启停风险。

YAML `ui_allowed_hosts: []` accepts literal IP/localhost by default. List custom DNS names explicitly as hostname-only values without scheme/port. Restrict anonymous mode to trusted networks and prefer a token to reduce DNS-rebinding and unauthenticated cell-control risk.

Origin 比较使用直接 TLS 状态，不信任 `X-Forwarded-Proto`。普通 TLS 终止反代可能因浏览器 `https` Origin 与后端 HTTP 不一致而令 UI 的状态变更返回 403；部署反代前必须做端到端同源验证。

Origin comparison uses direct TLS state and does not trust `X-Forwarded-Proto`. Ordinary TLS termination may make browser `https` Origin disagree with backend HTTP and return 403 for UI mutations; validate same-origin behavior end to end before adding a proxy.

## 完整构建 / Full build

完整 Dockerfile 是正式发布和 core/依赖变化的唯一规范路径。它保留 srsRAN 固定 revision、本地补丁及 CPU CTest；升级上游必须重新审查补丁。Docker 构建上下文是仓库根。

The full Dockerfile is the only canonical path for formal releases or core/dependency changes. It retains the pinned srsRAN revision, local patches and CPU CTests; upstream upgrades require patch review. The Docker build context is the repository root.

```bash
docker build -f deploy/docker/Dockerfile \
  -t ltesystem-dep:FULL_TAG .
```

## 快速 Web 增量 / Fast Web upgrade

清理完整 build cache 后，可在**已确认 core 完全相同**的现有 `ltesystem-dep:2.1`（或其不可变 image ID/digest）上使用 `Dockerfile.web-upgrade`。`RUNTIME_BASE` 没有默认值，遗漏即构建失败；Go builder 固定使用服务器已有的 `golang:1.26.8-bookworm`。

After full build cache has been cleared, `Dockerfile.web-upgrade` may reuse an existing `ltesystem-dep:2.1` (or immutable image ID/digest) **only after its core is confirmed identical**. `RUNTIME_BASE` has no default, so omission fails the build. Its Go builder uses the server-cached `golang:1.26.8-bookworm`.

```bash
REVISION=$(git rev-parse HEAD)
docker build -f deploy/docker/Dockerfile.web-upgrade \
  --build-arg RUNTIME_BASE=ltesystem-dep:2.1 \
  --build-arg OCI_VERSION=2.1 \
  --build-arg OCI_REVISION="$REVISION" \
  -t "ltesystem-dep:web-test-${REVISION:0:12}" .
```

增量层只替换：

The app layer replaces only:

- `/usr/local/bin/lte-system`（全新 Go + embedded Web UI）
- `/app/configs/app.yaml`（来自仓库非敏感 `configs/app.yaml.example`）
- OCI version/revision/base labels

它不重编/替换 srsRAN、UHD、pySIM、系统包或正常 entrypoint，也不接受 Token build arg。父 2.1 镜像可能遗留 `EXPOSE 8081` 元数据；Compose 默认仍只发布 18081，且 `LTE_EXPOSE_API=false` 不绑定独立 API。正式发布仍回到完整 Dockerfile。

It does not rebuild/replace srsRAN, UHD, pySIM, system packages or the normal entrypoint, and accepts no token build argument. A parent 2.1 image may retain legacy `EXPOSE 8081` metadata; default Compose still publishes only 18081 and `LTE_EXPOSE_API=false` does not bind the direct API. Formal releases return to the full Dockerfile.

本次实际 2.1 base（`c18151d`）到候选源码的只读比较确认 C++、固件与静态无线配置无变化；仅应用 example 配置与第三方说明文档有差异，因此本次候选符合该增量前提。其它 base 必须重新比较。

The read-only comparison from the actual 2.1 base (`c18151d`) to this candidate found no C++, firmware or static-radio-config changes; only the application example config and third-party documentation differed. This candidate therefore meets the incremental prerequisite. Recheck every other base.

## 服务器启停 / Server start and stop

默认 bridge：

Default bridge:

```bash
COMPOSE=deploy/docker/docker-compose.bridge.yml
docker compose -p "$PROJECT" -f "$COMPOSE" config --quiet
docker compose -p "$PROJECT" -f "$COMPOSE" up -d --no-build --force-recreate
curl -fsS http://127.0.0.1:${LTE_UI_PORT:-18081}/ui-config.json
docker compose -p "$PROJECT" -f "$COMPOSE" stop
```

bridge 双端口测试：

Bridge dual-port test:

```bash
BASE=deploy/docker/docker-compose.bridge.yml
TEST=deploy/docker/docker-compose.test.yml
test "${LTE_UI_PORT:-18081}" != "${LTE_API_PORT:-8081}"
docker compose -p "$PROJECT" -f "$BASE" -f "$TEST" config --quiet
docker compose -p "$PROJECT" -f "$BASE" -f "$TEST" up -d --no-build --force-recreate
BASE=http://127.0.0.1:${LTE_API_PORT:-8081} bash scripts/smoke.sh
```

host 测试设置私有环境 `LTE_EXPOSE_API=true` 后只使用 `docker-compose.yml`；host 网络不使用 `ports`，也不叠加 test override。

For a host test, set private environment `LTE_EXPOSE_API=true` and use only `docker-compose.yml`; host networking uses no `ports` and never adds the test override.

## 正常 entrypoint 与管理专用 override / Normal entrypoint and management-only override

正常 `entrypoint.sh` 保持现有完整 LTE 行为：选择 FPGA；创建/刷新 `/data` 中的静态配置与缺失字典；启动 `pcscd`；探测 UHD/bladeRF；最后执行 Go 服务。它本身不启动小区。

The normal `entrypoint.sh` preserves full LTE behavior: select FPGA; create/refresh static `/data` configuration and a missing wordlist; start `pcscd`; probe UHD/bladeRF; then execute Go. It does not start a cell.

若管理测试必须避开共享 SDR 探测，可使用私有 Compose override：

For a management test that must avoid probing a shared SDR, use a private Compose override:

```yaml
services:
  lte-system:
    entrypoint: ["/usr/local/bin/lte-system"]
```

它只适用于继续挂载同一实际、**已播种** `lte-data` 卷；空卷/首次部署不使用。该方式跳过 FPGA 选择、静态配置刷新、字典播种、PC/SC 与无线探测。需要首次播种、完整 LTE 或写卡行为时恢复正常 entrypoint。详见 [部署指南](../../docs/DEPLOY.md) 和 [Web 控制台](../../docs/WEB_UI.md)。

Use it only while mounting the same actual, **already seeded** `lte-data` volume; never use it for an empty volume or first deployment. It skips FPGA selection, static-config refresh, wordlist seeding, PC/SC and radio probes. Restore the normal entrypoint for initial seeding, full LTE or SIM-programming behavior. See the [deployment guide](../../docs/DEPLOY.md) and [Web UI](../../docs/WEB_UI.md).

## 保留项 / Preserved deployment contracts

- bridge 固定容器 `eth0`，Compose >= 2.36.0；默认 `172.30.8.0/24` 不与 UE `172.16.0.0/24` 重叠。
- fixed bridge `eth0`, Compose >= 2.36.0; default `172.30.8.0/24` does not overlap UE `172.16.0.0/24`.
- host/bridge 同样保留 `/dev/bus/usb`、`lte-data:/data`、`privileged`、`UHD_FPGA=compat` 与 210 秒 stop grace。
- host/bridge preserve `/dev/bus/usb`, `lte-data:/data`, `privileged`, `UHD_FPGA=compat` and the 210-second stop grace.
- 实际卷名从现有容器解析；升级/回滚不 `down -v`，不覆盖最新 SQN。
- resolve the actual volume from the existing container; never `down -v` or overwrite the newest SQN during upgrade/rollback.
- 替换前备份数据卷；旧镜像/缓存仅保留至新部署验收，之后按用户保留策略定向清理。当前测试部署明确不保留回滚镜像。
- Back up the data volume; retain old images/caches until acceptance, then clean only selected obsolete artifacts according to the user's retention choice. The current test deployment explicitly does not retain rollback images.

---
**导航 Navigation:** [README](../../README.md) · [DEPLOY](../../docs/DEPLOY.md) · [WEB_UI](../../docs/WEB_UI.md) · [SDR](../../docs/SDR.md)
