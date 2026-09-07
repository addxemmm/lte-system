# 使用规则 / Usage rules

## 持久化与生命周期 / Persistence and lifecycle

| 内容 / Content | 位置 / Location | 语义 / Meaning |
|---|---|---|
| 启动配置 / Launch profile | `/data/last_start.json` | 复用配置，不自动发射 / Reuse configuration, no automatic RF start |
| 订户库 / Subscriber database | `/data/conf/user_db.csv` | 认证材料与 SQN，必须保密和保留 / Sensitive credentials and SQN; preserve privately |
| 日志、抓包 / Logs and captures | `/data/log/` | 历史证据，不等于在线状态 / Historical evidence, not current online state |
| EPC/eNB 会话 / EPC/eNB sessions | 当前进程 / Current processes | 重启后重建 / Re-established after restart |

一个容器运行一组 EPC/eNB，不等于只支持一个 UE。srsRAN 的核心网和基站维护多个独立用户上下文；旧文档“首台独占上网”的说法不正确，已删除。编译容量上限也不等于当前硬件可稳定支持的终端数。
One EPC/eNB pair per container does not mean one UE. The core and base station maintain multiple user contexts. The old statement that the first UE monopolizes Internet access was incorrect and has been removed. A compiled capacity limit is not a hardware performance guarantee.

## 标准操作 / Standard operations

```bash
BASE=http://127.0.0.1:8081
curl --fail "$BASE/api/v1/cell"
curl --fail "$BASE/api/v1/profile"
curl --fail "$BASE/api/v1/subscribers"
curl --fail "$BASE/api/v1/ues"
# 空闲时启动；不会覆盖保存的 DNS / Start only when idle; preserve saved DNS
curl --fail -X POST "$BASE/api/v1/cell" -H 'Content-Type: application/json' -d '{}'
# 结束 / Stop
curl --fail -X DELETE "$BASE/api/v1/cell"
```

API 请求字段优先于保存的 profile，profile 优先于服务端默认。仅发送想改的字段；Postman 中启动请求的默认 `{}` 用于复用，不是首次安装配置。
Request fields override the saved profile, which overrides server defaults. Send only intentional changes. Postman's default `{}` start body is for reuse, not first-time setup.

## 多设备管理 / Multi-device management

- 一个 IMSI 对应一个订户身份；每个终端使用不同 IMSI。相同 IMSI 的重复接入会替换上下文。
  Use a different IMSI per device. Duplicate attachment with one IMSI replaces its context.
- 订户清单是配置，UE 会话集合是运行证据。不要把最后一个 APN、IMSI、IP 的独立日志行拼接成一台设备。
  Subscriber inventory is configuration; the UE collection is runtime evidence. Never combine unrelated last-seen APN, IMSI and IP log lines into a device.
- 单 APN 默认承载与多 UE 是兼容的。APN UI“名称”不是 APN 标识；严格匹配发生于新承载请求，不会追溯修改旧连接。
  A single APN/default bearer supports multiple UEs. The APN UI label is not the identifier. Matching applies to new bearer requests, not retroactive changes to existing connections.
- 订户变更要求小区停止，并与启动互斥；禁止用旧 CSV 覆盖 EPC 更新后的 SQN。写卡不会自动启动基站。
  Subscriber mutations require a stopped cell and are serialized with startup. Never overwrite current EPC SQNs with an old CSV. SIM programming does not start the cell.

## 网络与升级 / Networking and upgrades

默认使用独立 [bridge 编排](../deploy/docker/docker-compose.bridge.yml)，仅发布 API TCP 8081；S1AP/GTP-U 在容器内部，不需要发布。出口 `auto` 在当前 namespace 解析，终端 PCO DNS 与容器自身 DNS 分开配置。
Use the standalone [bridge deployment](../deploy/docker/docker-compose.bridge.yml), publishing only API TCP 8081. Internal S1AP/GTP-U need no published ports. `auto` resolves the uplink in the current namespace; handset PCO DNS is separate from container DNS.

Stop 只回收本 Manager 的进程和规则；不要对主机执行全局 iptables flush 或 Docker volume prune。镜像升级保留实际 Compose 项目名、数据卷、BlackSDR compat FPGA 和旧镜像。详见 [DEPLOY](DEPLOY.md)。
Stop removes only this Manager's processes and rules. Never globally flush host iptables or prune Docker volumes. Preserve the Compose project, data volume, BlackSDR compat FPGA and old image when upgrading; see [DEPLOY](DEPLOY.md).

射频参数须与设备、天线和测试环境匹配。虚拟机 USB 默认 25 PRB；多 UE 共用无线资源，增加终端数不自动增加带宽。测试结束后显式停止小区。
Match RF settings to the hardware, antennas and test environment. VM USB defaults to 25 PRBs. Multiple UEs share radio resources; more devices do not create more bandwidth. Explicitly stop the cell after testing.
