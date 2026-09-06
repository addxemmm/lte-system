# deploy 部署物 Deployment Artifacts

服务器部署物。
Server deploy artifacts.

> Docker 全指南见 **`docker/README.md`**（镜像三段构建、compose 逐项解释、版本迭代、entrypoint 流程、构建排障）。下面是索引：
> Full Docker guide: **`docker/README.md`** (three-stage image build, compose field-by-field, version iteration, entrypoint flow, build troubleshooting). Index below:

- `docker/Dockerfile` — 三段构建：srsRAN_4G（`release_23_11`，Ubuntu 22.04）→ Go 二进制 → 运行镜像。在**服务器上**构建，开发机不跑 docker / Three-stage build: srsRAN_4G (`release_23_11`, Ubuntu 22.04) → Go binary → runtime image. Build on the **server**; dev machine never runs docker
- `docker/docker-compose.yml` — 正式启动方式：`host` 网络 + `privileged` + USB 映射，`UHD_FPGA: compat` 适配 BlackSDR 兼容板compatible board / Official startup: `host` network + `privileged` + USB mapping, `UHD_FPGA: compat` for BlackSDR compatible board
- `docker/entrypoint.sh` — 容器启动：选 FPGA → 种子seed/seeding `/data`（`sib,rb.conf` 跟随镜像覆盖，`rr.conf` 每次 `/start` 按频段渲染；`user_db.csv`/`wordlist` 仅缺失时复制）→ 起 `pcscd` → exec Go 服务 / Container startup: select FPGA → seed/seeding `/data` (`sib,rb.conf` overwritten with image, `rr.conf` rendered per band on each `/start`; `user_db.csv`/`wordlist` copied only when missing) → start `pcscd` → exec Go service
- `docker/select-uhd-fpga.sh` — `stock`（正版 B210）/`compat`（兼容板compatible board）镜像切换 / `stock` (genuine B210) / `compat` (compatible board) image switching

部署步骤见 [`docs/DEPLOY.md`](../docs/DEPLOY.md)，日常同步代码到服务器见 [`scripts/README.md`](../scripts/README.md)。
For deploy steps see [`docs/DEPLOY.md`](../docs/DEPLOY.md); for daily code sync to server see [`scripts/README.md`](../scripts/README.md).

---
**导航 Navigation:** [仓库根 Repo Root](../README.md) · [文档索引 Docs](../docs/README.md) · [Docker 详解 Docker Guide](docker/README.md)
