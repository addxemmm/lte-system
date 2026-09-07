# LTE-System API v3

> **2.1 发布更新 / Release update:** 本契约已配套部署于 2.1，restricted 已显式启用。服务器构建、CPU 回归和正常 APN 路径注册已观察；错误 APN 手机行为和 Internet 验收仍待进行。见 [发布记录](RELEASE_2.1_2026-09-07.md)。
> This contract ships in image 2.1 with restricted explicitly enabled. Server build/CPU regressions and normal-path registration were observed; incorrect-APN handset behavior and Internet acceptance remain pending.


机器可读契约见 [`api/openapi.yaml`](api/openapi.yaml)。v3 只提供 `/api/v1/*` 标准接口；已删除根路径旧接口以及单终端 `/api/v1/ue`。

The machine-readable contract is [`api/openapi.yaml`](api/openapi.yaml). v3 exposes only the standard `/api/v1/*` surface; root-path legacy endpoints and singular `/api/v1/ue` have been removed.

## 通用约定 Common Contract

- 基地址：`http://HOST:8081`；客户端不要使用监听地址 `0.0.0.0`。
- JSON 包络：`{"code":int,"message":string,"data":object|null,"request_id":string}`；成功 `code=0`。
- 每个响应都有 `X-Request-ID`。未知路径和方法分别返回标准 404/405 包络。
- 设置 `LTE_API_TOKEN` 后，每个请求须带 `Authorization: Bearer TOKEN`；日志只记录 request id、方法、路径、状态及耗时，不记录请求体。
- JSON body 必须是单一对象；多余值、数组、`null` 或尾随垃圾返回 400。上传默认上限 8 MiB。

## 1. 小区 Cell

### `POST /api/v1/cell`

默认启动请求应发送最小 body `{}`，继承已验证的 `/data/last_start.json`，避免 Postman 意外覆盖现场 DNS、接口和隔离策略。没有存档的首次配置才显式发送参数：

```json
{
  "band":"7", "apn":"srsapn", "mcc":"001", "mnc":"01",
  "network":"auto", "dns":"192.168.100.1",
  "ue_subnet":"172.16.0.0/24", "ue_access":"isolated",
  "apn_mismatch_policy":"strict"
}
```

`network` 省略、空或 `auto` 时按当前网络命名空间的 IPv4 默认路由 metric 解析；`GET /api/v1/cell` 的 `network` 是请求策略，`resolved_network` 是实际接口。`dns` 必须是 UE 可达的 resolver。当前只支持一个配置 APN/一个 SGi 网段；`ue_subnet` 必须是 RFC1918 `/24`，默认 `172.16.0.0/24`；`ue_access=isolated|allow`，默认 `isolated`。

2.1 新增参数 `apn_mismatch_policy=strict|restricted`：缺省/旧 profile 为 strict（错误 APN 拒绝注册）；restricted 对已通过原有 SIM/AKA 鉴权、格式合法但显式不匹配的 APN 建立受限默认承载，并在 SPGW 丢弃双向用户面。正确/省略 APN 保持正常；异常 APN 仍拒绝。参数保存到 profile，空请求继承，显式 strict 可覆盖；非法值返回字段级 422。`ue_access=allow` 不解除 restricted 限制。新版本需成套构建、部署并明确启用后生效，详见 [受限接入](APN_RESTRICTED_ACCESS_2026-09-07.md)。

Unreleased `apn_mismatch_policy` defaults to strict for compatibility. Explicit restricted mode retains normal subscriber/AKA checks and admits only syntactically valid APN mismatches with a restricted default bearer and bidirectional SPGW drops. The effective setting is inherited/persisted with the startup profile; invalid values return 422. This is not a claim that the current server supports the new mode.

成功 200；非法字段 422；已运行 409；无 SDR 503。启动可能需要约 6–10 秒。

### `GET /api/v1/cell`

返回 Manager 管理的 EPC/eNB/pcap 状态、存档配置，以及 `network`/`resolved_network`。状态不等于终端已注册或公网可达。

### `DELETE /api/v1/cell`

幂等停止；运行中返回 `stopped:true`，本来空闲返回 `stopped:false`，均为 200。

### `GET /api/v1/network`

只读返回 Manager 当前生效的 UE 网络计划：

```json
{"code":0,"message":"ok","data":{"active":true,"ue_subnet":"172.16.0.0/24",
 "sgi_address":"172.16.0.1","ue_access":"isolated","resolved_network":"eth0"},
 "request_id":"..."}
```

`active=false` 表示当前没有生效的 Manager 网络计划。该端点不发探测、不修改规则，也不推断 Internet、DNS 或任意目标的可达性；需要分层证据时使用 `/api/v1/diagnostics/connectivity`。

## 2. UE 会话 UE Sessions

### `GET /api/v1/ues`

返回当前 EPC 进程每秒原子发布的结构化快照，不解析或拼接文本日志。消费端兼容 schema 1/2；本地新增 producer 使用 schema 2，须成套升级。以下为 schema 2 的受限会话示例，不是现网验收记录：

```json
{"code":0,"message":"ok","data":{
  "state":"current","cell_state":"running","schema_version":2,
  "run_id":"RUN_ID","sequence":42,"updated_at_unix_ms":1788700000000,
  "count":1,"sessions":[{
    "session_id":"RUN_ID:3","imsi":"001010123456789",
    "mme_ue_s1ap_id":1,"enb_ue_s1ap_id":2,"sctp_assoc_id":4,
    "emm_state":"registered","ecm_state":"idle","ue_ipv4":"172.16.0.2",
    "requested_apn":"wrong-apn","apn_source":"pdn_request",
    "selected_apn":"internet","apn_validated":false,
    "access_policy":"restricted","access_reason":"apn_mismatch",
    "bearers":[{"ebi":5,"qci":7,"state":"active"}]
  }]},"request_id":"..."}
```

- `state=current` 表示快照归属与新鲜度已验证，`cell_state=running` 是 producer 状态。`registered + idle` 表示已附着但当前无 S1 无线连接；不等于掉线，也不证明 Internet 可达。
- IMSI 在认证前可为空；这种上下文仍出现在列表，但不能通过 IMSI 详情查询。
- `requested_apn` 是 UE 实际请求值。schema 1 保留 `selected_apn` 要求 `apn_validated=true` 的原契约；schema 2 的 selected APN 表示实际下发配置，restricted 时仍为非空，但 `apn_validated=false`，不伪装为请求匹配。
- schema 2 每个会话必须带 `access_policy=normal|restricted|deny` 与 `access_reason=apn_match|apn_omitted|apn_mismatch|session_unavailable`。normal 对应校验成功，restricted 对应合法显式错误 APN，deny 对应尚未确认/不可用会话。未知、缺失、null 或矛盾组合拒绝；schema 1 不接受这两个新字段。注册状态与 Internet 访问权限分开解释。
- API 先读文件 metadata、有界读取最多 1 MiB、再核对 metadata；只接受匹配当前 `run_id`/PID、schema 1 或 2、`state=running`、无重复会话且更新时间不超过 5 秒的快照。列表最多 256 个会话、每会话最多 16 个 bearer。
- 没有可靠当前快照仍返回 200，`data.state=missing|stale|invalid|cell_stopped` 且 `sessions=[]`；这不表示“没有 UE”。不会回退到可能跨 UE 拼错字段的 EPC 日志。

### `GET /api/v1/ues/{imsi}`

只查完整当前快照中带 IMSI 的会话。完整快照确认不存在时返回 404；快照缺失/过期/非法时返回上述状态而不是虚构 404。IMSI 必须是 15 位数字。本 API 没有伪造的 disconnect/delete 操作。

## 3. 授权订户 Subscribers

配置授权与当前活跃会话是不同资源：`/subscribers` 描述允许认证的 HSS 行，`/ues` 描述运行态。订户响应只含：

```json
{"name":"phone","auth":"mil","imsi":"001010123456789","op_type":"opc",
 "qci":7,"ip_alloc":"dynamic","authorized":true,"active":null,
 "credential_status":"configured"}
```

`active=true` 只表示完整当前快照中该 IMSI 的 EMM 状态为 `registered`（ECM 可为 idle）；`false` 表示完整快照中未注册，`null` 表示没有可安全关联的当前快照。不能把“已授权”当作“在线”。任何响应或校验错误都不返回 Key、OP/OPc、AMF、SQN 或其 hash。

### `GET /api/v1/subscribers?limit=50&offset=0`

分页列表；`limit` 为 1–200。数据库缺失返回空数组，CSV 非法则整次失败 500，不静默跳过坏行。

### `GET /api/v1/subscribers/{imsi}`

返回一条脱敏配置；不存在 404。

### `POST /api/v1/subscribers`

仅停站时可新增，成功 201。body 包含 `name,auth,imsi,key,op_type,op|opc,amf,sqn,qci,ip_alloc`；name 与 IMSI 均须唯一，IMSI 为 15 位数字，Key 与 OP/OPc 为 32 hex，AMF 为 4 hex，SQN 为 12 hex，QCI 为 1–9。本 API 只新增 `ip_alloc=dynamic`；已存在 static 值可读，但不能由 API 新增或修改。

### `PATCH /api/v1/subscribers/{imsi}`

仅停站时可改 `name`、`qci`、`ip_alloc`（新值仅 `dynamic`；已存在 static 值为只读）。`key/op/opc/op_type/auth/amf/sqn` 即使传 `null` 也一律 422；尤其不会覆盖 EPC 最新 SQN。

### `DELETE /api/v1/subscribers/{imsi}`

仅停站时删除授权行；返回 `disconnected:false`，因为删除配置不等于断开 UE，也没有伪造的在线控制能力。

所有写操作由 Manager 生命周期锁原子确认停站，再按固定锁序取得 subscriber 锁；运行中返回 409。写入使用权限 `0600` 的同目录临时文件、fsync、完整验证后原子替换。

### `POST /api/v1/config/subscribers`

停站批量替换 `user_db.csv`，multipart 字段 `userdb`。严格校验每行恰好 10 列、大小/行数、重复 name/IMSI/static IP、QCI 与认证字段格式；任一失败 422 且原文件不变。对上传中已存在的 IMSI，无条件保留磁盘最新 SQN；用于凭据轮换，不可回滚 EPC SQN。成功 data 含 `sqn_policy=preserved_for_existing_imsi`。

## 4. 连通性诊断 Connectivity Diagnostics

`GET /api/v1/diagnostics/connectivity` 是“已入网但不能上网”的只读首选：不停止/重启小区、不新增抓包、不发探测包、不改规则，也不返回 IMSI 或 CHAP 凭据。

- `registration` 与 `pdn` 是有界日志窗口的 aggregate-only 证据，不跨 UE 拼接。
- `chap` 与 LTE attach/AKA 分离；`not_collected`、缺 tshark、超时、不可解码、未观察到握手分别表达。iPhone 不必使用 CHAP。
- `user_plane`/`dns` 统计已有 SGi pcap 的上下行及 DNS 请求/响应；UE 方向只按当前 `NetworkPlanSnapshot.ue_subnet` 分类并返回 `classification_subnet`，不会硬编码或回退到另一网段。部分可解码文件保留正证据并标注 scan 不完整。
- `network` 只是配置证据，规则存在不证明公网可达。
- 同时只运行一个诊断；并发请求立即 429/`diagnostic_busy`。每项 tshark 使用 `-n`、3 秒 context budget/128 KiB 输出上限，pcap 最大 64 MiB；文件 IO 为协作式取消，5 秒是子检查 budget，不是包含文件 IO、进程回收和启动锁等待的严格总墙钟 SLA。

### 抓包完整性 / Capture integrity

S1AP 文件可能正在写入，最后一条 packet 尚未写完并不等于整个文件损坏。诊断只读取一个有界、权限 0600 的完整 record 前缀临时副本，检查结束即清理；原始抓包不改动。支持 classic-PCAP 的 micro/nanosecond 大小端格式与本项目 DLT 150；其他格式显式报错。读取或检查期间源文件变化，`scan_complete=false`。

A live S1AP capture can end mid-record. Diagnostics inspect a bounded private immutable complete-record prefix, then remove it, without modifying the original. Classic-PCAP micro/nanosecond byte orders and this project's DLT 150 are supported. Source mutation makes the scan incomplete.

- `capture_incomplete`：未写完或检查期间变化，`state=unknown`，不是“没有 CHAP”。
- `capture_format_invalid` / `capture_linktype_unsupported`：格式或链路类型不符。
- `capture_no_packets`：仅文件头，尚无完整 packet。
- `capture_read_failed` / `capture_not_regular`：读取失败或非普通文件。
- `inspection_timeout` / `inspection_cancelled`：副本检查超时或请求取消；与 `tshark_timeout` 分开。
- `tshark_incompatible`：字段或配置选项不受支持；原始 stderr 不回显。
- `pap_frames_observed`：仅观察到 PAP 协议存在性，不读取用户名或密码。

`chap.capture` 可新增 `snapshot_size_bytes`、`complete_packets`、`incomplete`。`chap_observed=true` 只表示协议元数据存在，不保证完整认证交换，更不表示成功恢复密码。`scan_complete=false` 时不得据未观察结果推断协议不存在；完整性 reason 由只读诊断及既有失败诊断分支提供。本段描述早期诊断增量；后续认证接口的发布边界见第 5 节。

Optional capture metadata reports prefix size, complete-packet count and incompleteness. Protocol presence is not proof of a complete exchange or recovered credentials. Never interpret an incomplete negative scan as absence. This paragraph describes the earlier diagnostics increment; section 5 defines the later authentication endpoint boundaries.

UE 清单新方案仍是[设计](UE_PRESENCE_DESIGN_2026-09-07.md)，未上线；上文现行 UE/subscriber 契约保持不变。
The new UE-presence scheme remains a design, not a released change to UE/subscriber semantics.

## 5. 其他标准接口 Other Standard Endpoints

- `POST /api/v1/crack/jobs`：空 body 保留旧的整份抓包审计流程，不声明目标归属。显式 `{"imsi":"15位","confirm_ownership":true}` 先校验订户：未知身份 404、数据库读取失败 500；即使订户存在，因尚无可靠的每 UE 凭据绑定仍返回 412，不提取凭据、不停止小区、不启动任务。旧流程先查 PAP（不停小区），无 PAP 再检查 CHAP/字典后停止小区并启动作业（202）；字典缺失/空返回 412。成功结果的 `ownership_verified` 始终为 false；`apn/imsi/ue_ipv4` 仅是 EPC 聚合上下文，不证明凭据归属。
  An empty body retains the legacy capture-wide audit without target attribution. Explicit IMSI requests return 404 for an unknown subscriber, 500 on database failure, or 412 when the subscriber exists but per-UE credential binding is unavailable. Targeted calls return no credentials and cause no cell/job side effects. Legacy successful results always carry `ownership_verified=false`; enrichment is aggregate context only.
- `GET /api/v1/crack/result`：旧流程返回 PAP `ready`、CHAP `running|ready` 或相应错误；显式 `?imsi=` 同样校验订户并在缺少可靠绑定时返回 412，不返回凭据。`no_password` 只表示字典未命中，不等于强口令。完整 record 前缀可避免未写完的尾记录，但并不能解决并发 CHAP identifier 复用或建立每 UE 关联。
  Explicit IMSI result queries fail closed without a reliable per-UE association. A dictionary miss is not proof of password strength; complete-record snapshots do not solve CHAP identifier reuse or establish credential ownership.
- `POST /api/v1/config/wordlist`：multipart `wordlist`，原子替换，下次作业使用。
- `GET /api/v1/captures/{id}`：`lte-data|s1ap|enb|epc`；成功为文件，未知/未就绪 404。
- `POST /api/v1/simcards`：最长约 180 秒；整个写卡及入库事务只允许停站并与 Start 原子互斥，运行中 409。无读卡器 503，无卡 412。
- `GET /api/v1/profile`：只返回 `has_profile` 与 `profile?`；订户见 `/subscribers`，运行态见 `/ues`。
- `GET /api/v1/health`：返回服务、Manager running 状态及 SDR 检测；不表示 UE 或公网健康。

## 6. 错误码 Error Codes

| code | HTTP | 含义 |
|---|---:|---|
| 0 | 200/201/202 | 成功 |
| 40001 | 400 | 非法 JSON/multipart |
| 40101 | 401 | Bearer token 缺失或错误 |
| 40401 | 404 | 路径或已确认资源不存在 |
| 40501 | 405 | 方法不允许 |
| 40901 | 409 | 生命周期/重复资源/作业冲突 |
| 41201 | 412 | CHAP/capture/硬件等前置条件不足 |
| 41301 | 413 | body/upload 超限 |
| 42901 | 429 | 诊断执行槽忙 |
| 42201 | 422 | 字段或 subscriber CSV 校验失败 |
| 42202 | 422 | capture 不可可靠解码 |
| 50001 | 500 | 内部失败或现有 subscriber DB 非法 |
| 50301 | 503 | SDR/读卡器缺失 |
| 50302 | 503 | tshark 等依赖不可用 |

---
**导航 Navigation:** [文档索引](README.md) · [迁移](MIGRATION.md) · [Postman](../postman/README.md)
