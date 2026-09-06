# SDR 射频指南 Radio Guide（USRP B210 / bladeRF，服务器端运行 / server side）

## 1. USRP B210：正版 vs BlackSDR 兼容板 Genuine vs BlackSDR clone

正版 Ettus B210 与 BlackSDR 兼容板 FPGA 型号不同，镜像不可混用：
Genuine Ettus B210 and BlackSDR clone use different FPGAs; images are NOT interchangeable:

| 镜像 Image | 大小 Size | 位置 Location | 适用 Hardware | SHA256 头 Prefix |
|---|---|---|---|---|
| `usrp_b210_fpga.bin`（`ettus-B210-stock/` 下） | 4,224,632 B | [`firmware/uhd/ettus-B210-stock/`](../firmware/uhd/ettus-B210-stock) | 正版 B210 / genuine | `4ab68af9…` |
| `usrp_b210_fpga.bin`（`blacksdr-B210mini-clone/` 下） | 3,825,788 B | [`firmware/uhd/blacksdr-B210mini-clone/`](../firmware/uhd/blacksdr-B210mini-clone) | BlackSDR 兼容板 / clone | `8e2acce1…` |

两文件同名，**只看目录名 + `SHA256SUMS` 认身份**（`sha256sum -c`），详见 [`firmware/uhd/README.md`](../firmware/uhd/README.md)。
Same filename in both dirs — identify by **directory name + `SHA256SUMS`** (`sha256sum -c`), see [`firmware/uhd/README.md`](../firmware/uhd/README.md).

切换由 [`deploy/docker/select-uhd-fpga.sh`](../deploy/docker/select-uhd-fpga.sh) 完成，本质是覆盖 `UHD_IMAGES_DIR` 下的 `usrp_b210_fpga.bin`：
Switching is done by [`deploy/docker/select-uhd-fpga.sh`](../deploy/docker/select-uhd-fpga.sh), essentially overwriting `usrp_b210_fpga.bin` under `UHD_IMAGES_DIR`:

```bash
UHD_FPGA=compat sudo docker compose -f deploy/docker/docker-compose.yml up -d
sudo docker exec ltesystem select-uhd-fpga compat
sudo docker exec ltesystem select-uhd-fpga stock
sudo docker exec ltesystem md5sum /usr/share/uhd/images/usrp_b210_fpga*.bin
echo $UHD_IMAGES_DIR
```

`Dockerfile` 先跑 `/usr/lib/uhd/utils/uhd_images_downloader.py` 拉取 stock 镜像，再 `COPY firmware/uhd/*.bin` 覆盖。本机 `UHD_FPGA=compat`（见 `docker-compose.yml`）。
`Dockerfile` first runs `/usr/lib/uhd/utils/uhd_images_downloader.py` to fetch the stock image, then `COPY firmware/uhd/*.bin` to overwrite it. This machine uses `UHD_FPGA=compat` (see `docker-compose.yml`).

硬件要求：必须 USB3 口 + 充足供电（B210 满功率发射时 USB2/弱供电会掉设备）。判读：
Hardware requirements: USB3 port + sufficient power (B210 at full TX power drops off the bus on USB2/weak power). Check as follows:

```bash
sudo docker exec ltesystem uhd_find_devices
```

正常应含 `B210`/`B200`、serial、USB `3.0`；若为空/只有 `No UHD Devices Found` 则检查 `lsusb`、换线/换口/加 Hub 外接电源；若报 FPGA 不匹配则切 `stock/compat` 后 `docker restart ltesystem`。
Normal output contains `B210`/`B200`, serial, USB `3.0`; if empty or only `No UHD Devices Found`, check `lsusb`, change cable/port, or add an externally powered Hub; on FPGA mismatch switch `stock/compat` then `docker restart ltesystem`.

## 天线（重要，实测血泪） Antennas (Important, Hard-Learned)

- 必须用 **700–2700MHz 宽频 4G 天线**（之前验证过的船形桨板天线）。普通 **2.4GHz WiFi 棒状天线在 B7（2.68G）失配、B3（1.84G）几乎无辐射**，现象是：CPE 能连但延迟几秒、手机搜不到网——和基站配置无关，别在软件上浪费时间。 / You must use **700–2700MHz wideband 4G antennas** (the previously validated paddle panel antenna). Ordinary **2.4GHz WiFi whip antennas mismatch at B7 (2.68G) and barely radiate at B3 (1.84G)**; symptoms: CPE connects but lags by seconds, phones find no network — unrelated to eNB config, don't waste time on software.
- 两根都接 TX/RX 和 RX2 口并拧紧；手机测试时离天线 1–3 米（贴太近会饱和，太远更搜不到）。 / Connect both to TX/RX and RX2 and tighten; keep phones 1–3 m from antennas during tests (too close saturates, farther finds nothing).

## 2. bladeRF 设备 bladeRF Devices

支持 x40 / xA4 / 2.0 micro，对应 `device_name=bladerf`：
Supports x40 / xA4 / 2.0 micro with `device_name=bladerf`:

```bash
sudo docker exec ltesystem bladeRF-cli -e info
```

需根据型号将 `hostedx40.rbf` / `hostedxA4.rbf` / `hosted2.0micro.rbf` 放入 [`firmware/bladerf/`](../firmware/bladerf)（当前仅占位 `README.md`，有硬件后再 bake 到 `/etc/Nuand/bladeRF/`）。
Per your model, place the `hostedx40.rbf` / `hostedxA4.rbf` / `hosted2.0micro.rbf` firmware into [`firmware/bladerf/`](../firmware/bladerf) (currently only a placeholder `README.md`; bake into `/etc/Nuand/bladeRF/` once hardware is available).

## 3. `/start` 射频参数 `/start` Radio Parameters

```bash
curl -s -X POST http://192.0.2.10:8081/start \
  -H 'Content-Type: application/json' \
  -d '{"band":"41","apn":"srsapn","mcc":"001","mnc":"01","network":"eth0","sdr":"auto","device_args":"auto","tx_gain":80,"rx_gain":40}' ; echo
```

- `sdr`：`uhd`（B210）| `bladerf`（`device_name=bladerf`）| `zmq`（仿真）| `auto`（有 bladeRF 且无 B210 则选 bladeRF，否则按 [`configs/app.yaml.example`](../configs/app.yaml.example) 的 `default_sdr`）。 / `sdr`: `uhd` (B210) | `bladerf` (`device_name=bladerf`) | `zmq` (simulation) | `auto` (if bladeRF is present and no B210, choose bladeRF; otherwise follow `default_sdr` in [`configs/app.yaml.example`](../configs/app.yaml.example)).
- `device_args`：透传到 `enb.conf [rf] device_args`。`auto` + 检测到 B210 时服务端自动注入 VM-USB 稳定参数（`recv/send_frame_size=9232`，`num_recv/send_frames=64`）；显式传参永远优先；bladeRF/ZMQ 不受影响。 / `device_args`: passed through to `enb.conf [rf] device_args`. With `auto` + detected B210, the server auto-injects VM-USB stabilizing params (`recv/send_frame_size=9232`, `num_recv/send_frames=64`); explicit params always win; bladeRF/ZMQ are unaffected.
- `tx_gain/rx_gain`：覆盖 `default_tx_gain: 80` / `default_rx_gain: 40`，B210 建议从默认值起调，勿直接拉满。 / `tx_gain/rx_gain`: override `default_tx_gain: 80` / `default_rx_gain: 40`; for B210 start from the defaults, don't max out directly.
- `n_prb`：默认 `25`（5MHz）。虚拟机 USB 吞吐有限，10MHz（`50`）易出现 `Tx while waiting for EOB, timed out` 导致信号断续、手机搜不到网；裸金属可提到 `50`/`100`。 / `n_prb`: default `25` (5MHz). VM USB throughput is limited; 10MHz (`50`) easily hits `Tx while waiting for EOB, timed out`, causing choppy signal and phones finding no network; bare metal may raise to `50`/`100`.

> 虚拟机 USB 排查：`sudo docker exec ltesystem grep -c "timed out" /data/log/enb_run.log` —— 1 分钟内持续增长说明 USB 跟不上，先降 `n_prb` 到 `25`（默认已是），再检查宿主机负载与 USB3 直通。
> VM USB troubleshooting: `sudo docker exec ltesystem grep -c "timed out" /data/log/enb_run.log` — steady growth within 1 minute means USB can't keep up; lower `n_prb` to `25` (already the default) first, then check host load and USB3 passthrough.

> `network` 是服务器 uplink 网卡名（如 `eth0`/`enpXsY`），不是 `wlo1`（旧文档的笔记本 Wi-Fi 名）。用 `ip route get 8.8.8.8` 确认。
> `network` is the server uplink NIC name (e.g. `eth0`/`enpXsY`), not `wlo1` (the laptop Wi-Fi name from old docs). Confirm with `ip route get 8.8.8.8`.

## 4. 为何必须 host + privileged + USB 映射 Why host + privileged + USB Mapping Is Required

```yaml
network_mode: host
privileged: true
volumes: [/dev/bus/usb:/dev/bus/usb]
```

srsRAN 会创建 `srs_spgw_sgi` 网卡并需要宿主 uplink 接口名，bridge 网络拿不到；USRP/ACR1281U 经 `/dev/bus/usb` + `pcscd` 访问，无 `privileged` 会权限不足。开发机仅做编译，不跑射频。
srsRAN creates the `srs_spgw_sgi` NIC and needs the host uplink interface name, which bridge networking can't provide; USRP/ACR1281U are accessed via `/dev/bus/usb` + `pcscd`, and lack of `privileged` causes permission errors. Dev machines only compile, never run radio/RF.

## 5. 常见错误 Common Errors

| 现象 Phenomenon | 原因 / 处理 Cause / Handling |
|---|---|
| `No UHD Devices Found` | USB2/供电/线材；换 USB3，`lsusb` 确认，`docker logs ltesystem` 看启动探测 / USB2/power/cable issue; switch to USB3, confirm with `lsusb`, check the boot probe via `docker logs ltesystem` |
| `FPGA mismatch / failed to load` | 选错镜像；`select-uhd-fpga compat`（BlackSDR）或 `stock`（正版）后重启 / Wrong image; restart after `select-uhd-fpga compat` (BlackSDR compatible board) or `stock` (genuine) |
| `bladeRF-cli: No devices` | 未挂 USB / rbf 缺失；检查映射，[`firmware/bladerf`](../firmware/bladerf) 放入对应 `hosted*.rbf` / USB not passed through / rbf missing; check the mapping, place the matching `hosted*.rbf` into [`firmware/bladerf`](../firmware/bladerf) |
| `pcsc_scan: No readers` | `privileged`/USB 映射缺失；`service pcscd start`，`lsusb \| grep ACR128` / Missing `privileged`/USB mapping; `service pcscd start`, `lsusb \| grep ACR128` |
| `8081 connection refused` | 容器未起/端口占用；`docker ps`，`docker logs --tail 200 ltesystem` / Container down / port taken; `docker ps`, `docker logs --tail 200 ltesystem` |
| 手机搜不到网 / Phone finds no network | 先看 TX 稳定性（上条）；再确认频段手机支持、天线在 TX/RX 口、距离 1 米内、手动搜网多等几分钟 / Check TX stability first (previous rows); then confirm the phone supports the band, antennas are on TX/RX, distance is within 1 m, and wait minutes on manual search |
| 无信号/增益异常 / No signal / abnormal gain | `tx_gain/rx_gain` 过高/低；先用 80/40，天线接 TX/RX 口 / `tx_gain/rx_gain` too high/low; start with 80/40, antennas on the TX/RX port |

---
**导航 Navigation:** [文档索引 Docs](README.md) · [QUICKSTART](QUICKSTART.md) · [RULES](RULES.md) · [API v1](API.md) · [旧版API Legacy](API_LEGACY.md) · [DEPLOY](DEPLOY.md) · [SIM](SIM.md) · [SDR](SDR.md) · [MIGRATION](MIGRATION.md)
