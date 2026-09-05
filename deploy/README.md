# deploy — 服务器部署物

- `docker/Dockerfile` — 三段构建：srsRAN_4G（`release_23_11`，Ubuntu 22.04）→ Go 二进制 → 运行镜像。在**服务器上**构建，本地 Windows 不跑 docker
- `docker/docker-compose.yml` — 正式启动方式：`host` 网络 + `privileged` + USB 映射，`UHD_FPGA: compat` 适配 BlackSDR 兼容板
- `docker/entrypoint.sh` — 容器启动：选 FPGA → seeding `/data`（`sib,rr,rb.conf` 跟随镜像覆盖；`user_db.csv`/`wordlist` 仅缺失时复制）→ 起 `pcscd` → exec Go 服务
- `docker/select-uhd-fpga.sh` — `stock`（正版 B210）/`compat`（兼容板）镜像切换

部署步骤见 `docs/DEPLOY.md`，日常用 `scripts/deploy_to_ubuntu.sh` 从本地同步代码到服务器。
