# LTE-System API v1 标准接口 Standard API

> 旧版根路径接口（`/start`、`/stop`…）仍可用但已冻结，见 [`API_LEGACY.md`](API_LEGACY.md)。
> Legacy root-path APIs (`/start`, `/stop`, …) still work but are frozen; see [`API_LEGACY.md`](API_LEGACY.md).
> 机器可读契约：[`api/openapi.yaml`](api/openapi.yaml)（OpenAPI 3.0）。
> Machine-readable contract: [`api/openapi.yaml`](api/openapi.yaml) (OpenAPI 3.0).

- 基地址 Base URL：Docker 默认监听 `0.0.0.0:8081`；客户端访问 `http://HOST:8081`（HOST 为服务器 IP），不是 `0.0.0.0`。建议设置强随机 `LTE_API_TOKEN` / Docker listens on `0.0.0.0:8081` by default; clients use `http://HOST:8081` with the server IP, not `0.0.0.0`. A strong random `LTE_API_TOKEN` is recommended
- 统一包络 Envelope：`{"code": int, "message": str, "data": obj|null, "request_id": str}`，`code 0` = 成功 / Unified envelope: same schema, `code 0` means success
- 错误码 Error code：`code = HTTP状态码*100+序号`（如 `40401` → HTTP 404），对照表见 §8 错误码 / Formula: `code = HTTP status * 100 + index` (e.g. `40401` means HTTP 404); see §8 Error Codes for the table
- 每个响应 Response 带 `X-Request-ID` 头；服务端按 `rid=<id> <METHOD> <PATH> -> <状态> (<耗时>)` 记审计日志，401 与已恢复 panic 也记录且使用同一 request ID / Every response carries an `X-Request-ID` header; the server audits `rid=<id> <METHOD> <PATH> -> <status> (<duration>)`, including 401 responses and recovered panics under the same request ID
- 可选鉴权 Optional auth：服务端设 `LTE_API_TOKEN` 后，所有接口（新旧）都要带 `Authorization: Bearer <token>`，否则 401。默认未设 = 局域网开放模式（启动日志有 WARNING） / After the server sets `LTE_API_TOKEN`, all APIs (old and new) require `Authorization: Bearer <token>`, otherwise 401. Unset by default = open LAN mode (a WARNING appears in the boot log)
- 404/405 也是 JSON 包络 Envelope（旧版根路径 404 保持纯文本，不变） / 404/405 also use the JSON envelope (legacy root-path 404 stays plain text, unchanged)
- JSON 请求体必须是且只能是一个对象；`null`、数组、第二个 JSON 值或尾随垃圾均返回 400 且不产生副作用。`cell` 上限 1 MiB，`simcards` 上限 64 KiB，超限返回 413。各端点原有未知字段策略不变 / A JSON body must contain exactly one object; `null`, arrays, a second JSON value, and trailing garbage return 400 without side effects. Limits are 1 MiB for `cell` and 64 KiB for `simcards`; excess returns 413. Each endpoint keeps its existing unknown-field policy
- Postman 开箱即用：导入 [`postman/lte-system.postman_collection.json`](../postman/lte-system.postman_collection.json)（26 个请求 + 断言，用法见 [`postman/README.md`](../postman/README.md)）/ Ready-to-import Postman collection (26 requests with assertions, see [`postman/README.md`](../postman/README.md))

目录 Contents：[§1 小区 Cell](#1-小区-cell) · [§2 终端 UE](#2-终端-ue) · [§3 爆破 Cracking](#3-爆破-cracking) · [§4 配置上传 Config Upload](#4-配置上传-config-upload) · [§5 抓包下载 Captures](#5-抓包下载-captures-packet-capture) · [§6 写卡 Simcards](#6-写卡-simcards) · [§7 存档与健康 Saved Profile and Health](#7-存档与健康-saved-profile-and-health) · [§8 错误码 Error Codes](#8-错误码-error-codes) · [§9 旧版新版对照表 Legacy to v1 Mapping](#9-旧版新版对照表-legacy-to-v1-mapping)

---

## 1. 小区 Cell

### POST /api/v1/cell — 启动 Start

启动顺序与耗时同旧版（渲染配置 → srsepc → NAT → srsenb → tcpdump，约 6–10 秒，curl 设 `--max-time 100`）。

Startup sequence and timing are the same as legacy (render config → srsepc → NAT → srsenb → tcpdump, about 6–10 seconds; set `--max-time 100` for curl).

字段与旧 `/start` 一致（band/apn/mcc/mnc/network/sdr/device_args/tx_gain/rx_gain/n_prb/full_net_name/short_net_name/dns，见 openapi.yaml），另有三处**标准化差异**：

Fields are the same as legacy `/start` (band/apn/mcc/mnc/network/sdr/device_args/tx_gain/rx_gain/n_prb/full_net_name/short_net_name/dns; see openapi.yaml), with three **standardization differences**:

1. **空 `{}` 复用存档 Reuse saved profile with empty `{}`**（`/data/last_start.json`，行为同旧版，见 `RULES.md`） / Reuses the saved profile at `/data/last_start.json` when the request body is empty `{}`, same as legacy; see `RULES.md`
2. **严格校验 Strict validation**：缺字段/非法值/未知 band/非法 `n_prb`（仅允许 6/15/25/50/75/100）/`tx_rx` 越界（0–90）一律 **422**，`data.errors` 逐字段说明；**未知 band 不再静默回退** / Missing fields, illegal values, unknown band, illegal `n_prb` (only 6/15/25/50/75/100 allowed) or out-of-range `tx_rx` (0–90) always return **422** with per-field details in `data.errors`; **an unknown band never silently falls back**
3. TDD band 成功时 `data.warning` 提示上行 Uplink 注意事项 / On success with a TDD band, `data.warning` notes uplink precautions

```bash
# 最小
curl -X POST http://127.0.0.1:8081/api/v1/cell -H 'Content-Type: application/json' \
  -d '{"band":"7","apn":"srsapn","mcc":"001","mnc":"01","network":"eth0"}'
# 200 {"code":0,"message":"cell started","data":{"band":"7","apn":"srsapn","net_name":"srsRAN"},...}

# 422 示例
# 422 {"code":42201,"message":"validation failed",
#      "data":{"errors":[{"field":"band","reason":"must be one of 1,3,5,7,8,34,39,40,41"}]},...}
```

| 情况 | HTTP | code | message |
|---|---|---|---|
| 成功 | 200 | 0 | `cell started` |
| body 非单一 JSON 对象 | 400 | 40001 | `malformed JSON body` |
| JSON body 超过 1 MiB | 413 | 41301 | `JSON body exceeds limit` |
| 校验失败 | 422 | 42201 | `validation failed` |
| 已在运行 | 409 | 40901 | `cell already running` |
| 无 SDR | 503 | 50301 | `no SDR device attached` |
| 网卡名不存在 | 422 | 42201 | `unknown network interface "xxx"` |
| 其他失败 | 500 | 50001 | `start failed: <原因>` |

### GET /api/v1/cell — 状态 Status

```json
{"code":0,"message":"ok","data":
 {"running":true,"epc":true,"enb":true,"pcap":true,
  "started_at":"2026-09-05T10:06:48Z","band":"7","apn":"srsapn","net_name":"MyLTE"}}
```

空闲时 `data` 为 `{"running":false,"epc":false,"enb":false,"pcap":false}`。

### DELETE /api/v1/cell — 停止 Stop（幂等 Idempotent）

运行中或仍有本服务管理的孤立抓包进程 → 清理后 `200 {"code":0,"message":"cell stopped","data":{"stopped":true}}`；
本来就没跑 → 同样 200，`stopped:false`（旧版此处返回“失败”，v1 改为幂等成功）。

Running, or an orphaned capture still owned by this service, is cleaned up and returns `stopped:true`; an already idle manager returns 200 with `stopped:false`.

```bash
curl -X DELETE http://127.0.0.1:8081/api/v1/cell
```

## 2. 终端 UE

### GET /api/v1/ue — 附着终端快照（第一台） Attached UE Snapshot (First UE Only)

```json
{"code":0,"message":"ok","data":{"apn":"srsapn","imsi":"001010123456789","ip":"172.16.0.2"}}
```

取不到的字段为 `null`（部分终端不走 ESM Information 流程时 `apn` 为 null，属正常）。

Fields that cannot be obtained are `null` (it is normal for `apn` to be null when some UEs skip the ESM Information procedure).

| 情况 Case | HTTP | code |
|---|---|---|
| 成功 / Success | 200 | 0 |
| 基站没跑 / Cell not running | 412 | 41201 `cell not running` |
| 无终端 / No UE attached | 404 | 40401 `no UE attached` |

## 3. 爆破 Cracking

### POST /api/v1/crack/jobs — 开始爆破（⚠️ 会停基站） Start Cracking (⚠️ Stops the Cell)

从 S1AP 包提 CHAP → 停基站 → 后台 `hashcat -m 4800`。成功返回 **202**：

Extract CHAP from S1AP packets → stop the cell → run `hashcat -m 4800` in the background. Returns **202** on success:

```json
{"code":0,"message":"crack job running",
 "data":{"state":"running","poll":"/api/v1/crack/result"}}
```

| 情况 Case | HTTP | code |
|---|---|---|
| 已有任务在跑 / Job already running | 409 | 40901 |
| 无终端数据 / No UE data | 404 | 40401 |
| pcap 无 CHAP（终端不用 CHAP，如 iPhone） / No CHAP in packet capture (UE does not use CHAP, e.g. iPhone) | 412 | 41201 |
| 停基站失败 / Failed to stop the cell | 500 | 50001 |

### GET /api/v1/crack/result — 取结果 Get Result

| data.state | 含义 Meaning |
|---|---|
| `running`（200） | 还在跑，稍后轮询 / Still running, poll later |
| `ready`（200） | 成功，带 `apn/imsi/ip/username/password` / Success, includes `apn/imsi/ip/username/password` |
| 无（404） / None (404) | 字典无此口令（`password not in dictionary`，带 `username`）/ 无终端数据 / Dictionary has no such password (with `username`) / No UE data |

## 4. 配置上传 Config Upload

`POST /api/v1/config/subscribers`（字段 `userdb` → `/data/conf/user_db.csv`）与
`POST /api/v1/config/wordlist`（字段 `wordlist` → `/data/wordlist.list`），`multipart/form-data`，上限 8MB。

`POST /api/v1/config/subscribers` (field `userdb` → `/data/conf/user_db.csv`) and
`POST /api/v1/config/wordlist` (field `wordlist` → `/data/wordlist.list`) use `multipart/form-data` with an 8MB limit.

成功 200，`data.note` 提示生效时机（卡库需重启 EPC，字典下次爆破即用）：

Success returns 200; `data.note` notes when it takes effect (subscriber DB needs an EPC restart, dictionary applies to the next cracking job):

```json
{"code":0,"message":"upload success",
 "data":{"bytes":2645,"note":"takes effect after restart (EPC reads at boot)"}}
```

缺文件 422，超限 413（`upload exceeds limit`，带 `max_bytes`），失败 500。

Missing file returns 422, over-limit returns 413 (`upload exceeds limit` with `max_bytes`), failure returns 500.

替换通过同目录唯一临时文件原子提交；并发上传不会互相截断，任何解析、大小或写入失败都会保留旧文件。订户库替换与写卡事务串行，避免丢失并发新增的用户。

Replacement is atomically committed from a unique same-directory temporary file. Concurrent uploads cannot truncate each other, and any parse, size, or write failure preserves the previous file. Subscriber-database replacement is serialized with SIM-programming transactions so a concurrently added subscriber is not lost.

```bash
curl -X POST http://127.0.0.1:8081/api/v1/config/subscribers -F userdb=@user_db.csv
```

## 5. 抓包下载 Captures Packet Capture

`GET /api/v1/captures/{id}`，`id` 取 `lte-data`（业务流量）/`s1ap`/`enb`/`epc`（兼容旧数字 `0-3`）。
成功直接下载；未知 id 或文件未就绪 → 404。

`GET /api/v1/captures/{id}`, where `id` is `lte-data` (user traffic) / `s1ap` / `enb` / `epc` (legacy numeric `0-3` still accepted).
On success the file downloads directly; unknown id or file not ready → 404.

```bash
curl -OJ http://127.0.0.1:8081/api/v1/captures/lte-data
```

> 想下 EPC/eNB 运行日志？暂不支持（在规划中），目前用 `docker exec ltesystem tail /data/log/srsLTE_epc.log`。
> Want EPC/eNB runtime logs? Not supported yet (planned); for now use `docker exec ltesystem tail /data/log/srsLTE_epc.log`.

## 6. 写卡 Simcards

`POST /api/v1/simcards`，body 同旧 `/writesim` 全字段（`imsi` 必填，其余可选，见 openapi.yaml；
**未知字段忽略**——旧版会直接拒绝，这是故意的向前兼容差异）。最长 180s。

`POST /api/v1/simcards` takes the same full field set as legacy `/writesim` (`imsi` required, others optional; see openapi.yaml;
**unknown fields are ignored** — legacy rejected them outright, an intentional forward-compatibility difference). Takes up to 180s.

写卡从冲突预检、硬件操作到用户库提交为单一串行事务。已有 IMSI 的认证参数若与请求冲突会在任何硬件操作前以 422 拒绝；请先解决数据库冲突。`name` 禁止逗号、双引号、控制字符和注释前缀，`op_type` 必须与实际提供的 OP/OPc 一致。

Programming is one serialized transaction from conflict preflight through hardware work and subscriber-database commit. Authentication parameters conflicting with an existing IMSI are rejected with 422 before any hardware action; resolve the database conflict first. `name` rejects commas, quotes, control characters, and comment prefixes, while `op_type` must match the supplied OP/OPc material.

| 情况 Case | HTTP | code |
|---|---|---|
| 写卡成功 / Programmed | 200 | 0，`data.result=programmed` |
| 卡已在库 / Card already in database | 409 | 40901，`data.result=exists` |
| 无读卡器 / No reader attached | 503 | 50301 |
| 没插卡 / No card inserted | 412 | 41201 |
| 参数非法 / Invalid parameters | 422 | 42201 |
| JSON body 超过 64 KiB / JSON body over 64 KiB | 413 | 41301 |
| 入库失败/写卡失败 / DB insert failed / Programming failed | 500 | 50001 |

无读卡器环境调本接口恒定 503，属正常（跳过即可，不影响入网）。

Calling this API without a reader always returns 503, which is normal (just skip it; network attach is unaffected).

## 7. 存档与健康 Saved Profile and Health

- `GET /api/v1/profile` → `200 {"code":0,…,"data":{"has_profile":true,"profile":{…},"ues":[{"name","auth","imsi"}]}}`（无密钥；无存档时无 `profile` 字段）。旧 `GET /profile` 已统一为同一包络。 / `GET /api/v1/profile` → `200 {"code":0,…,"data":{"has_profile":true,"profile":{…},"ues":[{"name","auth","imsi"}]}}` (no key material; no `profile` field without a saved profile). Legacy `GET /profile` now uses the same envelope.
- `GET /api/v1/health` → `200 {"code":0,…,"data":{"ok":true,"running":bool,"sdr":{…},"time":"…"}}`（旧 `GET /healthz` 保留原样）。 / `GET /api/v1/health` → `200 {"code":0,…,"data":{"ok":true,"running":bool,"sdr":{…},"time":"…"}}` (legacy `GET /healthz` stays as-is).

## 8. 错误码 Error Codes

| code | HTTP | 含义 Meaning |
|---|---|---|
| 0 | 200 | 成功（DELETE 停止空闲、GET 轮询 `running` 等也属成功） / Success (idle DELETE stop, polling `running` via GET, etc. also count as success) |
| 40001 | 400 | body 非单一 JSON 对象 / multipart 坏 / Body is not exactly one JSON object / bad multipart |
| 40101 | 401 | 缺少或错误的 Bearer token（仅设 `LTE_API_TOKEN` 时） / Missing or invalid Bearer token (only when `LTE_API_TOKEN` is set) |
| 40401 | 404 | 资源不存在（未知路径/capture id/无终端/字典无口令/无 UE 数据） / Resource not found (unknown path / capture id / no UE / password not in dictionary / no UE data) |
| 40501 | 405 | 方法不允许 / Method not allowed |
| 40901 | 409 | 冲突（小区在跑/爆破在跑/卡已在库） / Conflict (cell running / cracking job running / card already in database) |
| 41201 | 412 | 前置条件不满足（小区没跑/无 CHAP/没插卡） / Precondition failed (cell not running / no CHAP / no card inserted) |
| 41301 | 413 | JSON body 或上传超限（带 `max_bytes`） / JSON body or upload exceeds limit (with `max_bytes`) |
| 42201 | 422 | 校验失败（`data.errors:[{field,reason}]`） / Validation failed (`data.errors:[{field,reason}]`) |
| 50001 | 500 | 内部失败（message 带原因） / Internal failure (message carries the reason) |
| 50301 | 503 | 硬件缺失（无 SDR/无读卡器） / Hardware missing (no SDR / no reader attached) |

## 9. 旧版新版对照表 Legacy to v1 Mapping

| 旧版 Legacy（根路径，已冻结 Frozen） | 新版 New（`/api/v1`） | 行为差异 Behavior Change |
|---|---|---|
| `POST /start` | `POST /api/v1/cell` | 未知 band 由静默回退改为 422；`n_prb`/增益/网卡加入校验；TDD 成功带 warning / Unknown band changed from silent fallback to 422; added validation for `n_prb`/gains/network iface; TDD success carries warning |
| `POST /stop` | `DELETE /api/v1/cell` | 空闲停止由“失败”改为幂等成功 / Stopping an idle cell changed from "failure" to idempotent success |
| `POST /basicinfo` | `GET /api/v1/ue` | 无终端由包络失败改为 404 / No UE changed from envelope failure to 404 |
| `POST /crackapn` | `POST /api/v1/crack/jobs` | 成功改 202；无 CHAP 由失败改为 412 / Success changed to 202; no CHAP changed from failure to 412 |
| `POST /getcrackresult` | `GET /api/v1/crack/result` | `running`/`ready` 状态机替代真假值 / `running`/`ready` state machine replaces booleans |
| `POST /userupload` | `POST /api/v1/config/subscribers` | 成功带生效时机 note；超限改 413 / Success carries effective-timing note; over-limit changed to 413 |
| `POST /passwordupload` | `POST /api/v1/config/wordlist` | 同上 / Same as above |
| `POST /getfile {fileid}` | `GET /api/v1/captures/{id}` | 字符串 id（数字别名兼容） / String id (numeric aliases compatible) |
| `POST /writesim` | `POST /api/v1/simcards` | 未知字段由拒绝改忽略；卡已在库改 409 / Unknown fields changed from rejection to ignored; card already in database changed to 409 |
| `GET /healthz`·`/status`·`/profile` | `GET /api/v1/health`·`/api/v1/cell`·`/api/v1/profile` | 统一包络（旧 `/profile` 已同步新包络） / Unified envelope (legacy `/profile` already uses the new envelope) |

旧版完整逐字文档：[`API_LEGACY.md`](API_LEGACY.md)。旧版永不删除（除非大版本公告），但不再加功能。

Full verbatim legacy docs: [`API_LEGACY.md`](API_LEGACY.md). Legacy APIs are never removed (unless announced in a major version), but receive no new features.

---
**导航 Navigation:** [文档索引 Docs](README.md) · [QUICKSTART](QUICKSTART.md) · [RULES](RULES.md) · [API v1](API.md) · [旧版API Legacy](API_LEGACY.md) · [DEPLOY](DEPLOY.md) · [SIM](SIM.md) · [SDR](SDR.md) · [MIGRATION](MIGRATION.md)
