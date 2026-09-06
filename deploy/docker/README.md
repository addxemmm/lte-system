# Docker 指南 Docker Guide（Dockerfile · Compose · 镜像版本 / Dockerfile · Compose · Image Versions）

运行位置：**SDR 服务器**（开发机只改代码，不跑 docker）。日常部署看 [`docs/DEPLOY.md`](../../docs/DEPLOY.md)，这里讲镜像本身是怎么构成、怎么迭代的。
Runs on: **the SDR server** (dev machines only edit code, never run docker). For daily deploy see [`docs/DEPLOY.md`](../../docs/DEPLOY.md); this doc covers how the image itself is built and iterated.

## 1. 文件一览 Files at a Glance

| 文件 File | 作用 Purpose |
|---|---|
| [`deploy/docker/Dockerfile`](Dockerfile) | 三段构建，产出 `ltesystem-dep:<VERSION>` / Three-stage build producing `ltesystem-dep:<VERSION>` |
| [`deploy/docker/docker-compose.yml`](docker-compose.yml) | 正式启动编排（host 网络 + privileged + USB + 数据卷） / Production orchestration (host network + privileged + USB + data volume) |
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
  - 曾踩过的坑（已修，升级依赖时注意）：运行时包名是 `libmbedtls14` 不是 `libmbedtls7`；srsRAN 还要 `libboost-system/thread/test-dev`；runtime 层要装 `git`（给 pysim 用）。 / Past pitfalls (fixed, watch out when upgrading deps): the runtime package name is `libmbedtls14` not `libmbedtls7`; srsRAN also needs `libboost-system/thread/test-dev`; the runtime layer must install `git` (for pysim).
- **go-builder**：`CGO_ENABLED=0` 静态编译 [`cmd/server`](../../cmd/server)。改 Go 代码只会重跑这一段及之后（约 1–2 分钟），srsRAN 层走缓存。 / **go-builder**: statically builds [`cmd/server`](../../cmd/server) with `CGO_ENABLED=0`. Go-only changes re-run just this and later stages (about 1–2 min); the srsRAN layer hits cache.
- **runtime**：只装运行库 + 工具（`srsepc/srsenb` 从 builder 拷、`tcpdump/tshark/hashcat/pcscd`、UHD 镜像下载 + 兼容板 FPGA 覆盖、`pysim` 全量 clone、Go 二进制、[`configs/`](../../configs)、`entrypoint.sh`）。 / **runtime**: runtime libs + tools only (`srsepc/srsenb` copied from builder, `tcpdump/tshark/hashcat/pcscd`, UHD image download + compatible board FPGA overlay, full `pysim` clone, Go binary, [`configs/`](../../configs), `entrypoint.sh`).
  - `ENV LTE_CONFIG=/app/configs/app.yaml`：构建时由 `app.yaml.example` 物化，**改配置改仓库里的 example 文件**，直接改容器内文件重建即丢。 / `ENV LTE_CONFIG=/app/configs/app.yaml`: materialized from `app.yaml.example` at build time; **edit the example file in the repo to change config** — edits inside the container are lost on rebuild.
  - `EXPOSE 8081` 只是声明，实际靠 host 网络对外。 / `EXPOSE 8081` is declarative only; external access actually relies on host networking.

## 3. docker-compose.yml 逐项解释 docker-compose.yml Explained Item by Item

```yaml
build: { context: ../.., dockerfile: deploy/docker/Dockerfile }  # 相对本文件的仓库根
image: ltesystem-dep:2.0        # 与 VERSION 同步，改版时一起改
container_name: ltesystem
network_mode: host              # 必需：srsRAN 自建 srs_spgw_sgi 网卡 + 要上行口名，bridge 不行
privileged: true                # 必需：建网卡、实时线程、访问 USB
volumes:
  - /dev/bus/usb:/dev/bus/usb   # USRP + 读卡器直通
  - lte-data:/data              # 配置/卡库/日志/抓包持久化（重建不丢）
environment:
  UHD_FPGA: compat              # 本机兼容板；正版 B210 改 stock
restart: "no"                   # 手动启停，不开机自启 / manual start-stop, no auto-start
logging: { max-size: 50m, max-file: 5 }  # 防 docker 日志撑爆盘
```

构建上下文是相对本文件的仓库根；`image` 与 `VERSION` 同步，改版时一起改。
The build context is the repo root relative to this file; `image` stays in sync with `VERSION`, change both on release.

`network_mode: host` 必需：srsRAN 自建 `srs_spgw_sgi` 网卡 + 要上行口名，bridge 不行；`privileged: true` 必需：建网卡、实时线程、访问 USB。
`network_mode: host` is required: srsRAN creates its own `srs_spgw_sgi` NIC + needs the uplink interface name, bridge won't work; `privileged: true` is required: create NICs, realtime threads, access USB.

卷挂载：`/dev/bus/usb:/dev/bus/usb` 给 USRP + 读卡器直通；`lte-data:/data` 给配置/卡库/日志/抓包持久化（重建不丢）。
Volumes: `/dev/bus/usb:/dev/bus/usb` passes through USRP + reader; `lte-data:/data` persists config/SIM database/logs/packet capture (survives rebuilds).

环境：`UHD_FPGA: compat` 指本机兼容板；正版 B210 改 `stock`。`logging` 防 docker 日志撑爆盘。
Environment: `UHD_FPGA: compat` means this machine's compatible board; genuine B210 uses `stock`. `logging` keeps docker logs from filling the disk.

常用命令（服务器上，`~/lte-system` 下）：
Common commands (on the server, under `~/lte-system`):

```bash
sudo docker compose -f deploy/docker/docker-compose.yml up -d --build  # 构建+启动
sudo docker compose -f deploy/docker/docker-compose.yml up -d --force-recreate  # 光重建容器（镜像不变）
sudo docker compose -f deploy/docker/docker-compose.yml down   # 停+删容器（卷保留，配置不丢）
sudo docker logs --tail 200 ltesystem
sudo docker exec ltesystem <cmd>   # 进容器执行，如 uhd_find_devices
```

## 4. 版本迭代规范 Release Iteration Rules

1. 改代码/配置 → 本地 `go test ./...` 全绿。 / Change code/config → local `go test ./...` all green.
2. 根目录 `VERSION` 递增（如 `2.0` → `2.1`），`docker-compose.yml` 的 `image:` 同步改。 / Bump repo-root `VERSION` (e.g. `2.0` → `2.1`), update `image:` in `docker-compose.yml` in sync.
3. 服务器构建：`up -d --build`；再打 registry 别名保持旧命名延续：`sudo docker tag ltesystem-dep:2.1 docker.skygo/addx/ltesystem-dep:2.1` / Build on the server: `up -d --build`; then tag the registry alias to keep old naming: `sudo docker tag ltesystem-dep:2.1 docker.skygo/addx/ltesystem-dep:2.1`
4. 存 tarball 备份（1.5GB 级，`~` 下保留最近两个版本即可）： / Keep a tarball backup (1.5GB class, keep the latest two under `~`):
   ```bash
   sudo docker save ltesystem-dep:2.1 -o ~/ltesystem-dep-2.1.tar
   # 回滚：sudo docker load -i ~/ltesystem-dep-2.1.tar
   ```
   回滚：`sudo docker load -i ~/ltesystem-dep-2.1.tar`。 / Rollback: `sudo docker load -i ~/ltesystem-dep-2.1.tar`.
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
