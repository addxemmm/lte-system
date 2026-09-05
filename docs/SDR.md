# SDR 射频指南（USRP B210 / bladeRF，服务器端运行）

## 1. USRP B210：正版 vs BlackSDR 兼容板

正版 Ettus B210 与 BlackSDR 兼容板 FPGA 型号不同，镜像不可混用：

| 镜像 | 大小 | 位置 | 适用 |
|---|---|---|---|
| `usrp_b210_fpga.stock.bin` | 约 4.2MB | `firmware/uhd/` | 正版 B210 |
| `usrp_b210_fpga.blacksdr_compat.bin` | 约 3.8MB | `firmware/uhd/` | BlackSDR 兼容板 |

切换由 `deploy/docker/select-uhd-fpga.sh` 完成，本质是覆盖 `UHD_IMAGES_DIR` 下的 `usrp_b210_fpga.bin`：

```bash
UHD_FPGA=compat sudo docker compose -f deploy/docker/docker-compose.yml up -d
sudo docker exec ltesystem select-uhd-fpga compat
sudo docker exec ltesystem select-uhd-fpga stock
sudo docker exec ltesystem md5sum /usr/share/uhd/images/usrp_b210_fpga*.bin
echo $UHD_IMAGES_DIR
```

`Dockerfile` 先跑 `/usr/lib/uhd/utils/uhd_images_downloader.py` 拉取 stock 镜像，再 `COPY firmware/uhd/*.bin` 覆盖。本机 `UHD_FPGA=compat`（见 `docker-compose.yml`）。

硬件要求：必须 USB3 口 + 充足供电（B210 满功率发射时 USB2/弱供电会掉设备）。判读：

```bash
sudo docker exec ltesystem uhd_find_devices
```

正常应含 `B210`/`B200`、serial、USB `3.0`；若为空/只有 `No UHD Devices Found` 则检查 `lsusb`、换线/换口/加 Hub 外接电源；若报 FPGA 不匹配则切 `stock/compat` 后 `docker restart ltesystem`。

## 2. bladeRF

支持 x40 / xA4 / 2.0 micro，对应 `device_name=bladerf`：

```bash
sudo docker exec ltesystem bladeRF-cli -e info
```

需根据型号将 `hostedx40.rbf` / `hostedxA4.rbf` / `hosted2.0micro.rbf` 放入 `firmware/bladerf/`（当前仅占位 `README.md`，有硬件后再 bake 到 `/etc/Nuand/bladeRF/`）。

## 3. `/start` 射频参数

```bash
curl -s -X POST http://192.168.100.199:8081/start \
  -H 'Content-Type: application/json' \
  -d '{"band":"41","apn":"skygoapn","mcc":"001","mnc":"01","network":"eth0","sdr":"auto","device_args":"auto","tx_gain":80,"rx_gain":40}' ; echo
```

- `sdr`：`uhd`（B210）| `bladerf`（`device_name=bladerf`）| `zmq`（仿真）| `auto`（有 bladeRF 且无 B210 则选 bladeRF，否则按 `configs/app.yaml.example` 的 `default_sdr`）。
- `device_args`：透传到 `enb.conf [rf] device_args`，`auto` 即用 `default_device_args`。
- `tx_gain/rx_gain`：覆盖 `default_tx_gain: 80` / `default_rx_gain: 40`，B210 建议从默认值起调，勿直接拉满。

> `network` 是服务器 uplink 网卡名（如 `eth0`/`enpXsY`），不是 `wlo1`（旧文档的笔记本 Wi-Fi 名）。用 `ip route get 8.8.8.8` 确认。

## 4. 为何必须 host + privileged + USB 映射

```yaml
network_mode: host
privileged: true
volumes: [/dev/bus/usb:/dev/bus/usb]
```

srsRAN 会创建 `srs_spgw_sgi` 网卡并需要宿主 uplink 接口名，bridge 网络拿不到；USRP/ACR1281U 经 `/dev/bus/usb` + `pcscd` 访问，无 `privileged` 会权限不足。本地 Windows 仅做编译，不跑射频。

## 5. 常见错误

| 现象 | 原因 / 处理 |
|---|---|
| `No UHD Devices Found` | USB2/供电/线材；换 USB3，`lsusb` 确认，`docker logs ltesystem` 看启动探测 |
| `FPGA mismatch / failed to load` | 选错镜像；`select-uhd-fpga compat`（BlackSDR）或 `stock`（正版）后重启 |
| `bladeRF-cli: No devices` | 未挂 USB / rbf 缺失；检查映射，`firmware/bladerf` 放入对应 `hosted*.rbf` |
| `pcsc_scan: No readers` | `privileged`/USB 映射缺失；`service pcscd start`，`lsusb \| grep ACR128` |
| `8081 connection refused` | 容器未起/端口占用；`docker ps`，`docker logs --tail 200 ltesystem` |
| 无信号/增益异常 | `tx_gain/rx_gain` 过高/低；先用 80/40，天线接 TX/RX 口 |
