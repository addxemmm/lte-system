# 架构说明：v1.x（Python）→ v2.x（Go + srsRAN_4G） Architecture: v1.x (Python) to v2.x (Go + srsRAN_4G)

v1.x 实现已移出工作树，见 git 标签 `archive/v1-python`（HTTP 服务 + `run.sh`/`stop.sh` 脚本 + 守护配置），下表是两代实现的对照：

v1.x implementation was removed from the working tree; see git tag `archive/v1-python` (HTTP service + `run.sh`/`stop.sh` scripts + daemon config). The table below compares the two generations:

| v1.x | v2.x（本仓库） v2.x (This Repository) | 说明 Description |
|---|---|---|
| Python HTTP 服务 `:8081` / Python HTTP service `:8081` | [`cmd/server`](../cmd/server) + [`internal/api`](../internal/api)（stdlib） | 9 接口 + `message_id` 语义保留 / 9 endpoints + `message_id` semantics preserved |
| shell echo 生成 conf / Generate conf via shell echo | [`internal/lte`](../internal/lte) text/template | 去掉绝对路径硬编码；band→EARFCN 表保留 / Remove absolute-path hard-coding; band-to-EARFCN table preserved |
| `ps \| grep srs` 状态机 / state machine | [`internal/sysop`](../internal/sysop) pgrep + 进程组 / process group | 不再误杀同名进程 / No longer kills unrelated processes with the same name |
| `os.popen` shell 拼接 / shell concatenation | `exec.Command` 数组 + 校验 / array + validation | 防注入（APN/IMSI/band） / Prevents injection via APN/IMSI/band |
| `tcpdump &` | Manager 子进程 / child process | stop 联动 kill / Kill linked with stop |
| `tshark … grep -A 7` 下标解析 / index-based parsing | [`internal/crack`](../internal/crack) 正则解析 / regex parsing | 容忍 tshark 版本漂移 / Tolerates tshark version drift |
| `hashcat --force &` | `StartAsync` + `--show` | 相同 `-m 4800` / Same `-m 4800` |
| 写卡全硬编码 / Fully hard-coded SIM programming | [`internal/sim`](../internal/sim) + `configs/app.yaml` 默认 / defaults | IMSI 必填，其余可选 / `IMSI` required, others optional |
| `user_db.csv[1738:]` 魔数切片 / magic-number slicing | CSV 解析 + `ueN` 自增 / parsing + auto-increment | 并发追加安全 / Safe concurrent appends |
| `/home/workspace/log` 硬路径 / hard-coded path | `LTE_DATA_DIR`(`/data`) | compose 卷持久化 / Persisted via compose volume |
| srsLTE（停更） / srsLTE (discontinued) | srsRAN_4G `release_23_11` | 二进制名不变（`srsenb`/`srsepc`），日志关键字不变 / Binary names unchanged (`srsenb`/`srsepc`), log keywords unchanged |
| srsLTE `drb.conf`/`drb_config` | srsRAN_4G `rb.conf`/`rb_config` | 上游改名，旧 key 直接导致 eNB 退出 / Upstream rename; the old key makes the eNB exit immediately |
| srsLTE `rr.conf`（`meas_report_desc` 旧格式，无 `ul_earfcn`） / srsLTE `rr.conf` (legacy `meas_report_desc` format, no `ul_earfcn`) | 上游 `rr.conf.example` 格式 + 每次按频段渲染 / Upstream `rr.conf.example` format, rendered per band on each start | 旧测量报告格式新版不认；TDD 下 UL 推导失败，必须显式 `ul_earfcn`（FDD=DL+18000，TDD=DL） / New version rejects the legacy measurement report format; UL derivation fails under TDD, so `ul_earfcn` must be explicit (FDD=DL+18000, TDD=DL) |
| `ps \| grep` counted 僵尸进程 / zombie processes counted via `ps \| grep` | `ps -eo pid,stat,comm` 排除 `Z` 状态 + kill 后 `Wait()` 回收 + 自然退出自动回收 / excludes `Z` state, `Wait()` reaping after kill and auto-reap on natural exit | 崩溃残留的 `<defunct>` 不再误判为运行中；eNB 初始化失败直接报错不装活 / Leftover `<defunct>` after crashes is no longer misreported as running; eNB init failure fails fast instead of pretending to run |
| 固定运营商显示名 `srsRAN` / Fixed carrier display name `srsRAN` | `/start` 新增 `full_net_name`/`short_net_name`（NITZ 下发） / New in `/start`, delivered via NITZ | 默认仍是 `srsRAN`，传参即改，`/status` 回显 / Still `srsRAN` by default, changed via parameters, echoed by `/status` |
| DNS 写死 `8.8.8.8` / DNS hard-coded to `8.8.8.8` | `/start` 新增 `dns` + `default_dns` 配置 / New `dns` + `default_dns` config in `/start` | 上行过滤公网 DNS 时可改网关/内网 DNS / When uplink filters public DNS, switch to the gateway/intranet DNS |
| 只加 MASQUERADE 就上网 / Only MASQUERADE for Internet access | 每次 `/start` 额外确保 `DOCKER-USER` 放行 + TCP MSS 钳制，`/stop` 清理 / Each `/start` additionally ensures `DOCKER-USER` allowlisting + TCP MSS clamping, cleaned up by `/stop` | Docker 新版默认 FORWARD DROP 会静默丢掉 UE 流量；GTP 路径需要 MSS clamp / New Docker defaults to FORWARD DROP and silently drops UE traffic; the GTP path needs MSS clamp |

`getfile` 缺文件时 v1.x 误回 `message_id 2`，v2.x 按文档返回 `3`（唯一有意的行为修正）。

When files are missing, `getfile` in v1.x wrongly returned `message_id 2`; v2.x returns `3` as documented (the only intentional behavior fix).

旧资产对应位置：git 标签 `archive/v1-python`（v1.x 实现全文 + 原始多用户种子，[`configs/user_db.csv.example`](../configs/user_db.csv.example) 即沿用该多种子）、[`docs/legacy/`](legacy)（早期中文文档存档）、[`docs/samples/`](samples)（成功入网日志样本 + 白卡 ATR）。

Legacy asset locations: git tag `archive/v1-python` (full v1.x implementation + original multi-user seeds; [`configs/user_db.csv.example`](../configs/user_db.csv.example) keeps that multi-user seed), [`docs/legacy/`](legacy) (early Chinese doc archive), [`docs/samples/`](samples) (successful attach log samples + test SIM ATR).

## 2026-09 审查加固 / Audit hardening

- 子进程只有一个 Wait 调用者，通过 done channel 发布退出状态；信号退出也属于停止。
  Each child has one Wait owner; a done channel publishes completion, including signal exits.
- Status/Stop 只报告和终止本 Manager 创建的子进程；同名外部进程只阻止新启动，不接管或误杀。
  Status/Stop only report and stop children created by this Manager. Foreign same-name processes block startup but are never adopted or killed.
- Start 初始化等待遵守取消并回滚；父日志描述符及时关闭。仅清理由本轮确实新增的 NAT/转发规则，idle Stop 无主机规则副作用。
  Startup waits honor cancellation and roll back; parent log descriptors are closed. Only rules actually added by this run are removed; idle Stop does not change host rules.
- SIM 写入、订户 CSV 追加、API 卡库替换和首次 seed 共用进程内互斥；认证配置冲突在硬件操作前报错。此锁不协调外部 EPC 写回，修改卡库前应停止小区。
  SIM programming, subscriber append, API database replacement and first seed share a process-local mutex. Authentication conflicts fail before hardware access. External EPC writes are not coordinated: stop the cell before database changes.
- JSON 必须是单个对象；上传采用唯一临时文件原子替换；v1 超限统一 413，401/panic 也有请求 ID 与审计。
  JSON must be one object; uploads atomically replace files from unique temporary paths; v1 over-limit requests return 413, and 401/panic responses are correlated and audited.
- 部署使用排除私有数据的独立快照；按用户部署需求，Compose 默认全网卡 `0.0.0.0:8081` 监听且可传令牌。发布保持实际数据卷并保存旧镜像，详见 [DEPLOY](DEPLOY.md)。
  Deployment uses fresh snapshots excluding private state. Per the deployment requirement, Compose defaults to all-interface `0.0.0.0:8081` binding and passes tokens. Releases retain the actual data volume and old image; see [DEPLOY](DEPLOY.md).

---
**导航 Navigation:** [文档索引 Docs](README.md) · [QUICKSTART](QUICKSTART.md) · [RULES](RULES.md) · [API v1](API.md) · [旧版API Legacy](API_LEGACY.md) · [DEPLOY](DEPLOY.md) · [SIM](SIM.md) · [SDR](SDR.md) · [MIGRATION](MIGRATION.md)
