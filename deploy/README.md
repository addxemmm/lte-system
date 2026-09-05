# deploy — 服务器部署物

> Docker 全指南见 **`docker/README.md`**（镜像三段构建、compose 逐项解释、版本迭代、entrypoint 流程、构建排障）。下面是索引：

- `docker/Dockerfile` — 三段构建：srsRAN_4G（`release_23_11`，Ubuntu 22.04）→ Go 二进制 → 运行镜像。在**服务器上**构建，本地 Windows 不跑 docker
- `docker/docker-compose.yml` — 正式启动方式：`host` 网络 + `privileged` + USB 映射，`UHD_FPGA: compat` 适配 BlackSDR 兼容板
- `docker/entrypoint.sh` — 容器启动：选 FPGA → seeding `/data`（`sib,rb.conf` 跟随镜像覆盖，`rr.conf` 每次 `/start` 按频段渲染；`user_db.csv`/`wordlist` 仅缺失时复制）→ 起 `pcscd` → exec Go 服务
- `docker/select-uhd-fpga.sh` — `stock`（正版 B210）/`compat`（兼容板）镜像切换

部署步骤见 `docs/DEPLOY.md`，日常同步代码到服务器见 `scripts/README.md`。
