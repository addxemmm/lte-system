# QUICKSTART — 无写卡器 + 已写卡，直接入网

前提：服务器已部署（见 `DEPLOY.md`），手头白卡已写好，主配置行：

```csv
ue3,mil,001012333333333,00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8001,000000001234,7,dynamic
```

该卡 IMSI `001012333333333` → MCC `001`、MNC `01`。**全程不需要 `/writesim`**（无读卡器时它固定返回 `message_id 2`，属正常现象）。

## 1. 确认种子配置已就位（服务器上）

```bash
sudo docker exec ltesystem grep -v '^#' /data/conf/user_db.csv
# 应看到 ue3,001012333333333,... 这一行；看不到就重建容器让 entrypoint 重新 seeding：
# sudo docker compose -f deploy/docker/docker-compose.yml up -d
```

## 2. 启动基站

```bash
curl -X POST http://127.0.0.1:8081/start -H 'Content-Type: application/json' \
  -d '{"band":"7","apn":"addxLTE","mcc":"001","mnc":"01","network":"eth0","full_net_name":"addxLTE","short_net_name":"addxLTE"}'
# {"status":true,"message_id":1,"message":"Start successfully"}
```

- `band` 按当地空闲频段和终端支持选（`1/3/5/7/8/34/39/40/41`）。本机（虚拟机 USB）实测推荐 `7`（FDD，射频最稳定）；TDD（`39/40/41`）eNB 能起来但上行有上游推导问题，手机先能用 7 就用 7
- `network` 是服务器 uplink 网卡名（本机实测为 `ens33`，`ip route get 8.8.8.8` 看 `dev` 后面的名字），不是旧文档里的 `wlo1`
- `apn` 必须和终端 APN 设置一致（现网 `addxLTE`）
- 终端 DNS 经 PCO 下发，默认 `8.8.8.8`；若上行网络过滤公网 DNS（IP 通、域名不通），`/start` 加 `"dns":"<网关或内网DNS>"`（本机用 `"dns":"192.168.100.1"`）
- 自定义终端显示的运营商名：加 `"full_net_name":"addxLTE","short_net_name":"addxLTE"`（默认 `srsRAN`，不传也行）

## 3. 手机入网设置

1. 白卡插入终端，手动搜网，选中 MCC `001` MNC `01` 的网络（会显示运营商名 `addxLTE`）
2. APN 新建：名称任意，APN 栏填 `addxLTE`，保存并选中
3. 打开数据，等待附着（一般 10–60 秒）

## 4. 确认入网成功

```bash
curl -X POST http://127.0.0.1:8081/basicinfo -H 'Content-Type: application/json' -d '{}'
# 成功示例：
# {"status":true,"message_id":1,"message":"Getting information success.",
#  "apn":"addxLTE","imsi":"001012333333333","ip":"172.16.0.2"}
```

对照金样本 `docs/samples/epc-ue-attached.log`：你的实时日志
`sudo docker exec ltesystem tail /data/log/srsLTE_epc.log` 应出现同样的
`ESM Info: APN` → `Found User 001012333333333` → `pool ip addr` 三连。

## 5. 抓包下载

```bash
curl -X POST http://127.0.0.1:8081/getfile -H 'Content-Type: application/json' \
  -d '{"fileid":0}' -o lte_data.pcap
# fileid: 0 业务流量 / 1 S1AP / 2 eNB / 3 EPC
```

## 6. 结束

```bash
curl -X POST http://127.0.0.1:8081/stop -H 'Content-Type: application/json' -d '{}'
```

## 失败速查

| 现象 | 查哪里 |
|---|---|
| `/basicinfo` 返回 `3 no UE connected` | 终端没附着：检查频段/APN/搜网是否选对；`tail /data/log/srsLTE_enb.log` 看小区是否起来 |
| 手机搜不到 `00101` | 按序排查：1)确认 TX 稳定（grep timed out 计数接近 0）2)手动搜网等足 3-5 分钟找数字网号 3)**优先用原厂系统手机测**（第三方 ROM 射频/搜网行为不可靠）4)换 band 3 再试 5)贴天线 0 距离还搜不到则与信号强度无关 6)iPhone 对测试卡挑剔，优先安卓/CPE |
| 日志 `UE Authentication Rejected` | `user_db.csv` 的 Key/OPc 与卡内不一致，核对 ue3 行 |
| 反复 attach 失败 | `SQN` 过期：把 ue3 行 `SQN` 改大一点（如 `000000001235`），重启容器再试 |
| 能附着但不能上网 | 按顺序查：①终端 ping 网关 `172.16.0.1` ②ping `8.8.8.8` 看延迟/丢包（上行 SNR 差会导致 TCP 瘫痪，先看 `PUSCH snr`）③`nslookup` 查 DNS（上行过滤公网 DNS 时 `/start` 加 `"dns"` 参数）④仍不行看 `RULES.md` 转发链自查 |
| `/start` 返回 `4` | USRP 没识别：`sudo docker exec ltesystem uhd_find_devices`，见 `SDR.md` |
