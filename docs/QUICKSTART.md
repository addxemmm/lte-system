# 快速入网 / Quick start

只使用 `/api/v1` 标准接口。以下命令在 SDR 服务器执行；已写好的 SIM 不需要再次写卡。
Use only the `/api/v1` standard API. Run these commands on the SDR server; an already programmed SIM does not need programming again.

## 1. 先看现状 / Inspect first

```bash
BASE=http://127.0.0.1:8081
curl --fail "$BASE/api/v1/cell"
curl --fail "$BASE/api/v1/profile"
curl --fail "$BASE/api/v1/subscribers"
```

订户清单表示允许接入的卡，不表示当前在线。不要打印完整 `user_db.csv`，其中含认证密钥。
The subscriber inventory lists provisioned SIMs, not current connections. Do not print the full `user_db.csv`: it contains authentication keys.

## 2. 复用配置启动 / Start using the saved profile

仅在小区停止时执行。`{}` 继承保存的频段、APN、DNS、增益和网络设置，避免示例覆盖已验证配置。
Run only while the cell is stopped. `{}` inherits the saved band, APN, DNS, gains and network settings instead of overwriting a working configuration.

```bash
curl --fail -X POST "$BASE/api/v1/cell" \
  -H 'Content-Type: application/json' -d '{}'
```

首次部署没有 profile 时，在 [API](API.md) 中填写完整启动参数。`network: "auto"` 选择当前容器的默认 IPv4 出口；bridge 内固定为 `eth0`。先验证实际 DNS 可达再配置 `dns`，不要把 Docker loopback resolver 下发给手机。
For a first deployment without a profile, supply the initial parameters documented in [API](API.md). `network: "auto"` selects the container's default IPv4 uplink, fixed to `eth0` in bridge mode. Validate DNS reachability before setting `dns`; never advertise Docker's loopback resolver to a handset.

## 3. 手机设置 / Handset setup

1. 各 SIM 使用独立 IMSI，且认证材料与订户库一致；两台设备使用同一 IMSI 会发生上下文替换，不是两个独立用户。
   Use a distinct IMSI for each SIM with matching subscriber credentials. Reusing one IMSI on two devices replaces the subscriber context rather than creating two independent users.
2. 选择本项目配置的测试网络。Android APN 的“名称”只是本地显示标签，可任意填写；“APN”栏才是网络标识，必须匹配服务端配置。
   Select the configured test network. Android's APN **Name** is a local display label; the **APN** field is the network identifier that must match the server.
3. 修改 APN 后保存并选中，关闭 Wi-Fi、开启蜂窝数据，再开关一次飞行模式，重新建立默认承载。已有承载不会因编辑 UI 字段立即改变。
   Save/select the APN, disable Wi-Fi, enable cellular data and toggle airplane mode to establish a new default bearer. Editing a UI field does not immediately change an existing bearer.

显式错误 APN 会被拒绝；没有发送 APN 与发送错误 APN 是不同情况，前者使用默认 APN。APN 不是用户认证密码，SIM AKA 认证仍然独立进行。该核心网提供 IPv4 数据，不提供 IMS/VoLTE 服务。
An explicitly incorrect APN is rejected. An omitted APN selects the default and is distinct from a wrong APN. APN selection is separate from SIM AKA authentication. This core provides IPv4 data, not IMS/VoLTE.

## 4. 检查多 UE 与联网 / Check multiple UEs and connectivity

```bash
curl --fail "$BASE/api/v1/ues"
curl --fail "$BASE/api/v1/diagnostics/connectivity"
```

使用会话集合核对每个 IMSI 的独立 IP 和状态。配置的订户数、核心网会话数、无线连接数与实际可上网设备数不等价；历史 Attach 日志也不代表当前在线。两部手机同时通过域名打开网页，才是端到端多终端验证。
Use the session collection to inspect each IMSI's IP and state. Provisioned subscribers, core sessions, radio connections and working Internet connections are different counts. Historical Attach logs do not prove current connectivity. Open websites by domain simultaneously on both handsets for an end-to-end multi-UE test.

若 IP 可达而域名失败，优先查 PCO DNS 和解析器；若无线掉线，查 eNB HARQ、射频时序和信号，而不是盲目扩大 PRB 或改 SIM 密钥。
If IP connectivity works but domains fail, inspect PCO DNS and resolver reachability. For radio drops, inspect eNB HARQ, RF timing and signal quality rather than blindly increasing PRBs or changing SIM credentials.

## 5. 停止 / Stop

```bash
curl --fail -X DELETE "$BASE/api/v1/cell"
```

修改订户或写卡前先停止小区，防止与 EPC 的 SQN 写回竞争。升级保留数据卷和回滚镜像，详见 [DEPLOY](DEPLOY.md)。
Stop the cell before subscriber changes or SIM programming to avoid racing EPC SQN writes. Preserve the data volume and rollback image during upgrades; see [DEPLOY](DEPLOY.md).
