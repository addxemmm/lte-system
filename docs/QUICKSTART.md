# 无写卡器 + 已写卡，直接入网 QUICKSTART — No Card Reader + Programmed Test SIM, Direct Network Attach

> 本页命令用旧版根路径（最短）。新集成请用 `/api/v1` 等价接口，对照表见 `API.md §9`
> Commands on this page use the legacy root path (shortest). For new integrations, use the equivalent `/api/v1` interfaces; see the mapping table in `API.md §9`.
> （如 `POST /start` ↔ `POST /api/v1/cell`，`POST /basicinfo` ↔ `GET /api/v1/ue`）。
> (e.g. `POST /start` ↔ `POST /api/v1/cell`, `POST /basicinfo` ↔ `GET /api/v1/ue`).

前提：服务器已部署（见 `DEPLOY.md`），手头白卡已写好，主配置行：
Prerequisite: the server is already deployed (see `DEPLOY.md`), and your test SIM is already programmed. The primary configuration line is:

```csv
ue0,mil,001010123456789,00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8001,000000001234,7,dynamic
```

该卡 IMSI `001010123456789` → MCC `001`、MNC `01`。**全程不需要 `/writesim`**（无读卡器时它固定返回 `message_id 2`，属正常现象）。
The IMSI of this card `001010123456789` maps to MCC `001` and MNC `01`. **No `/writesim` is needed in the whole flow** (without a card reader it always returns `message_id 2`, which is normal).

## 1. 确认种子配置已就位 Confirm Seed Configuration Is Ready（服务器上 / On Server）

```bash
sudo docker exec ltesystem grep -v '^#' /data/conf/user_db.csv
# 应看到 ue0,001010123456789,... 这一行；文件在首次 /start 时自动 seeding，
# 若缺失则检查 lte-data 卷是否被删（docker volume ls / inspect）
```

## 2. 启动基站 Start the Self-hosted Base Station

```bash
curl -X POST http://127.0.0.1:8081/start -H 'Content-Type: application/json' \
  -d '{"band":"7","apn":"srsapn","mcc":"001","mnc":"01","network":"eth0","full_net_name":"MyLTE","short_net_name":"MyLTE"}'
# {"status":true,"message_id":1,"message":"Start successfully"}
```

- `band` 按当地空闲频段和终端支持选（`1/3/5/7/8/34/39/40/41`）。本机（虚拟机 USB）实测推荐 `7`（FDD，射频最稳定）；TDD（`39/40/41`）eNB 能起来但上行有上游推导问题，手机先能用 7 就用 7
  Select `band` by the local free band and UE/terminal device support (`1/3/5/7/8/34/39/40/41`). On this machine (VM USB) `7` is recommended (FDD, most stable radio/RF); TDD (`39/40/41`) eNB can start but uplink has an upstream derivation issue, so use 7 on the phone if it works
- `network` 是服务器 uplink 网卡名（`ip route get 8.8.8.8` 看 `dev` 后面的名字，如 `eth0`），不是家用 Wi-Fi 接口名
  `network` is the server uplink NIC name (check the name after `dev` in `ip route get 8.8.8.8`, e.g. `eth0`), not the home Wi-Fi interface name
- `apn` 必须和终端 APN 设置一致（如 `srsapn`） / `apn` must match the APN setting on the UE/terminal device (e.g. `srsapn`)
- 终端 DNS 经 PCO 下发，默认 `8.8.8.8`；若上行网络过滤公网 DNS（IP 通、域名不通），`/start` 加 `"dns":"<网关或内网DNS>"`（本机用 `"dns":"192.0.2.1"`）
  UE/terminal device DNS is delivered via PCO, default `8.8.8.8`; if the uplink network filters public DNS (IP works but domains fail), add `"dns":"<网关或内网DNS>"` to `/start` (on this machine use `"dns":"192.0.2.1"`)
- 自定义终端显示的运营商名：加 `"full_net_name":"MyLTE","short_net_name":"MyLTE"`（默认 `srsRAN`，不传也行）
  Carrier name shown on the UE/terminal device: add `"full_net_name":"MyLTE","short_net_name":"MyLTE"` (default `srsRAN`, optional)

## 3. 手机入网设置 UE Network Attach Setup

1. 白卡插入终端，手动搜网，选中 MCC `001` MNC `01` 的网络（会显示运营商名，即你设的 `full_net_name`）
   Insert the test SIM into the UE/terminal device, search networks manually, and select the network with MCC `001` MNC `01` (it shows the carrier name, i.e. your `full_net_name`)
2. APN 新建：名称任意，APN 栏填与基站一致的值（如 `srsapn`），保存并选中
   Create a new APN: any name, fill the APN field with the same value as the base station (e.g. `srsapn`), save and select it
3. 打开数据，等待附着（一般 10–60 秒） / Turn on mobile data and wait for attach (usually 10–60 seconds)

## 4. 确认入网成功 Confirm Successful Network Attach

```bash
curl -X POST http://127.0.0.1:8081/basicinfo -H 'Content-Type: application/json' -d '{}'
# 成功示例：
# {"status":true,"message_id":1,"message":"Getting information success.",
#  "apn":"srsapn","imsi":"001010123456789","ip":"172.16.0.2"}
```

对照金样本 [`docs/samples/epc-ue-attached.log`](samples/epc-ue-attached.log)：你的实时日志
`sudo docker exec ltesystem tail /data/log/srsLTE_epc.log` 应出现同样的
`ESM Info: APN` → `Found User 001010123456789` → `pool ip addr` 三连。
Compare with the golden sample [`docs/samples/epc-ue-attached.log`](samples/epc-ue-attached.log): your live log from
`sudo docker exec ltesystem tail /data/log/srsLTE_epc.log` should show the same
`ESM Info: APN` → `Found User 001010123456789` → `pool ip addr` triple.

## 5. 抓包下载 Download Packet Capture

```bash
curl -X POST http://127.0.0.1:8081/getfile -H 'Content-Type: application/json' \
  -d '{"fileid":0}' -o lte_data.pcap
# fileid: 0 业务流量 / 1 S1AP / 2 eNB / 3 EPC
```

## 6. 结束 Stop

```bash
curl -X POST http://127.0.0.1:8081/stop -H 'Content-Type: application/json' -d '{}'
```

## 失败速查 Troubleshooting Quick Reference

| 现象 / Symptom | 查哪里 / Where to Check |
|---|---|
| `/basicinfo` 返回 `3 no UE connected` / `/basicinfo` returns `3 no UE connected` | 终端没附着：检查频段/APN/搜网是否选对；`tail /data/log/srsLTE_enb.log` 看小区是否起来 / UE/terminal device not attached: check band/APN/network selection; use `tail /data/log/srsLTE_enb.log` to see if the cell is up |
| 手机搜不到 `00101` / Phone cannot find `00101` | 按序排查：1)确认 TX 稳定（grep timed out 计数接近 0）2)手动搜网等足 3-5 分钟找数字网号 3)**优先用原厂系统手机测**（第三方 ROM 射频/搜网行为不可靠）4)换 band 3 再试 5)贴天线 0 距离还搜不到则与信号强度无关 6)iPhone 对测试卡挑剔，优先安卓/CPE / Check in order: 1) confirm TX is stable (grep timed out count near 0) 2) manual network search, wait a full 3-5 minutes for the numeric network ID 3) **prefer a phone with stock ROM for testing** (third-party ROM radio/RF/network-search behavior is unreliable) 4) retry with band 3 5) if it is still not found with the antenna at 0 distance, it is not about signal strength 6) iPhone is picky with test SIMs, prefer Android/CPE |
| 日志 `UE Authentication Rejected` / Log shows `UE Authentication Rejected` | `user_db.csv` 的 Key/OPc 与卡内不一致，核对 ue0 行 / Key/OPc in `user_db.csv` does not match the card, check the ue0 line |
| 反复 attach 失败 / Repeated attach failures | `SQN` 过期：把 ue0 行 `SQN` 改大一点（如 `000000001235`），重启容器再试 / `SQN` expired: increase `SQN` in the ue0 line (e.g. `000000001235`) and restart the container |
| 能附着但不能上网 / Attached but no internet | 按顺序查：①终端 ping 网关 `172.16.0.1` ②ping `8.8.8.8` 看延迟/丢包（上行 SNR 差会导致 TCP 瘫痪，先看 `PUSCH snr`）③`nslookup` 查 DNS（上行过滤公网 DNS 时 `/start` 加 `"dns"` 参数）④仍不行看 `RULES.md` 转发链自查 / Check in order: 1) from the UE/terminal device ping the gateway `172.16.0.1` 2) ping `8.8.8.8` for latency/loss (poor uplink SNR stalls TCP, check `PUSCH snr` first) 3) `nslookup` for DNS (if uplink filters public DNS, add `"dns"` to `/start`) 4) if still failing, see the forwarding-chain self-check in `RULES.md` |
| `/start` 返回 `4` / `/start` returns `4` | USRP 没识别：`sudo docker exec ltesystem uhd_find_devices`，见 `SDR.md` / USRP not detected: `sudo docker exec ltesystem uhd_find_devices`, see `SDR.md` |
| 启动后 `status` 全 false / status all false right after start | eNB 初始化失败会直接报错（看 `/data/log/enb_run.log` 尾）；`lsusb` 无 B210 则重插 USB 查供电换口 / eNB init failure fails fast with the log tail; B210 missing from `lsusb` means re-plug USB and check power/port |

---
**导航 Navigation:** [文档索引 Docs](README.md) · [QUICKSTART](QUICKSTART.md) · [RULES](RULES.md) · [API v1](API.md) · [旧版API Legacy](API_LEGACY.md) · [DEPLOY](DEPLOY.md) · [SIM](SIM.md) · [SDR](SDR.md) · [MIGRATION](MIGRATION.md)
