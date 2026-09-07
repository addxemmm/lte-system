# Docker 指南 Docker Guide（Dockerfile · Compose · 镜像版本 / Dockerfile · Compose · Image Versions）

当前部署脚本仅同步/构建，不替换容器；全网卡监听、实际卷备份和唯一标签回滚见 [部署指南](../../docs/DEPLOY.md)。
Current deploy scripts sync/build only; see the [deploy guide](../../docs/DEPLOY.md) for all-interface binding, actual-volume backups and unique-tag rollback.

运行位置：**SDR 服务器**（开发机只改代码，不跑 docker）。日常部署看 [`docs/DEPLOY.md`](../../docs/DEPLOY.md)，这里讲镜像本身是怎么构成、怎么迭代的。
Runs on: **the SDR server** (dev machines only edit code, never run docker). For daily deploy see [`docs/DEPLOY.md`](../../docs/DEPLOY.md); this doc covers how the image itself is built and iterated.

## 1. 文件一览 Files at a Glance

| 文件 File | 作用 Purpose |
|---|---|
| [`deploy/docker/Dockerfile`](Dockerfile) | 三段构建，产出 `ltesystem-dep:<VERSION>` / Three-stage build producing `ltesystem-dep:<VERSION>` |
| [`deploy/docker/docker-compose.yml`](docker-compose.yml) | host 兼容/回滚编排 / Host-network compatibility/rollback orchestration |
| [`deploy/docker/docker-compose.bridge.yml`](docker-compose.bridge.yml) | 单容器 EPC/eNB 的独立 bridge 编排，固定 eth0 / Isolated all-in-one EPC/eNB bridge orchestration, fixed eth0 |
| [`deploy/docker/entrypoint.sh`](entrypoint.sh) | 容器启动流程：选 FPGA → seeding `/data` → 起 pcscd → exec Go 服务 / Container boot flow: pick FPGA → seed `/data` → start pcscd → exec Go service |
| [`deploy/docker/select-uhd-fpga.sh`](select-uhd-fpga.sh) | `stock`/`compat` FPGA 切换（见 [`docs/SDR.md`](../../docs/SDR.md)） / `stock`/`compat` FPGA switching (see [`docs/SDR.md`](../../docs/SDR.md)) |
| `VERSION`（仓库根） / `VERSION` (repo root) | 镜像版本号，当前 `2.0`（从旧 `ltesystem-dep:1.0` 迭代而来） / Image version, currently `2.0` (iterated from old `ltesystem-dep:1.0`) |

## 2. Dockerfile 三段在干什么 What the Three Dockerfile Stages Do

```
ubuntu:22.04 (srs-builder)  →  srsRAN_4G release_23_11 源码编译
golang:1.22-bookworm (go-builder) →  Go tool API 二进制
ubuntu:22.04 (runtime)      →  运行镜像（1.53GB，旧 1.0 镜像的一半不到）
```

`ubuntu:22.04 (srs-builder)` → srsRAN_4G `release_23_11` 源码编译；`golang:1.22-bookworm (go-builder)` → Go tool API 二进制；`ubuntu:22.04 (runtime)` → 运行镜像（1.53GB，不到旧 1.0 镜像的一半）。
`ubuntu:22.04 (srs-builder)` → srsRAN_4G `release_23_11` source build; `golang:1.22-bookworm (go-builder)` → Go tool API binary; `ubuntu:22.04 (runtime)` → runtime image (1.53GB, less than half of the old 1.0 image).

- **srs-builder**：装编译依赖（含 UHD/bladeRF 可选），`git clone --branch ${SRSRAN_VERSION}` 后 `cmake Release && make && make install`。换 srsRAN 版本只改顶部 `ARG SRSRAN_VERSION`（tag 形如 `release_23_11`）。 / **srs-builder**: installs build deps (incl. optional UHD/bladeRF), then `git clone --branch ${SRSRAN_VERSION}` followed by `cmake Release && make && make install`. To change the srsRAN version only edit the top `ARG SRSRAN_VERSION` (tags look like `release_23_11`).
  - 基座必须是 **22.04**：srsRAN_4G 在 gcc-13（24.04 默认）下编不过；22.04 自带 gcc-11 正好。 / The base must be **22.04**: srsRAN_4G fails to build under gcc-13 (24.04 default); 22.04 ships gcc-11 which fits.
  - 曾踩过的坑（已修，升级依赖时注意）：运行时包名是 `libmbedtls14` 不是 `libmbedtls7`；srsRAN 还要 `libboost-system/thread/test-dev`；runtime 层要装 `git`（给 pysim 用）；**pysim 必须 pin 在 `ARG PYSIM_COMMIT`（2023-08），master 已重构不兼容，且 `testsim` 卡逻辑来自 [`third_party/pysim/`](../../third_party/pysim) 覆盖**。 / Past pitfalls (fixed, watch out when upgrading deps): the runtime package name is `libmbedtls14` not `libmbedtls7`; srsRAN also needs `libboost-system/thread/test-dev`; the runtime layer must install `git` (for pysim); **pysim must stay pinned at `ARG PYSIM_COMMIT` (2023-08) — master was rewritten and is incompatible, and the `testsim` card logic comes from the [`third_party/pysim/`](../../third_party/pysim) overlay**.
- **go-builder**：`CGO_ENABLED=0` 静态编译 [`cmd/server`](../../cmd/server)。改 Go 代码只会重跑这一段及之后（约 1–2 分钟），srsRAN 层走缓存。 / **go-builder**: statically builds [`cmd/server`](../../cmd/server) with `CGO_ENABLED=0`. Go-only changes re-run just this and later stages (about 1–2 min); the srsRAN layer hits cache.
- **runtime**：只装运行库 + 工具（`srsepc/srsenb` 从 builder 拷、`tcpdump/tshark/hashcat/pcscd`、UHD 镜像下载 + 兼容板 FPGA 覆盖、**era-pinned pysim + 定制卡逻辑覆盖**、Go 二进制、[`configs/`](../../configs)、`entrypoint.sh`）。 / **runtime**: runtime libs + tools only (`srsepc/srsenb` copied from builder, `tcpdump/tshark/hashcat/pcscd`, UHD image download + compatible board FPGA overlay, **era-pinned pysim + custom card overlay**, Go binary, [`configs/`](../../configs), `entrypoint.sh`).
  - `ENV LTE_CONFIG=/app/configs/app.yaml`：构建时由 `app.yaml.example` 物化，**改配置改仓库里的 example 文件**，直接改容器内文件重建即丢。 / `ENV LTE_CONFIG=/app/configs/app.yaml`: materialized from `app.yaml.example` at build time; **edit the example file in the repo to change config** — edits inside the container are lost on rebuild.
  - `EXPOSE 8081` 只是声明；bridge 编排通过 ports 发布 API，host 编排直接使用宿主网络。 / `EXPOSE 8081` is declarative; bridge publishes the API via ports, while host uses the host namespace directly.

## 3. docker-compose.yml 逐项解释 docker-compose.yml Explained Item by Item

```yaml
build: { context: ../.., dockerfile: deploy/docker/Dockerfile }  # 相对本文件的仓库根
image: ${LTE_IMAGE:-ltesystem-dep:2.0}  # 支持唯一发布/回滚标签 / unique release/rollback tag
container_name: ltesystem
network_mode: host              # 兼容选项；隔离网络另用 docker-compose.bridge.yml
privileged: true                # 必需：建网卡、实时线程、访问 USB
volumes:
  - /dev/bus/usb:/dev/bus/usb   # USRP + 读卡器直通
  - lte-data:/data              # 配置/卡库/日志/抓包持久化（重建不丢）
environment:
  LTE_LISTEN: ${LTE_LISTEN:-0.0.0.0:8081}  # 默认全网卡 / all IPv4 interfaces by default
  LTE_API_TOKEN: ${LTE_API_TOKEN:-}         # A strong token is recommended for LAN access
  UHD_FPGA: compat              # 本机兼容板；正版 B210 改 stock
restart: "no"                   # 手动启停，不开机自启 / manual start-stop, no auto-start
logging: { max-size: 50m, max-file: 5 }  # 防 docker 日志撑爆盘
```

构建上下文是相对本文件的仓库根；`image` 与 `VERSION` 同步，改版时一起改。
The build context is the repo root relative to this file; `image` stays in sync with `VERSION`, change both on release.

host 不是 srsRAN 的硬性要求：本项目 EPC/eNB 同容器，S1AP/GTP 绑定内部 loopback，SGi 可建在容器网络命名空间。bridge 使用独立的 `docker-compose.bridge.yml`，不要与 host 文件叠加。保留 privileged 以兼容现有 USB/实时线程，权限收紧另行验证。
Host networking is not required: this project's EPC/eNB share a container, S1AP/GTP use internal loopback, and SGi can live in its network namespace. Use `docker-compose.bridge.yml` alone, never merged with the host file. Privileged mode is retained for existing USB/realtime compatibility; privilege reduction needs separate validation.

bridge 需要 Compose >= 2.36.0，以 `interface_name: eth0` 固定出口名，只发布 `0.0.0.0:8081/tcp`。UE 流量经过容器与 Docker 两层 NAT。启动小区使用 `network: "auto"`；旧 profile 的宿主接口名需显式迁移，不能在新命名空间继续用 `ens33` 等宿主名称。具体备份、DNS 与回滚步骤见 [部署指南](../../docs/DEPLOY.md#7-bridge-迁移--bridge-migration)。
Bridge requires Compose >= 2.36.0 for `interface_name: eth0`, publishing only `0.0.0.0:8081/tcp`. UE traffic passes through container and Docker NAT. Start with `network: "auto"`; explicitly migrate host-specific interface names saved in old profiles. See the [deployment guide](../../docs/DEPLOY.md#7-bridge-迁移--bridge-migration) for backups, DNS and rollback.

卷挂载：`/dev/bus/usb:/dev/bus/usb` 给 USRP + 读卡器直通；`lte-data:/data` 给配置/卡库/日志/抓包持久化（重建不丢）。
Volumes: `/dev/bus/usb:/dev/bus/usb` passes through USRP + reader; `lte-data:/data` persists config/SIM database/logs/packet capture (survives rebuilds).

环境：`UHD_FPGA: compat` 指本机兼容板；正版 B210 改 `stock`。`logging` 防 docker 日志撑爆盘。
Environment: `UHD_FPGA: compat` means this machine's compatible board; genuine B210 uses `stock`. `logging` keeps docker logs from filling the disk.

常用命令（服务器上，源码快照根目录；必须选择与当前部署一致的编排并保留私有 env）：
Common commands (on the server, in the source snapshot root; select the current deployment's orchestration and retain its private env):

```bash
COMPOSE=deploy/docker/docker-compose.bridge.yml  # host deployment: docker-compose.yml
sudo docker compose -p "$PROJECT" -f "$COMPOSE" build  # 先构建，不停现有实例 / build without stopping
# After backup: recreate the API only, using the previously selected image/tag.
sudo docker compose -p "$PROJECT" -f "$COMPOSE" up -d --no-build --force-recreate
sudo docker logs --tail 200 ltesystem
sudo docker exec ltesystem <cmd>   # 进容器执行，如 uhd_find_devices
```

## 4. 版本迭代规范 Release Iteration Rules

1. 改代码/配置 → 本地 `go test ./...` 全绿。 / Change code/config → local `go test ./...` all green.
2. 根目录 `VERSION` 递增（如 `2.0` → `2.1`），`docker-compose.yml` 的 `image:` 同步改。 / Bump repo-root `VERSION` (e.g. `2.0` → `2.1`), update `image:` in `docker-compose.yml` in sync.
3. 服务器按 [DEPLOY](../../docs/DEPLOY.md) 分开构建、备份和替换，使用唯一镜像标签与当前网络编排。 / Follow [DEPLOY](../../docs/DEPLOY.md) to separately build, back up and replace, with a unique image tag and the selected network orchestration.
4. 存 tarball 备份（1.5GB 级，`~` 下保留最近两个版本即可）： / Keep a tarball backup (1.5GB class, keep the latest two under `~`):
   ```bash
   sudo docker save ltesystem-dep:2.1 -o ~/ltesystem-dep-2.1.tar
   # Import only: sudo docker load -i ~/ltesystem-dep-2.1.tar
   ```
   `docker load` 仅导入镜像；实际回滚还需替换容器，并恢复原编排和 profile 的网络/DNS 字段，不覆盖最新订户/SQN。 / `docker load` only imports an image; actual rollback must replace the container and restore the original orchestration plus profile network/DNS fields, without overwriting current subscriber/SQN data.
5. 旧镜像/容器（`ltesystem-dep:1.0`、`ltesystem-v1-backup`）在确认新版稳定一周后再清：`sudo docker rm ltesystem-v1-backup; sudo docker rmi docker.skygo/addx/ltesystem-dep:1.0` / Remove old images/containers (`ltesystem-dep:1.0`, `ltesystem-v1-backup`) only after the new version proves stable for a week: `sudo docker rm ltesystem-v1-backup; sudo docker rmi docker.skygo/addx/ltesystem-dep:1.0`

## 5. entrypoint 启动流程 entrypoint Boot Flow

1. `select-uhd-fpga $UHD_FPGA`（默认 `auto`=不动当前镜像） / `select-uhd-fpga $UHD_FPGA` (default `auto` = leave the current image alone)
2. `mkdir -p /data/conf /data/log`；`sib/rb.conf` **每次跟随镜像覆盖**；`user_db.csv`/`wordlist.list` **缺失才复制**（你的卡库永不被覆盖）；`rr.conf` 不由这里管（每次 `/start` 按频段渲染） / `mkdir -p /data/conf /data/log`; `sib/rb.conf` is **overwritten following the image every time**; `user_db.csv`/`wordlist.list` wordlist are **copied only when missing** (your SIM database is never overwritten); `rr.conf` is not handled here (re-rendered per band on each `/start`)
3. `service pcscd start`（无读卡器时失败也继续） / `service pcscd start` (continues even if it fails with no reader)
4. 打印 `uhd_find_devices` / `bladeRF-cli info` 供排障 / Prints `uhd_find_devices` / `bladeRF-cli info` for troubleshooting
5. `exec lte-system`（PID 1，接管信号，`docker stop` 优雅退出） / `exec lte-system` (PID 1, takes over signals, `docker stop` exits gracefully)

## 6. 构建排障 Build Troubleshooting

| 现象 Phenomenon | 处理 Handling |
|---|---|
| `Unable to locate package ...` | 包名随 Ubuntu 版本变，先在宿主 `apt-cache policy <包>` 核实再改 Dockerfile / Package names vary by Ubuntu release; verify on the host with `apt-cache policy <pkg>` before editing the Dockerfile |
| `Boost required` / cmake 报错 / cmake error | 补 `libboost-*-dev`（srsRAN 要 program-options + system + thread + test） / Add `libboost-*-dev` (srsRAN needs program-options + system + thread + test) |
| `git: command not found`（某 RUN 层） / (in some RUN layer) | 该层基座缺 git，加到 apt 列表 / That layer's base lacks git; add it to the apt list |
| 构建卡住不动 / Build stuck | srsRAN `make -j$(nproc)` 正常要 10–20 分钟，看 `docker compose build` 输出确认在编译 / srsRAN `make -j$(nproc)` normally takes 10–20 min; watch `docker compose build` output to confirm compiling |
| 磁盘告警 / Disk warning | 构建峰值多占 ~8GB；`sudo docker system df`，`sudo docker builder prune` 清陈旧缓存 / Peak build usage adds ~8GB; `sudo docker system df`, `sudo docker builder prune` to clear stale cache |

---
**导航 Navigation:** [仓库根 Repo Root](../../README.md) · [文档索引 Docs](../../docs/README.md) · [DEPLOY](../../docs/DEPLOY.md) · [SDR](../../docs/SDR.md)
