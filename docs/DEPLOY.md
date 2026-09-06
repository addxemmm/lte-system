# 部署指南 Deploy Guide（SDR 主机 `192.0.2.10`，Linux 服务器运行 / SDR host `192.0.2.10`, runs on a Linux server）

开发机（Windows/macOS/Linux 均可）只负责代码编辑与 git 管理；一切构建、运行、射频验证都在 SDR 服务器上执行。
Dev machines (Windows/macOS/Linux) only handle code editing and git; all builds, runs, and radio validation execute on the SDR server.

想了解镜像内部构造/版本迭代见 **[`deploy/docker/README.md`](../deploy/docker/README.md)**。
For image internals/version iteration see **[`deploy/docker/README.md`](../deploy/docker/README.md)**.

## 1. 前置条件 Prerequisites

- 主机：Ubuntu 22.04 x86_64，USB3 口可用，已接 USRP B210（BlackSDR 兼容板）+ ACR1281U 读卡器。 / Host: Ubuntu 22.04 x86_64 with a usable USB3 port, USRP B210 (BlackSDR compatible board) + ACR1281U reader attached.
- 软件：`docker` + `docker compose plugin`，端口 `8081` 未被占用。 / Software: `docker` + `docker compose plugin`, port `8081` free.
- 数据卷：`/data`（compose 命名卷 `lte-data` 挂载到 `/data`，含 `conf/`、`log/`、pcap）。 / Data volume: `/data` (compose named volume `lte-data` mounted at `/data`, containing `conf/`, `log/`, pcap).

```bash
lsb_release -a
sudo docker --version
sudo docker compose version
lsusb | grep -E "B210|B200|ACR128|Nuand|bladeRF"
sudo ss -tlnp | grep 8081 || echo "8081 free"
```

## 2. 部署方式 Deploy Methods（均在服务器上执行 / all run on the server）

### 方式 A：compose（推荐） Method A: compose (Recommended)

```bash
cd ~/lte-system
sudo docker compose -f deploy/docker/docker-compose.yml up -d --build
sudo docker ps --filter name=ltesystem
curl -s http://127.0.0.1:8081/healthz; echo
```

[`deploy/docker/docker-compose.yml`](../deploy/docker/docker-compose.yml) 已设 `network_mode: host`、`privileged: true`、`UHD_FPGA: compat`、`UHD_IMAGES_DIR: /usr/share/uhd/images`。
[`deploy/docker/docker-compose.yml`](../deploy/docker/docker-compose.yml) already sets `network_mode: host`, `privileged: true`, `UHD_FPGA: compat`, `UHD_IMAGES_DIR: /usr/share/uhd/images`.

### 方式 B：docker run Method B: docker run

```bash
sudo docker build -f deploy/docker/Dockerfile -t ltesystem-dep:2.0 .
sudo docker tag ltesystem-dep:2.0 docker.skygo/addx/ltesystem-dep:2.0
sudo docker rm -f ltesystem || true
sudo docker run -d --name ltesystem --restart no \
  --network host --privileged \
  -v /dev/bus/usb:/dev/bus/usb -v lte-data:/data \
  -e LTE_CONFIG=/app/configs/app.yaml -e UHD_FPGA=compat \
  -e UHD_IMAGES_DIR=/usr/share/uhd/images \
  ltesystem-dep:2.0
```

当前版本见根目录 `VERSION`（`2.0`，从旧 `ltesystem-dep:1.0` 迭代而来）。
Current version is in the repo-root `VERSION` (`2.0`, iterated from old `ltesystem-dep:1.0`).

### 本地只构建 Linux 二进制（不运行） Build the Linux Binary Locally Only (No Run)

```powershell
# Windows 本地仅验证编译：
go test ./...
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o bin/lte-system-linux-amd64 ./cmd/server
Remove-Item Env:\GOOS; Remove-Item Env:\GOARCH
```

Windows 本地仅验证编译，不运行。
Local Windows builds only verify compilation, never run.

一键同步+重建（本地开发机执行，把代码推到服务器后在远端构建）：
One-shot sync + rebuild (run on the local dev machine; pushes code to the server, then builds remotely):

```bash
./scripts/deploy_to_ubuntu.sh addx@192.0.2.10
```

## 3. UHD_FPGA 选择 UHD_FPGA Selection

BlackSDR 兼容板必须用 `compat`，正版 Ettus B210 用 `stock`：
BlackSDR compatible boards must use `compat`, genuine Ettus B210 uses `stock`:

```bash
grep UHD_FPGA deploy/docker/docker-compose.yml
sudo docker exec ltesystem select-uhd-fpga compat
sudo docker restart ltesystem
sudo docker exec ltesystem md5sum /usr/share/uhd/images/usrp_b210_fpga*.bin
```

## 4. 首次启动 seeding First-Boot Seeding

文件来源分两处（幂等，已有文件永不覆盖你的数据）：
Files come from two places (idempotent; existing files never overwrite your data):

- `entrypoint.sh`（容器启动时）：`sib/rb.conf` 跟随镜像覆盖（保证修复能生效）；`wordlist.list` 字典缺失才复制 / `entrypoint.sh` (at container boot): `sib/rb.conf` is overwritten following the image (so fixes take effect); `wordlist.list` wordlist is copied only when missing
- `/start`（首次调用时，[`internal/lte`](../internal/lte)）：`user_db.csv` 缺失才从 example 播种；`rr.conf` 每次按频段重新渲染 / `/start` (on first call, [`internal/lte`](../internal/lte)): `user_db.csv` is seeded from the example only when missing; `rr.conf` is re-rendered per band every time

```bash
sudo docker exec ltesystem ls -l /data/conf /data/wordlist.list
sudo docker exec ltesystem cat /data/conf/user_db.csv | head
```

改卡库（`user_db.csv`）后必须 `/stop` 再 `/start` 才生效（EPC 只在启动时读库）；改小区静态配置（`sib/rb.conf`）重建容器即生效。
After editing the SIM database (`user_db.csv`), `/stop` then `/start` is required (EPC reads the database only at boot); editing static cell config (`sib/rb.conf`) takes effect on container rebuild.

## 5. 验证 Verification（服务器上 / on the server）

```bash
curl -s -X POST http://127.0.0.1:8081/stop; echo
curl -s http://127.0.0.1:8081/healthz; echo
curl -s http://127.0.0.1:8081/status; echo
BASE=http://127.0.0.1:8081 bash scripts/smoke.sh
```

`smoke.sh` 覆盖 `/healthz /status /stop /basicinfo /getfile /userupload`，`/start` 需真实射频硬件。
`smoke.sh` covers `/healthz /status /stop /basicinfo /getfile /userupload`; `/start` needs real radio/RF hardware.

## 6. 升级 / 备份 / 排障 Upgrade / Backup / Troubleshooting

```bash
cd ~/lte-system && git pull && sudo docker compose -f deploy/docker/docker-compose.yml up -d --build
sudo docker logs --tail 200 ltesystem
sudo docker exec ltesystem uhd_find_devices
sudo docker exec ltesystem bladeRF-cli -e info
sudo docker exec ltesystem pcsc_scan -n || sudo docker exec ltesystem lsusb | grep ACR128
sudo docker exec ltesystem ls -lh /data/log
```

备份/恢复：
Backup/restore:

```bash
sudo docker run --rm -v lte-data:/data -v $PWD:/bak ubuntu tar czf /bak/lte-data-$(date +%F).tgz -C /data .
sudo docker run --rm -v lte-data:/data -v $PWD:/bak ubuntu tar xzf /bak/lte-data-2026-09-05.tgz -C /data
```

日志/抓包在 `/data/log`：`srsLTE_enb.log`、`srsLTE_epc.log`、`lte_data.pcap`、`srsLTE_enb.pcap`、`srsLTE_enb_s1ap.pcap`、`srsLTE_epc.pcap`。
Logs / packet capture live in `/data/log`: `srsLTE_enb.log`, `srsLTE_epc.log`, `lte_data.pcap`, `srsLTE_enb.pcap`, `srsLTE_enb_s1ap.pcap`, `srsLTE_epc.pcap`.

---
**导航 Navigation:** [文档索引 Docs](README.md) · [QUICKSTART](QUICKSTART.md) · [RULES](RULES.md) · [API v1](API.md) · [旧版API Legacy](API_LEGACY.md) · [DEPLOY](DEPLOY.md) · [SIM](SIM.md) · [SDR](SDR.md) · [MIGRATION](MIGRATION.md)
