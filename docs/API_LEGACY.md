# LTE-System 旧版接口参考（根路径，已冻结） Legacy API Reference (Root Path, Frozen)

> **新集成请用 [`/api/v1`](API.md)（标准 REST + OpenAPI）。**
> **For new integrations, use [`/api/v1`](API.md) (standard REST + OpenAPI).**
> 本文件记录冻结的旧版路由：行为不再变更（只修严重 bug），`message_id` 语义永久保留。
> This file documents the frozen legacy routes: behavior no longer changes (only critical bug fixes), and `message_id` semantics are preserved permanently.
> 新旧对照见 [`API.md` §迁移表](API.md#9-旧版新版对照表-legacy-to-v1-mapping)。
> For the old-to-new mapping, see the migration table in [`API.md`](API.md#9-旧版新版对照表-legacy-to-v1-mapping)。

# LTE-System API 参考手册（Go `:8081`） API Reference (Go `:8081`)

> 实现：[`internal/api/server.go`](../internal/api/server.go)。本文逐字对应实现——`message` 文案、`message_id`、字段名都以本文为准；改实现必须同步改本文 + `*_test.go`（见 `AGENTS.md`）。
> Implementation: [`internal/api/server.go`](../internal/api/server.go). This document matches the implementation word for word — `message` text, `message_id`, and field names are authoritative here; any implementation change must update this document plus `*_test.go` together (see `AGENTS.md`).

- 基地址 Base URL：`http://<服务器IP>:8081`（服务器本机 `http://127.0.0.1:8081`，局域网如 `http://192.0.2.10:8081`） / Base URL: `http://<服务器IP>:8081` (local: `http://127.0.0.1:8081`; LAN example: `http://192.0.2.10:8081`)
- 9 个旧接口全部 `POST` + JSON；新增 `GET /healthz`、`GET /status`、`GET /profile` / All 9 legacy endpoints use `POST` + JSON; plus new `GET /healthz`, `GET /status`, `GET /profile`
- **HTTP 状态码恒为 `200`**（含错误情况；`GET /getfile` 成功下载除外，见 §8）。成功失败只看 JSON 里的 `status` / **HTTP status is always `200`** (including errors; except successful `GET /getfile` downloads, see §8). Success or failure is determined only by `status` in the JSON
- 统一响应包络 Unified response envelope：`{"status": bool, "message_id": int, "message": str, ...扩展字段}` / `{"status": bool, "message_id": int, "message": str, ...extra fields}`
- `message_id = 0` 在所有接口都表示“通用失败，需查日志”（`sudo docker logs ltesystem` / `/data/log/`） / `message_id = 0` means "generic failure, check the logs" on all endpoints (`sudo docker logs ltesystem` / `/data/log/`)
- 请求体大小限制 Request body size limits：`/start` 1MB，`/getfile`·`/writesim` 64KB，上传类 8MB（超限 `upload failed: file too large`） / `/start` 1MB, `/getfile` and `/writesim` 64KB, upload types 8MB (over limit: `upload failed: file too large`)
- 服务端超时 Server timeouts：读 30s / 写 60s；`/start` 最长 90s（下面解释为什么）；`/writesim` 最长 180s / Read 30s / write 60s; `/start` up to 90s (explained below); `/writesim` up to 180s
- 非 POST 调 POST 接口：返回各接口自己的 `message_id 0` 文案（见各节），不是 404/405 / Calling a POST endpoint with non-POST returns that endpoint's own `message_id 0` text (see each section), not 404/405

目录 Contents：[§1 通用约定 General Conventions](#1-通用约定-general-conventions) · [§2 /start](#2-post-start启动基站并抓包-start-enodeb-and-packet-capture) · [§3 /stop](#3-post-stop停止-stop) · [§4 /basicinfo](#4-post-basicinfo读终端基础信息-read-ue-basic-info) · [§5 /crackapn](#5-post-crackapn开始破解-apn-密码注意会停基站-start-apn-dictionary-attack-stops-enodeb) · [§6 /getcrackresult](#6-post-getcrackresult取破解结果-get-cracking-result) · [§7 上传 Upload](#7-post-userupload--passwordupload上传卡库字典-upload-subscriber-db-and-wordlist) · [§8 /getfile](#8-post-getfile下载抓包文件-download-packet-capture-files) · [§9 /writesim](#9-post-writesim写卡有写卡器时才用-sim-programming-card-reader-required) · [§10 状态接口 Status](#10-状态接口-healthzstatusprofile-status-endpoints) · [§11 典型流程 Workflows](#11-典型流程-typical-workflows) · [§12 message_id 总表 Summary](#12-message_id-总表-summary)

---

## 1. 通用约定 General Conventions

### 1.1 响应包络 Response Envelope

```jsonc
{
  "status": true,          // 本次语义是否成功（注意不是 HTTP 状态）
  "message_id": 1,         // 机器可判读码，各接口独立编号
  "message": "Start successfully",  // 人读文案，逐字见各节
  // ... 各接口扩展字段（apn/imsi/ip/username/password/profile/ues 等）
}
```

### 1.2 缺失值用 `null` Missing Values Use `null`

`basicinfo` / `getcrackresult` 里没取到的字段返回 JSON `null`（不是空字符串也不是 `"NULL"`），客户端判空用 `== null`。

Fields not captured in `basicinfo` / `getcrackresult` return JSON `null` (not an empty string or `"NULL"`); clients should test emptiness with `== null`.

### 1.3 配置持久化（`/start` 空 body） Configuration Persistence (Empty `/start` Body)

每次 `/start` 成功后，**解析后的全量参数**存入 `/data/last_start.json`（随 `lte-data` 卷保留，重建容器不丢）。之后：

After each successful `/start`, the **parsed full parameter set** is saved to `/data/last_start.json` (kept in the `lte-data` volume, survives container rebuilds). Afterwards:

- `POST /start` 传 `{}` → 复用上次配置直接启动 / Pass `{}` to start directly with the previous configuration
- 只传个别字段（如 `{"band":"3"}`）→ 其余自动继承上次 / Pass only some fields (e.g. `{"band":"3"}`) → the rest inherit the previous values automatically
- 全新环境无存档时传 `{}` → `message_id 3`（参数不全） / Pass `{}` with no saved profile on a fresh setup → `message_id 3` (incomplete parameters)
- `GET /profile` 查看存档 + 卡库清单 / Check the saved profile + subscriber DB list

## 2. POST /start（启动基站并抓包） Start eNodeB and Packet Capture

启动顺序：渲染 `epc_run.conf`/`enb_run.conf`/`rr.conf` → 起 `srsepc`（等 3s）→ 加 NAT → 起 `srsenb`（等 3s）→ 起 `tcpdump` 抓 `srs_spgw_sgi`。**全程约 6–10 秒**，curl 记得设 `--max-time 100`。

Startup sequence: render `epc_run.conf`/`enb_run.conf`/`rr.conf` → start `srsepc` (wait 3s) → add NAT → start `srsenb` (wait 3s) → start `tcpdump` on `srs_spgw_sgi`. **About 6–10 seconds total**; remember to set `--max-time 100` for curl.

### 2.1 请求字段 Request Fields

| 字段 Field | 类型 Type | 必填 Required | 默认/说明 Default / Description |
|---|---|---|---|
| `band` | string | ✅ | 频段编号，仅限 `1/3/5/7/8/34/39/40/41`（映射见下表） / Band number, only `1/3/5/7/8/34/39/40/41` (see mapping below) |
| `apn` | string | ✅ | 接入点名，任意（如 `srsapn`，须与终端侧一致），禁 `空格"';&\|<>$`\\` 等注入字符 / Access point name, arbitrary (e.g. `srsapn`, must match the UE side); `空格"';&\|<>$`\\` and other injection characters forbidden |
| `mcc` | string | ✅ | 3 位数字，如 `001` / `460` / 3-digit number, e.g. `001` / `460` |
| `mnc` | string | ✅ | 2 或 3 位数字，如 `01` / `00` / 2- or 3-digit number, e.g. `01` / `00` |
| `network` | string | ✅ | 服务器上行网卡名（`ip route get 8.8.8.8` 看 `dev`，如 `eth0`），禁注入字符 / Server uplink NIC name (check `dev` via `ip route get 8.8.8.8`, e.g. `eth0`); injection characters forbidden |
| `sdr` | string | ❌ | `uhd`/`bladerf`/`zmq`/`auto`（默认 `auto`：有 B210 用 B210，只有 bladeRF 用 bladeRF） / `uhd`/`bladerf`/`zmq`/`auto` (default `auto`: use B210 when present, use bladeRF only when it is the sole device) |
| `device_args` | string | ❌ | 透传 `enb.conf [rf] device_args`。`auto`/缺省 + 检测到 B210 时自动注入 VM-USB 稳定参数；显式值永远优先；bladeRF/ZMQ 不受影响 / Passed through to `enb.conf [rf] device_args`. With `auto`/unset plus a detected B210, VM-USB stability parameters are injected automatically; an explicit value always wins; bladeRF/ZMQ unaffected |
| `tx_gain` | int | ❌ | 默认 `80` / Default `80` |
| `rx_gain` | int | ❌ | 默认 `40`（本机上行弱时实测 `60` 有效） / Default `40` (measured `60` works when the local uplink is weak) |
| `n_prb` | int | ❌ | 默认 `25`（5MHz，虚拟机 USB 安全值；裸金属可 `50`/`100`） / Default `25` (5MHz, safe value for VM USB; bare metal can use `50`/`100`) |
| `full_net_name` | string | ❌ | 终端显示的运营商全称（NITZ 下发），默认 `srsRAN`，1–32 可打印 ASCII，禁 `"';#$`\\` / Full carrier name shown on the UE (delivered via NITZ), default `srsRAN`, 1–32 printable ASCII, `"';#$`\\` forbidden |
| `short_net_name` | string | ❌ | 简称，同上 / Short name, same as above |
| `dns` | string | ❌ | 经 PCO 下发给终端的 DNS，默认 `8.8.8.8`（上行过滤公网 DNS 时填网关，如 `192.0.2.1`），须为合法 IPv4 / DNS delivered to the UE via PCO, default `8.8.8.8` (when uplink filters public DNS, fill the gateway, e.g. `192.0.2.1`); must be a valid IPv4 |

### 2.2 band → 频点映射 Band to Frequency Mapping（[`internal/lte/band.go`](../internal/lte/band.go)，UL EARFCN 显式写入 `rr.conf` / UL EARFCN explicitly written to `rr.conf`)

| band | DL EARFCN | UL EARFCN | 下行 MHz / Downlink MHz | 上行 MHz / Uplink MHz | 双工 / Duplex |
|---|---|---|---|---|---|
| 1 | 300 | 18300 | 2140 | 1950 | FDD |
| 3 | 1575 | 19575 | 1842.5 | 1747.5 | FDD |
| 5 | 2525 | 20525 | 881.5 | 836.5 | FDD |
| 7 | 3350 | 21350 | 2680 | 2560 | FDD |
| 8 | 3625 | 21625 | 942.5 | 897.5 | FDD |
| 34 | 36275 | 36275 | 2017.5 | — | TDD |
| 39 | 38450 | 38450 | 1900 | — | TDD |
| 40 | 39150 | 39150 | 2350 | — | TDD |
| 41 | 40620 | 40620 | 2593 | — | TDD |

> 虚拟机 USB 下建议优先 `7`（FDD 最稳）；TDD（39/40/41）eNB 能起来但上游 UL 推导有坑，已用显式 `ul_earfcn` 绕过，终端先能用 7 就用 7。
> Under VM USB, prefer `7` (FDD is the most stable); TDD (39/40/41) can bring up the eNB but upstream UL derivation is problematic, already worked around with explicit `ul_earfcn`. If the UE works on 7, stay on 7.

### 2.3 响应 Responses

| id | message（原文） | 说明 Description |
|---|---|---|
| 1 | `Start successfully` | 成功（`status:true`）。配置已存档，可 `GET /profile` 核对 / Success (`status:true`). Configuration saved; verify with `GET /profile` |
| 2 | `is running` | 已有 srsepc/srsenb 在跑，先 `/stop` / srsepc/srsenb already running; call `/stop` first |
| 3 | `Incomplete parameters` | 缺 band/apn/mcc/mnc/network 任一；或 JSON 解析失败；或无存档时空启动 / Missing any of band/apn/mcc/mnc/network; or JSON parse failure; or empty start with no saved profile |
| 4 | `device is not connected, please connect usrp device.` | 没检测到 USRP（`sdr:uhd` 强制要求 B210；`auto` 且无任何 SDR 也报这个） / No USRP detected (`sdr:uhd` strictly requires a B210; `auto` with no SDR at all also returns this) |
| 0 | `Start Failed` / `Start Failed: <原因>` | 其他失败（mcc/mnc/apn/network 格式非法、epc 早退、二进制缺失…），看 `docker logs` / Other failures (illegal mcc/mnc/apn/network format, early EPC exit, missing binaries…); check `docker logs` |

### 2.4 示例 Examples

```bash
# 最小：mcc/mnc 与卡的 IMSI 对应即可
curl -X POST http://127.0.0.1:8081/start -H 'Content-Type: application/json' \
  -d '{"band":"7","apn":"srsapn","mcc":"001","mnc":"01","network":"eth0"}'

# 全参数
curl -X POST http://127.0.0.1:8081/start -H 'Content-Type: application/json' -d '{
  "band":"7","apn":"srsapn","mcc":"001","mnc":"01","network":"eth0",
  "sdr":"auto","tx_gain":80,"rx_gain":60,"n_prb":25,
  "full_net_name":"MyLTE","short_net_name":"MyLTE","dns":"192.0.2.1"}'

# 日常：一键复用上次（重建容器/重启后）
curl -X POST http://127.0.0.1:8081/start -H 'Content-Type: application/json' -d '{}'

# 只换频段，其余继承
curl -X POST http://127.0.0.1:8081/start -H 'Content-Type: application/json' -d '{"band":"3"}'
```

注意事项：启动后终端用相匹配的白卡才能附着（IMSI 的 MCC/MNC须与本次一致）；多终端时**只有第一台能上网**（上游 SPGW 限制）；`network` 填错会导致终端能上网但出不了公网（NAT 绑错口）。

Notes: after startup, the UE can only attach with a matching test SIM (the IMSI's MCC/MNC must match this run); with multiple UEs, **only the first one gets Internet access** (upstream SPGW limitation); a wrong `network` lets the UE attach but blocks public Internet access (NAT bound to the wrong interface).

## 3. POST /stop（停止） Stop

无参数（body 可空）。停止顺序：杀 `tcpdump` → `srsenb` → `srsepc` → 删本次加的 NAT 规则 → 清理自加的转发规则。约 2–4 秒。

No parameters (body may be empty). Stop sequence: kill `tcpdump` → `srsenb` → `srsepc` → delete the NAT rules added this time → clean up self-added forwarding rules. About 2–4 seconds.

| id | message（原文） | 说明 Description |
|---|---|---|
| 1 | `Stop successfully.` | 停干净了 / Stopped cleanly |
| 2 | `Not running.` | 本来就没在跑 / Was not running |
| 0 | `Stop failed.` | 还有杀不掉的进程，看 `docker exec ltesystem ps -eo pid,stat,comm`（`Z` 僵尸会被排除判定） / Some processes cannot be killed; check `docker exec ltesystem ps -eo pid,stat,comm` (`Z` zombies are excluded from the running check) |

```bash
curl -X POST http://127.0.0.1:8081/stop -H 'Content-Type: application/json' -d '{}'
```

## 4. POST /basicinfo（读终端基础信息） Read UE Basic Info

无参数。解析 `/data/log/srsLTE_epc.log` 三个关键字（`ESM Info: APN` / `Found User` / `pool ip addr`，取**最后一次**出现=最新附着），**只报第一台终端**。

No parameters. Parses three keywords in `/data/log/srsLTE_epc.log` (`ESM Info: APN` / `Found User` / `pool ip addr`, taking the **last** occurrence = latest attach), and **reports only the first UE**.

成功响应（取不到的字段为 `null`） Success response (missing fields are `null`)：

```json
{"status":true,"message_id":1,"message":"Getting information success.",
  "apn":"srsapn","imsi":"001010123456789","ip":"172.16.0.2"}
```

> `apn` 为 `null` 是正常现象：部分终端（实测华为 CPE）的 PDN 请求不走 ESM Information 流程，日志里就没有该行，不代表异常。`imsi`+`ip` 都有即附着成功。
> A `null` `apn` is normal: PDN requests from some UEs (measured Huawei CPE) skip the ESM Information procedure, so that line never appears in the log; it is not an error. Having both `imsi` and `ip` means attach succeeded.

| id | message（原文） | 说明 Description |
|---|---|---|
| 1 | `Getting information success.` | 成功（见上） / Success (see above) |
| 2 | `Is not running, please start first.` | 基站没在跑 / eNodeB not running |
| 3 | `no UE connected` | 跑着但尚无终端附着 / Running but no UE attached yet |
| 0 | `Failed` | 非 POST 或未知错误 / Non-POST or unknown error |

```bash
curl -X POST http://127.0.0.1:8081/basicinfo -H 'Content-Type: application/json' -d '{}'
```

## 5. POST /crackapn（开始破解 APN 密码，注意会停基站） Start APN Dictionary Attack (Stops eNodeB)

无参数。流程：从 `srsLTE_enb_s1ap.pcap` 用 tshark 提取 CHAP（`response:challenge:id`）→ **停掉正在运行的基站**（释放 CPU、固定 pcap）→ 后台起 `hashcat -m 4800` 字典爆破 → 立即返回。**调用前确保已有一台 CHAP 认证的终端连过**（iPhone 不行，读不到 CHAP）。

No parameters. Flow: extract CHAP (`response:challenge:id`) from `srsLTE_enb_s1ap.pcap` with tshark → **stop the running eNodeB** (free CPU, freeze the pcap) → launch a background `hashcat -m 4800` wordlist dictionary attack → return immediately. **Before calling, make sure one CHAP-authenticated UE has already connected** (iPhone does not work; no CHAP can be read).

| id | message（原文） | 说明 Description |
|---|---|---|
| 1 | `Start crack success.` | 爆破已在后台跑，用 `/getcrackresult` 查 / Dictionary attack now running in the background; poll `/getcrackresult` |
| 2 | `Hashcat is running.` | 已有一个爆破在跑，等它结束 / A dictionary attack is already running; wait for it to finish |
| 3 | `Can not get UE's data, please start first and connect UE, or just connect UE. Then try again.` | 日志里无终端数据（没启动过或没终端连过） / No UE data in the logs (never started or no UE ever connected) |
| 4 | `Can not get username and password` | pcap 里提不出 CHAP（终端不支持或 pcap 为空） / No CHAP extractable from the pcap (UE unsupported or empty pcap) |
| 5 | `Stop program failed, please try to stop manually.` | 停基站失败，手动调 `/stop` 再试 / Failed to stop the eNodeB; call `/stop` manually and retry |
| 0 | `Failed` | 非 POST / hashcat 起不来 / Non-POST / hashcat failed to start |

```bash
curl -X POST http://127.0.0.1:8081/crackapn -H 'Content-Type: application/json' -d '{}'
```

## 6. POST /getcrackresult（取破解结果） Get Cracking Result

无参数。hashcat 还在跑就直接返回等候；否则用当前字典跑 `--show` 取口令。**本接口不启停基站**，读的是历史日志+pcap。

No parameters. Returns a wait response while hashcat is still running; otherwise runs `--show` with the current wordlist to retrieve the password. **This endpoint never starts or stops the eNodeB**; it reads historical logs + pcap.

成功响应 Success response：

```json
{"status":true,"message_id":1,"message":"Getting information success.",
 "apn":"srsapn","imsi":"001010123456789","ip":"172.16.0.2",
 "username":"mi6test","password":"cmwap"}
```

| id | message（原文） | 说明 Description |
|---|---|---|
| 1 | `Getting information success.` | 成功（见上） / Success (see above) |
| 2 | `Cracking apn is still running, please try again later.` | 还在爆破，稍后再查 / Dictionary attack still running; check again later |
| 3 | `Can not get password from dict.` | 字典里没有这个口令，换更大的字典重传后重跑 / Password not in the wordlist; upload a larger wordlist and rerun |
| 4 | `Can not get username and password.` | 同 crackapn 的 4 / Same as crackapn 4 |
| 5 | `Can not get UE's data, please start first and connect UE, or just connect UE, then try again.` | 同 crackapn 的 3 / Same as crackapn 3 |
| 0 | `Failed.` | 非 POST / `--show` 执行失败 / Non-POST / `--show` execution failed |

```bash
curl -X POST http://127.0.0.1:8081/getcrackresult -H 'Content-Type: application/json' -d '{}'
```

## 7. POST /userupload · /passwordupload（上传卡库/字典） Upload Subscriber DB and Wordlist

`multipart/form-data`，字段名固定（错了按空文件处理）：

`multipart/form-data` with fixed field names (a wrong name is treated as an empty file):

| 接口 Endpoint | 字段名 Field | 落盘位置 Destination | 上限 Limit |
|---|---|---|---|
| `/userupload` | `userdb` | `/data/conf/user_db.csv`（原子替换） / atomic replace | 8MB |
| `/passwordupload` | `wordlist` | `/data/wordlist.list`（原子替换） / atomic replace | 8MB |

| id | message（原文） | 说明 Description |
|---|---|---|
| 1 | `upload success` | 成功（原子替换，旧文件先写 `.tmp` 再改名） / Success (atomic replace: old file first written to `.tmp`, then renamed) |
| 2 | `no file` | 字段名错 / 没带文件 / 空文件 / Wrong field name / no file attached / empty file |
| 0 | `upload failed` / `upload failed: file too large` | 解析失败 / 超 8MB / 写盘失败 / Parse failure / over 8MB / disk write failure |

```bash
curl -X POST http://127.0.0.1:8081/userupload -F userdb=@user_db.csv
curl -X POST http://127.0.0.1:8081/passwordupload -F wordlist=@wordlist.list
```

- `user_db.csv` 格式：`Name,Auth,IMSI,Key,OP_Type,OP/OPc,AMF,SQN,QCI,IP_alloc`，`Name` 唯一，末尾留空行（见 [`configs/README.md`](../configs/README.md)）。**EPC 只在启动时读库**：上传后必须 `/stop` 再 `/start` 才生效。
- The `user_db.csv` format is `Name,Auth,IMSI,Key,OP_Type,OP/OPc,AMF,SQN,QCI,IP_alloc` with a unique `Name` and a trailing empty line (see [`configs/README.md`](../configs/README.md)). **The EPC only reads the DB at startup**: after uploading you must `/stop` then `/start` for it to take effect.
- 字典格式：一行一个候选口令（见 [`configs/wordlist.list.example`](../configs/wordlist.list.example)）。下次 `/crackapn` 即用新字典。
- Wordlist format: one candidate password per line (see [`configs/wordlist.list.example`](../configs/wordlist.list.example)). The next `/crackapn` uses the new wordlist immediately.

## 8. POST /getfile（下载抓包文件） Download Packet Capture Files

请求：`{"fileid": N}`（`fileid` 缺失或体裁错误 → `Error id`）。

Request: `{"fileid": N}` (missing or mistyped `fileid` → `Error id`).

| fileid | 文件 File | 内容 Contents |
|---|---|---|
| 0 | `lte_data.pcap` | SGi 口业务流量（tcpdump 抓的，上网行为分析用这个） / SGi user-plane traffic (captured by tcpdump; use this for Internet behavior analysis) |
| 1 | `srsLTE_enb_s1ap.pcap` | eNB S1AP（Wireshark DLT=150 + s1ap 解析） / eNB S1AP (Wireshark DLT=150 + s1ap dissector) |
| 2 | `srsLTE_enb.pcap` | eNB 空口 MAC / eNB air-interface MAC |
| 3 | `srsLTE_epc.pcap` | EPC / EPC |

- 成功：直接返回文件下载（`Content-Disposition: attachment`），curl 加 `-OJ` 保存。 / On success: returns the file as a download (`Content-Disposition: attachment`); add `-OJ` to curl to save it.
- 失败 JSON：`{"status":false,"message_id":2,"message":"Error id"}`（id 非法/缺失）；`{"status":false,"message_id":3,"message":"Cant not find the file"}`（文件尚不存在，如还没跑过基站）。 / Failure JSON: `{"status":false,"message_id":2,"message":"Error id"}` (illegal/missing id); `{"status":false,"message_id":3,"message":"Cant not find the file"}` (file does not exist yet, e.g. the eNodeB has never run).
- 非 POST → `message_id 0` + `Failed`。 / Non-POST → `message_id 0` + `Failed`.

```bash
curl -X POST http://127.0.0.1:8081/getfile -H 'Content-Type: application/json' \
  -d '{"fileid":0}' -OJ
```

## 9. POST /writesim（写卡，有写卡器时才用） SIM Programming (Card Reader Required)

> **当前无写卡器：本接口固定返回 `message_id 2`，直接跳过**，走 [`docs/QUICKSTART.md`](QUICKSTART.md) 用已写好的卡。有写卡器（ACR1281U 接好）后按下文使用。
> **Currently there is no card reader: this endpoint always returns `message_id 2`; skip it** and use the pre-programmed cards via [`docs/QUICKSTART.md`](QUICKSTART.md). With a card reader (ACR1281U connected), use it as described below.

超时 180s（写卡+回读慢）。未知 JSON 字段会被**直接拒绝**（`Invalid parameters`），拼写注意。

Timeout is 180s (SIM programming + read-back is slow). Unknown JSON fields are **rejected outright** (`Invalid parameters`); watch your spelling.

### 9.1 请求字段（除 `imsi` 全可选，缺省取服务端默认） Request Fields (All Optional Except `imsi`; Unset Fields Use Server Defaults)

| 字段 Field | 说明 Description | 默认 Default |
|---|---|---|
| `imsi` | ✅必填，15 位数字字符串 / ✅required, 15-digit numeric string | — |
| `mcc`/`mnc` | 缺省从 IMSI 切片（前 3 / 接着 2 位） / By default sliced from the IMSI (first 3 / next 2 digits) | IMSI 派生 / Derived from IMSI |
| `mnc3` | `true` 时 MNC 取 3 位 / Take a 3-digit MNC when `true` | `false` |
| `ki` | 32 hex | `00112233445566778899aabbccddeeff` |
| `opc` / `op` | 32 hex，**二选一互斥** / 32 hex, **mutually exclusive, pick one** | `opc=63bfa50ee6523365ff14c1f45f88737d` |
| `op_type` | `op` 或 `opc` / `op` or `opc` | 跟随所选 / Follows the selected one |
| `auth` | `mil` 或 `xor` / `mil` or `xor` | `mil` |
| `amf` | 4 hex | `8001`（与示例卡一致） / `8001` (matches the example card) |
| `acc` | 4 hex 接入等级 / 4-hex access class | `FFFF` |
| `adm` | ADM hex | `3030303030303030` |
| `spn` | 运营商显示名 / Carrier display name | `LTESystem` |
| `iccid` | 指定值，或 `"auto"`=保留出厂值（SJA2 类卡必须用 `auto`） / Explicit value, or `"auto"` = keep the factory value (SJA2 cards must use `auto`) | `89860123456789012345` |
| `sqn` | 12 hex 序列号 / 12-hex sequence number | 随机 / Random |
| `qci` | int | `7` |
| `card` | pysim 卡型（`testsim`/`sysmoUSIM-SJS1`/…，见 [`configs/sim_profiles.yaml`](../configs/sim_profiles.yaml)） / pysim card type (`testsim`/`sysmoUSIM-SJS1`/…, see [`configs/sim_profiles.yaml`](../configs/sim_profiles.yaml)) | `testsim` |
| `name` | 卡库 `Name` 列 / `Name` column in the subscriber DB | 自动 `ueN` / Auto `ueN` |
| `pin_adm` | 极少用，ADM 覆盖 / Rarely used, ADM override | — |

### 9.2 响应 Responses

| id | message（原文） | 说明 Description |
|---|---|---|
| 1 | `Succeed.` | 写卡+回读校验+入库全成 / SIM programming + read-back verification + DB insert all succeeded |
| 2 | `Device is not connected, please connect acr1281 first.` | 读卡器没连（**无写卡器时恒定返回这个**） / Card reader not connected (**always returned when there is no card reader**) |
| 3 | `Writting card successfully, but write user_db.csv failed.` | 卡写好了但入库失败，手动补一行 / Card programmed but DB insert failed; add the row manually |
| 4 | `The card already exists and can be used directly.` | 该 IMSI 已在库，直接用 / This IMSI is already in the DB; use it directly |
| 5 | `SIM card is not inserted.` | 读卡器在但没插卡 / Card reader present but no card inserted |
| 6 | `Invalid parameters` / `Invalid parameters: <原因>` | JSON 非法、未知字段、`imsi` 缺失、ki/opc/amf 等格式错 / Illegal JSON, unknown fields, missing `imsi`, or bad ki/opc/amf format |
| 0 | `Failed.` | 非 POST / 写卡失败 / 回读校验失败 / Non-POST / SIM programming failed / read-back verification failed |

```bash
# 最小
curl -X POST http://127.0.0.1:8081/writesim -H 'Content-Type: application/json' -d '{"imsi":"001010123456781"}'
# 全参数（460 网络示例）
curl -X POST http://127.0.0.1:8081/writesim -H 'Content-Type: application/json' -d '{
  "imsi":"460001234567890","mcc":"460","mnc":"00",
  "ki":"00112233445566778899aabbccddeeff","opc":"63bfa50ee6523365ff14c1f45f88737d",
  "auth":"mil","amf":"8001","spn":"CMCC","card":"testsim"}'
```

流程细节：`service pcscd restart` → 探卡 → `pySim-prog.py` 写卡（判 `Programming successful`）→ `pySim-read.py` 回读核对 IMSI → 追加 `user_db.csv`。注意 `testsim` 类卡 Ki/OPc 出厂预置不可改，写卡参数必须与出厂值一致（见 [`docs/SIM.md`](SIM.md) §0）。

Process details: `service pcscd restart` → detect the card → program it with `pySim-prog.py` (look for `Programming successful`) → read it back with `pySim-read.py` to verify the IMSI → append to `user_db.csv`. Note that `testsim` cards have factory-preset Ki/OPc that cannot be changed; programming parameters must match the factory values (see [`docs/SIM.md`](SIM.md) §0).

## 10. 状态接口 /healthz、/status、/profile Status Endpoints

### GET /healthz（任意方法均可） Any Method Allowed

```json
{"ok":true,"running":true,
 "sdr":{"uhd_b210":true,"bladerf":false,"acr1281":false,
        "uhd_raw":"...","bladerf_raw":"...","usb_acr_raw":"..."},
 "time":"2026-09-05T08:13:53Z"}
```

`sdr` 为实时探测：B210（含序列号/型号原文）、bladeRF、ACR1281U。容器刚起、USB 刚插拔后查这个。

`sdr` is probed live: B210 (with raw serial/model strings), bladeRF, ACR1281U. Check this right after the container starts or USB devices are plugged/unplugged.

### GET /status（任意方法均可） Any Method Allowed

```json
{"running":true,"epc":true,"enb":true,"pcap":true,
 "started_at":"2026-09-05T10:06:48Z","band":"7","apn":"srsapn","net_name":"MyLTE"}
```

空闲时只有 `{"running":false,"epc":false,"enb":false,"pcap":false}`（无 started_at 等字段）。`started_at` 为本次 `/start` 的 UTC 时间。

When idle, only `{"running":false,"epc":false,"enb":false,"pcap":false}` is returned (no started_at or similar fields). `started_at` is the UTC time of the current `/start`.

### GET /profile（必须 GET，其他方法 → `message_id 0` + `Failed`） Must Be GET (Other Methods → `message_id 0` + `Failed`)

```json
{"has_profile":true,
 "profile":{"band":"7","apn":"srsapn","mcc":"001","mnc":"01","network":"eth0",
            "sdr":"","device_args":"","tx_gain":80,"rx_gain":40,"n_prb":25,
            "full_net_name":"MyLTE","short_net_name":"MyLTE","dns":"192.0.2.1"},
 "ues":[{"name":"ue0","auth":"mil","imsi":"001010123456789"}]}
```

- `profile` 是**解析后的生效值**（含继承的默认增益/带宽），与 `last_start.json` 一致；无存档时无此字段 / `profile` holds the **parsed effective values** (including inherited default gains/bandwidth), consistent with `last_start.json`; absent when there is no saved profile
- `ues` 为卡库清单（**不含密钥**，密钥只在服务器文件里）；空库为 `[]` / `ues` is the subscriber DB list (**without secrets**; secrets live only in server-side files); empty DB is `[]`

```bash
curl http://127.0.0.1:8081/healthz
curl http://127.0.0.1:8081/status
curl http://127.0.0.1:8081/profile
```

## 11. 典型流程 Typical Workflows

```bash
BASE=http://127.0.0.1:8081
curl -s $BASE/healthz                          # 1. 硬件在不在（uhd_b210 应 true）
curl -s -X POST $BASE/start -d '{}'            # 2. 一键复用上次配置启动（首次用全参数）
curl -s -X POST $BASE/basicinfo -d '{}'        # 3. 终端附着后查 IMSI/IP
curl -s -X POST $BASE/getfile -d '{"fileid":0}' -OJ   # 4. 下业务包分析
curl -s -X POST $BASE/stop -d '{}'             # 5. 收工
# 换卡库：先传后重启
curl -s -X POST $BASE/userupload -F userdb=@user_db.csv
curl -s -X POST $BASE/stop -d '{}' && curl -s -X POST $BASE/start -d '{}'
# APN 口令爆破（会停基站！做完重起）
curl -s -X POST $BASE/crackapn -d '{}'
curl -s -X POST $BASE/getcrackresult -d '{}'
```

冒烟脚本：`BASE=http://127.0.0.1:8081 bash scripts/smoke.sh`（覆盖 healthz/status/stop/basicinfo/getfile/上传干跑；start/crack/writesim 需硬件不测）。

Smoke script: `BASE=http://127.0.0.1:8081 bash scripts/smoke.sh` (covers healthz/status/stop/basicinfo/getfile/dry-run uploads; start/crack/writesim need hardware and are skipped).

## 12. message_id 总表 Summary

| 接口 Endpoint | 0 | 1 | 2 | 3 | 4 | 5 | 6 |
|---|---|---|---|---|---|---|---|
| `/start` | 启动失败（原因见 message） / Start failed (see message for the reason) | 启动成功 / Started | 运行中 / Already running | 参数不全/JSON错/无存档 / Incomplete parameters/bad JSON/no saved profile | 无 USRP / No USRP | — | — |
| `/stop` | 停止失败 / Stop failed | 停止成功 / Stopped | 本来就没跑 / Not running | — | — | — | — |
| `/basicinfo` | 失败 / Failed | 获取成功 / Info retrieved | 基站没跑 / eNodeB not running | 无终端 / No UE | — | — | — |
| `/crackapn` | 失败 / Failed | 爆破已启动 / Dictionary attack started | hashcat忙 / hashcat busy | 无终端数据 / No UE data | 无CHAP / No CHAP | 停基站失败 / Failed to stop eNodeB | — |
| `/getcrackresult` | 失败 / Failed | 获取成功 / Info retrieved | 爆破中 / Dictionary attack running | 字典无此口令 / Password not in wordlist | 无CHAP / No CHAP | 无终端数据 / No UE data | — |
| `/userupload`·`/passwordupload` | 上传失败/超8MB / Upload failed/over 8MB | 上传成功 / Uploaded | 无文件 / No file | — | — | — | — |
| `/getfile` | 方法错 / Wrong method | —（成功直接下载） / — (success returns the download directly) | 非法id / Illegal id | 文件不存在 / File not found | — | — | — |
| `/writesim` | 写卡/校验失败 / SIM programming/verification failed | 写卡成功 / Card programmed | 无读卡器 / No card reader | 入库失败 / DB insert failed | 卡已在库 / Card already in DB | 没插卡 / No card inserted | 参数非法 / Invalid parameters |

> `/getcrackresult` 没有 6（旧文档误列，已按实现修正）；`/start` 没有 5；`getfile` 成功时不返回 JSON。
> `getcrackresult` has no 6 (wrongly listed in the old docs, fixed per the implementation); `/start` has no 5; `getfile` returns no JSON on success.

---
**导航 Navigation:** [文档索引 Docs](README.md) · [QUICKSTART](QUICKSTART.md) · [RULES](RULES.md) · [API v1](API.md) · [旧版API Legacy](API_LEGACY.md) · [DEPLOY](DEPLOY.md) · [SIM](SIM.md) · [SDR](SDR.md) · [MIGRATION](MIGRATION.md)
