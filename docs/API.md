# LTE-System API v1（标准接口）

> 旧版根路径接口（`/start`、`/stop`…）仍可用但已冻结，见 [`API_LEGACY.md`](API_LEGACY.md)。
> 机器可读契约：[`api/openapi.yaml`](api/openapi.yaml)（OpenAPI 3.0）。

- 基地址：`http://<服务器IP>:8081`（服务器本机 `http://127.0.0.1:8081`，局域网如 `http://192.0.2.10:8081`）
- 统一包络：`{"code": int, "message": str, "data": obj|null, "request_id": str}`，`code 0` = 成功
- `code = HTTP状态码*100+序号`（如 `40401` → HTTP 404），对照表见 §错误码
- 每个响应带 `X-Request-ID` 头；服务端按 `rid=<id> <METHOD> <PATH> -> <状态> (<耗时>)` 记审计日志（`docker logs` 可查谁何时调了什么）
- 可选鉴权：服务端设 `LTE_API_TOKEN` 后，所有接口（新旧）都要带 `Authorization: Bearer <token>`，否则 401。默认未设 = 局域网开放模式（启动日志有 WARNING）
- 404/405 也是 JSON 包络（旧版根路径 404 保持纯文本，不变）

目录：[§1 小区](#1-小区-cell) · [§2 终端](#2-终端-ue) · [§3 爆破](#3-爆破-crack) · [§4 配置上传](#4-配置上传) · [§5 抓包下载](#5-抓包下载-captures) · [§6 写卡](#6-写卡-simcards) · [§7 存档与健康](#7-存档与健康) · [§8 错误码](#8-错误码) · [§9 旧版新版对照表](#9-旧版新版对照表)

---

## 1. 小区 cell

### POST /api/v1/cell — 启动

启动顺序与耗时同旧版（渲染配置 → srsepc → NAT → srsenb → tcpdump，约 6–10 秒，curl 设 `--max-time 100`）。

字段与旧 `/start` 一致（band/apn/mcc/mnc/network/sdr/device_args/tx_gain/rx_gain/n_prb/full_net_name/short_net_name/dns，见 openapi.yaml），另有三处**标准化差异**：

1. **空 `{}` 复用存档**（`/data/last_start.json`，行为同旧版，见 `RULES.md`）
2. **严格校验**：缺字段/非法值/未知 band/非法 `n_prb`（仅允许 6/15/25/50/75/100）/`tx_rx` 越界（0–90）一律 **422**，`data.errors` 逐字段说明；**未知 band 不再静默回退**
3. TDD band 成功时 `data.warning` 提示上行注意事项

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
| body 非 JSON | 400 | 40001 | `malformed JSON body` |
| 校验失败 | 422 | 42201 | `validation failed` |
| 已在运行 | 409 | 40901 | `cell already running` |
| 无 SDR | 503 | 50301 | `no SDR device attached` |
| 网卡名不存在 | 422 | 42201 | `unknown network interface "xxx"` |
| 其他失败 | 500 | 50001 | `start failed: <原因>` |

### GET /api/v1/cell — 状态

```json
{"code":0,"message":"ok","data":
 {"running":true,"epc":true,"enb":true,"pcap":true,
  "started_at":"2026-09-05T10:06:48Z","band":"7","apn":"srsapn","net_name":"MyLTE"}}
```

空闲时 `data` 为 `{"running":false,"epc":false,"enb":false,"pcap":false}`。

### DELETE /api/v1/cell — 停止（幂等）

运行中 → `200 {"code":0,"message":"cell stopped","data":{"stopped":true}}`；
本来就没跑 → 同样 200，`stopped:false`（旧版此处返回“失败”，v1 改为幂等成功）。

```bash
curl -X DELETE http://127.0.0.1:8081/api/v1/cell
```

## 2. 终端 ue

### GET /api/v1/ue — 附着终端快照（第一台）

```json
{"code":0,"message":"ok","data":{"apn":"srsapn","imsi":"001010123456789","ip":"172.16.0.2"}}
```

取不到的字段为 `null`（部分终端不走 ESM Information 流程时 `apn` 为 null，属正常）。

| 情况 | HTTP | code |
|---|---|---|
| 成功 | 200 | 0 |
| 基站没跑 | 412 | 41201 `cell not running` |
| 无终端 | 404 | 40401 `no UE attached` |

## 3. 爆破 crack

### POST /api/v1/crack/jobs — 开始爆破（⚠️ 会停基站）

从 S1AP 包提 CHAP → 停基站 → 后台 `hashcat -m 4800`。成功返回 **202**：

```json
{"code":0,"message":"crack job running",
 "data":{"state":"running","poll":"/api/v1/crack/result"}}
```

| 情况 | HTTP | code |
|---|---|---|
| 已有任务在跑 | 409 | 40901 |
| 无终端数据 | 404 | 40401 |
| pcap 无 CHAP（终端不用 CHAP，如 iPhone） | 412 | 41201 |
| 停基站失败 | 500 | 50001 |

### GET /api/v1/crack/result — 取结果

| data.state | 含义 |
|---|---|
| `running`（200） | 还在跑，稍后轮询 |
| `ready`（200） | 成功，带 `apn/imsi/ip/username/password` |
| 无（404） | 字典无此口令（`password not in dictionary`，带 `username`）/ 无终端数据 |

## 4. 配置上传

`POST /api/v1/config/subscribers`（字段 `userdb` → `/data/conf/user_db.csv`）与
`POST /api/v1/config/wordlist`（字段 `wordlist` → `/data/wordlist.list`），`multipart/form-data`，上限 8MB。

成功 200，`data.note` 提示生效时机（卡库需重启 EPC，字典下次爆破即用）：

```json
{"code":0,"message":"upload success",
 "data":{"bytes":2645,"note":"takes effect after restart (EPC reads at boot)"}}
```

缺文件 422，超限 413（`upload exceeds limit`，带 `max_bytes`），失败 500。

```bash
curl -X POST http://127.0.0.1:8081/api/v1/config/subscribers -F userdb=@user_db.csv
```

## 5. 抓包下载 captures

`GET /api/v1/captures/{id}`，`id` 取 `lte-data`（业务流量）/`s1ap`/`enb`/`epc`（兼容旧数字 `0-3`）。
成功直接下载；未知 id 或文件未就绪 → 404。

```bash
curl -OJ http://127.0.0.1:8081/api/v1/captures/lte-data
```

> 想下 EPC/eNB 运行日志？暂不支持（在规划中），目前用 `docker exec ltesystem tail /data/log/srsLTE_epc.log`。

## 6. 写卡 simcards

`POST /api/v1/simcards`，body 同旧 `/writesim` 全字段（`imsi` 必填，其余可选，见 openapi.yaml；
**未知字段忽略**——旧版会直接拒绝，这是故意的向前兼容差异）。最长 180s。

| 情况 | HTTP | code |
|---|---|---|
| 写卡成功 | 200 | 0，`data.result=programmed` |
| 卡已在库 | 409 | 40901，`data.result=exists` |
| 无读卡器 | 503 | 50301 |
| 没插卡 | 412 | 41201 |
| 参数非法 | 422 | 42201 |
| 入库失败/写卡失败 | 500 | 50001 |

无读卡器环境调本接口恒定 503，属正常（跳过即可，不影响入网）。

## 7. 存档与健康

- `GET /api/v1/profile` → `200 {"code":0,…,"data":{"has_profile":true,"profile":{…},"ues":[{"name","auth","imsi"}]}}`（无密钥；无存档时无 `profile` 字段）。旧 `GET /profile` 已统一为同一包络。
- `GET /api/v1/health` → `200 {"code":0,…,"data":{"ok":true,"running":bool,"sdr":{…},"time":"…"}}`（旧 `GET /healthz` 保留原样）。

## 8. 错误码

| code | HTTP | 含义 |
|---|---|---|
| 0 | 200 | 成功（DELETE 停止空闲、GET 轮询 `running` 等也属成功） |
| 40001 | 400 | body 非 JSON / multipart 坏 |
| 40101 | 401 | 缺少或错误的 Bearer token（仅设 `LTE_API_TOKEN` 时） |
| 40401 | 404 | 资源不存在（未知路径/capture id/无终端/字典无口令/无 UE 数据） |
| 40501 | 405 | 方法不允许 |
| 40901 | 409 | 冲突（小区在跑/爆破在跑/卡已在库） |
| 41201 | 412 | 前置条件不满足（小区没跑/无 CHAP/没插卡） |
| 41301 | 413 | 上传超限（带 `max_bytes`） |
| 42201 | 422 | 校验失败（`data.errors:[{field,reason}]`） |
| 50001 | 500 | 内部失败（message 带原因） |
| 50301 | 503 | 硬件缺失（无 SDR/无读卡器） |

## 9. 旧版新版对照表

| 旧版（根路径，已冻结） | 新版（`/api/v1`） | 行为差异 |
|---|---|---|
| `POST /start` | `POST /api/v1/cell` | 未知 band 由静默回退改为 422；`n_prb`/增益/网卡加入校验；TDD 成功带 warning |
| `POST /stop` | `DELETE /api/v1/cell` | 空闲停止由“失败”改为幂等成功 |
| `POST /basicinfo` | `GET /api/v1/ue` | 无终端由包络失败改为 404 |
| `POST /crackapn` | `POST /api/v1/crack/jobs` | 成功改 202；无 CHAP 由失败改为 412 |
| `POST /getcrackresult` | `GET /api/v1/crack/result` | `running`/`ready` 状态机替代真假值 |
| `POST /userupload` | `POST /api/v1/config/subscribers` | 成功带生效时机 note；超限改 413 |
| `POST /passwordupload` | `POST /api/v1/config/wordlist` | 同上 |
| `POST /getfile {fileid}` | `GET /api/v1/captures/{id}` | 字符串 id（数字别名兼容） |
| `POST /writesim` | `POST /api/v1/simcards` | 未知字段由拒绝改忽略；卡已在库改 409 |
| `GET /healthz`·`/status`·`/profile` | `GET /api/v1/health`·`/api/v1/cell`·`/api/v1/profile` | 统一包络（旧 `/profile` 已同步新包络） |

旧版完整逐字文档：[`API_LEGACY.md`](API_LEGACY.md)。旧版永不删除（除非大版本公告），但不再加功能。
