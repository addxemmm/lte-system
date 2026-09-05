# Docker 指南（Dockerfile · Compose · 镜像版本）

> 运行位置：**Ubuntu 服务器**（本地 Windows 只改代码，不跑 docker）。日常部署看 `docs/DEPLOY.md`，这里讲镜像本身是怎么构成、怎么迭代的。

## 1. 文件一览

| 文件 | 作用 |
|---|---|
| `deploy/docker/Dockerfile` | 三段构建，产出 `ltesystem-dep:<VERSION>` |
| `deploy/docker/docker-compose.yml` | 正式启动编排（host 网络 + privileged + USB + 数据卷） |
| `deploy/docker/entrypoint.sh` | 容器启动流程：选 FPGA → seeding `/data` → 起 pcscd → exec Go 服务 |
| `deploy/docker/select-uhd-fpga.sh` | `stock`/`compat` FPGA 切换（见 `docs/SDR.md`） |
| `VERSION`（仓库根） | 镜像版本号，当前 `2.0`（从旧 `ltesystem-dep:1.0` 迭代而来） |

## 2. Dockerfile 三段在干什么

```
ubuntu:22.04 (srs-builder)  →  srsRAN_4G release_23_11 源码编译
golang:1.22-bookworm (go-builder) →  Go tool API 二进制
ubuntu:22.04 (runtime)      →  运行镜像（1.53GB，旧 1.0 镜像的一半不到）
```

- **srs-builder**：装编译依赖（含 UHD/bladeRF 可选），`git clone --branch ${SRSRAN_VERSION}` 后 `cmake Release && make && make install`。换 srsRAN 版本只改顶部 `ARG SRSRAN_VERSION`（tag 形如 `release_23_11`）。
  - 基座必须是 **22.04**：srsRAN_4G 在 gcc-13（24.04 默认）下编不过；22.04 自带 gcc-11 正好。
  - 曾踩过的坑（已修，升级依赖时注意）：运行时包名是 `libmbedtls14` 不是 `libmbedtls7`；srsRAN 还要 `libboost-system/thread/test-dev`；runtime 层要装 `git`（给 pysim 用）。
- **go-builder**：`CGO_ENABLED=0` 静态编译 `cmd/server`。改 Go 代码只会重跑这一段及之后（约 1–2 分钟），srsRAN 层走缓存。
- **runtime**：只装运行库 + 工具（`srsepc/srsenb` 从 builder 拷、`tcpdump/tshark/hashcat/pcscd`、UHD 镜像下载 + 兼容板 FPGA 覆盖、`pysim` 全量 clone、Go 二进制、`configs/`、`entrypoint.sh`）。
  - `ENV LTE_CONFIG=/app/configs/app.yaml`：构建时由 `app.yaml.example` 物化，**改配置改仓库里的 example 文件**，直接改容器内文件重建即丢。
  - `EXPOSE 8081` 只是声明，实际靠 host 网络对外。

## 3. docker-compose.yml 逐项解释

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
restart: unless-stopped
logging: { max-size: 50m, max-file: 5 }  # 防 docker 日志撑爆盘
```

常用命令（服务器上，`~/lte-system` 下）：

```bash
sudo docker compose -f deploy/docker/docker-compose.yml up -d --build  # 构建+启动
sudo docker compose -f deploy/docker/docker-compose.yml up -d --force-recreate  # 光重建容器（镜像不变）
sudo docker compose -f deploy/docker/docker-compose.yml down   # 停+删容器（卷保留，配置不丢）
sudo docker logs --tail 200 ltesystem
sudo docker exec ltesystem <cmd>   # 进容器执行，如 uhd_find_devices
```

## 4. 版本迭代规范

1. 改代码/配置 → 本地 `go test ./...` 全绿。
2. 根目录 `VERSION` 递增（如 `2.0` → `2.1`），`docker-compose.yml` 的 `image:` 同步改。
3. 服务器构建：`up -d --build`；再打 registry 别名保持旧命名延续：
   `sudo docker tag ltesystem-dep:2.1 docker.skygo/addx/ltesystem-dep:2.1`
4. 存 tarball 备份（1.5GB 级，`~` 下保留最近两个版本即可）：
   ```bash
   sudo docker save ltesystem-dep:2.1 -o ~/ltesystem-dep-2.1.tar
   # 回滚：sudo docker load -i ~/ltesystem-dep-2.1.tar
   ```
5. 旧镜像/容器（`ltesystem-dep:1.0`、`ltesystem-v1-backup`）在确认新版稳定一周后再清：
   `sudo docker rm ltesystem-v1-backup; sudo docker rmi docker.skygo/addx/ltesystem-dep:1.0`

## 5. entrypoint 启动流程

1. `select-uhd-fpga $UHD_FPGA`（默认 `auto`=不动当前镜像）
2. `mkdir -p /data/conf /data/log`；`sib/rb.conf` **每次跟随镜像覆盖**；`user_db.csv`/`wordlist.list` **缺失才复制**（你的卡库永不被覆盖）；`rr.conf` 不由这里管（每次 `/start` 按频段渲染）
3. `service pcscd start`（无读卡器时失败也继续）
4. 打印 `uhd_find_devices` / `bladeRF-cli info` 供排障
5. `exec lte-system`（PID 1，接管信号，`docker stop` 优雅退出）

## 6. 构建排障

| 现象 | 处理 |
|---|---|
| `Unable to locate package ...` | 包名随 Ubuntu 版本变，先在宿主 `apt-cache policy <包>` 核实再改 Dockerfile |
| `Boost required` / cmake 报错 | 补 `libboost-*-dev`（srsRAN 要 program-options + system + thread + test） |
| `git: command not found`（某 RUN 层） | 该层基座缺 git，加到 apt 列表 |
| 构建卡住不动 | srsRAN `make -j$(nproc)` 正常要 10–20 分钟，看 `docker compose build` 输出确认在编译 |
| 磁盘告警 | 构建峰值多占 ~8GB；`sudo docker system df`，`sudo docker builder prune` 清陈旧缓存 |
