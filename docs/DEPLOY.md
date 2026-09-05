# 部署指南（SDR 主机 `192.168.100.199`，Linux 服务器运行）

> 本地（Windows）只负责代码编辑与 git 管理；一切构建、运行、射频验证都在 Ubuntu 服务器上执行。

## 1. 前置条件

- 主机：Ubuntu 22.04 x86_64，USB3 口可用，已接 USRP B210（BlackSDR 兼容板）+ ACR1281U 读卡器。
- 软件：`docker` + `docker compose plugin`，端口 `8081` 未被占用。
- 数据卷：`/data`（compose 命名卷 `lte-data` 挂载到 `/data`，含 `conf/`、`log/`、pcap）。

```bash
lsb_release -a
sudo docker --version
sudo docker compose version
lsusb | grep -E "B210|B200|ACR128|Nuand|bladeRF"
sudo ss -tlnp | grep 8081 || echo "8081 free"
```

## 2. 部署方式（均在服务器上执行）

### 方式 A：compose（推荐）

```bash
cd ~/lte-system
sudo docker compose -f deploy/docker/docker-compose.yml up -d --build
sudo docker ps --filter name=ltesystem
curl -s http://127.0.0.1:8081/healthz; echo
```

`deploy/docker/docker-compose.yml` 已设 `network_mode: host`、`privileged: true`、`UHD_FPGA: compat`、`UHD_IMAGES_DIR: /usr/share/uhd/images`。

### 方式 B：docker run

```bash
sudo docker build -f deploy/docker/Dockerfile -t lte-system:latest .
sudo docker rm -f ltesystem || true
sudo docker run -d --name ltesystem --restart unless-stopped \
  --network host --privileged \
  -v /dev/bus/usb:/dev/bus/usb -v lte-data:/data \
  -e LTE_CONFIG=/app/configs/app.yaml -e UHD_FPGA=compat \
  -e UHD_IMAGES_DIR=/usr/share/uhd/images \
  lte-system:latest
```

### 本地只构建 Linux 二进制（不运行）

```powershell
# Windows 本地仅验证编译：
go test ./...
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o bin/lte-system-linux-amd64 ./cmd/server
Remove-Item Env:\GOOS; Remove-Item Env:\GOARCH
```

一键同步+重建（本地开发机执行，把代码推到服务器后在远端构建）：

```bash
./scripts/deploy_to_ubuntu.sh addx@192.168.100.199
```

## 3. UHD_FPGA 选择

BlackSDR 兼容板必须用 `compat`，正版 Ettus B210 用 `stock`：

```bash
grep UHD_FPGA deploy/docker/docker-compose.yml
sudo docker exec ltesystem select-uhd-fpga compat
sudo docker restart ltesystem
sudo docker exec ltesystem md5sum /usr/share/uhd/images/usrp_b210_fpga*.bin
```

## 4. 首次启动 seeding

`entrypoint.sh` 幂等复制（已存在不覆盖）：

- `/app/configs/user_db.csv.example` -> `/data/conf/user_db.csv`
- `/app/configs/sib.conf,rr.conf,drb.conf` -> `/data/conf/`
- `/app/configs/wordlist.list.example` -> `/data/wordlist.list`

```bash
sudo docker exec ltesystem ls -l /data/conf /data/wordlist.list
sudo docker exec ltesystem cat /data/conf/user_db.csv | head
```

改用户/小区配置后重启即可生效。

## 5. 验证（服务器上）

```bash
curl -s -X POST http://127.0.0.1:8081/stop; echo
curl -s http://127.0.0.1:8081/healthz; echo
curl -s http://127.0.0.1:8081/status; echo
BASE=http://127.0.0.1:8081 bash scripts/smoke.sh
```

`smoke.sh` 覆盖 `/healthz /status /stop /basicinfo /getfile /userupload`，`/start` 需真实射频硬件。

## 6. 升级 / 备份 / 排障

```bash
cd ~/lte-system && git pull && sudo docker compose -f deploy/docker/docker-compose.yml up -d --build
sudo docker logs --tail 200 ltesystem
sudo docker exec ltesystem uhd_find_devices
sudo docker exec ltesystem bladeRF-cli -e info
sudo docker exec ltesystem pcsc_scan -n || sudo docker exec ltesystem lsusb | grep ACR128
sudo docker exec ltesystem ls -lh /data/log
```

备份/恢复：

```bash
sudo docker run --rm -v lte-data:/data -v $PWD:/bak ubuntu tar czf /bak/lte-data-$(date +%F).tgz -C /data .
sudo docker run --rm -v lte-data:/data -v $PWD:/bak ubuntu tar xzf /bak/lte-data-2026-09-05.tgz -C /data
```

日志/pcap 在 `/data/log`：`srsLTE_enb.log`、`srsLTE_epc.log`、`lte_data.pcap`、`srsLTE_enb.pcap`、`srsLTE_enb_s1ap.pcap`、`srsLTE_epc.pcap`。
