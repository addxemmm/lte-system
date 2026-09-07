# 架构与迁移 / Architecture and migration

> 当前版本为 3.0，只保留标准 `/api/v1`。以下 v1→v2 对照是历史记录，不代表旧接口仍可调用；最新变更见文末。
> Version 3.0 exposes only standard `/api/v1`. The v1→v2 comparison below is historical, not an active legacy API contract; see the latest changes at the end.

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
| 只加 MASQUERADE 就上网 / Only MASQUERADE for Internet access | 每次 `/start` 校验 IPv4 转发、确保 namespace 内精确 FORWARD + 双向 MSS，`/stop` 仅清理自有规则 / Each `/start` validates IPv4 forwarding and ensures scoped namespace FORWARD + bidirectional MSS; `/stop` cleans only owned rules | Docker 新版默认 FORWARD DROP 会静默丢掉 UE 流量；GTP 路径需要 MSS clamp / New Docker defaults to FORWARD DROP and silently drops UE traffic; the GTP path needs MSS clamp |

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

## 2026-09 NAS 互通修复 / NAS interoperability fixes

srsRAN 仍基于 `release_23_11` / `eea87b1d893ae58e0b08bc381730c502024ae71f`，镜像构建时应用 [`third_party/srsran`](../third_party/srsran) 中的显式补丁，并运行纯 CPU C++ 回归。升级上游版本时必须重新审查补丁，构建会拒绝不匹配的基线。详见 [终端诊断](UE_DIAGNOSTICS_2026-09-06.md)。
srsRAN remains based on `release_23_11` / `eea87b1d893ae58e0b08bc381730c502024ae71f`. Image builds apply explicit patches from [`third_party/srsran`](../third_party/srsran) and run CPU-only C++ regressions. Upstream upgrades require patch review; a mismatched baseline fails the build. See [UE diagnostics](UE_DIAGNOSTICS_2026-09-06.md).

额外 PDN 拒绝修复消息结构/内层 ESM 解包与响应安全封装；IPv4-only 默认承载按原始 IPv4v6 请求有条件附带 cause 50。没有增加 IPv6、IMS 或 VoLTE，也没有改动 BlackSDR FPGA、射频配置、SIM 内容或 API 契约。
Additional-PDN rejection fixes message/inner-ESM decoding and response security protection. IPv4-only default bearers conditionally include cause 50 for original IPv4v6 requests. This adds no IPv6, IMS or VoLTE and changes no BlackSDR FPGA, RF configuration, SIM contents or API contract.

## 2026-09 Bridge 与只读联网诊断 / Bridge and read-only connectivity diagnostics

- `docker-compose.yml` 保留为 host 兼容/回滚入口；`docker-compose.bridge.yml` 独立使用，单网络固定 eth0，只发布 API。两者共用相同逻辑数据卷和 compat FPGA 设置，迁移须保留原 Compose 项目名。不要合并两个编排文件。
  `docker-compose.yml` remains the host compatibility/rollback entry; use `docker-compose.bridge.yml` alone for one fixed eth0 uplink and API-only publishing. Both retain the same logical data volume and compat FPGA setting; preserve the original Compose project. Never merge the two files.
- 新启动允许 `network: "auto"`，按当前 namespace 的主路由表默认 IPv4 出口解析。保存策略而不是解析后的网卡名；`resolved_network` 另行报告。旧 profile 的显式接口保持兼容，但跨 namespace 迁移时须明确覆盖为 auto。
  New starts accept `network: "auto"`, resolving the current namespace's main-table default IPv4 uplink. Persist the policy, not the resolved interface; report `resolved_network` separately. Old explicit profile interfaces remain compatible, but namespace migration must explicitly override them with auto.
- 启动前检查 IPv4 转发；修复 mangle argv 顺序和静默忽略错误，精确 SGi/UE/出口规则适用于 host 与 bridge，错误回滚只处理本 Manager 新增项。复杂 policy routing/ECMP 不等同于当前 main-table auto 选择。
  Check IPv4 forwarding before startup; fix mangle argument order and swallowed errors. Scoped SGi/UE/uplink rules work in host and bridge, with rollback limited to rules added by this Manager. Complex policy routing/ECMP is not equivalent to current main-table auto selection.
- `GET /api/v1/diagnostics/connectivity` 只读汇总各层证据，不停止小区、不新增抓包、不启动破解、不返回 CHAP 凭据。无 CHAP 与 LTE 注册失败分开；有 IP/包计数也不直接声称手机互联网可用。
  `GET /api/v1/diagnostics/connectivity` reads layered evidence without stopping the cell, starting a capture/cracker, or returning CHAP credentials. Missing CHAP is distinct from LTE registration failure; an IP or packet count does not prove handset Internet access.

本轮 DNS 取证、限制、验证与发布记录见 [网络诊断](NETWORK_DIAGNOSTICS_2026-09-07.md)，迁移和成套回滚见 [DEPLOY](DEPLOY.md)。
See [network diagnostics](NETWORK_DIAGNOSTICS_2026-09-07.md) for DNS evidence, limitations, validation and release records; [DEPLOY](DEPLOY.md) covers migration and coordinated rollback.

---
**导航 Navigation:** [文档索引 Docs](README.md) · [QUICKSTART](QUICKSTART.md) · [RULES](RULES.md) · [API v1](API.md) · [DEPLOY](DEPLOY.md) · [SIM](SIM.md) · [SDR](SDR.md) · [MIGRATION](MIGRATION.md)

## 3.0：多终端与标准接口 / Multi-UE and standard-only API

- 根路径旧 API 和 `/api/v1/ue` 已删除；迁移到 `/api/v1/ues`、按 IMSI 查询、`/api/v1/subscribers` 和 `/api/v1/network`。删除旧 Postman 集合后导入新版。
  Legacy root APIs and `/api/v1/ue` are removed; migrate to UE collections, IMSI lookup, subscriber management and network planning. Replace old imported Postman collections.
- 当前会话快照取代单条日志推断，严格 APN 策略阻止显式错误值；手机实际 APN 与配置匹配，显示名称可自定义。
  Current session snapshots replace singleton log inference. Strict APN policy rejects explicit mismatches; configure the actual APN, not merely its display label.
- 默认 UE 互相隔离，支持配置私网 /24；保留 bridge、持久化数据和兼容 FPGA。订户/写卡变更必须停站；不强制改写认证 SQN。
  UEs are peer-isolated by default with a configurable private /24. Keep bridge, persistent data and compatible FPGA. Subscriber/SIM mutations require a stopped cell and must not force-rewrite SQNs.
- 接口精确字段见 [API](API.md)，边界与验证见 [多终端报告](MULTI_UE_2026-09-07.md)。
  See [API](API.md) for schemas and the [multi-UE report](MULTI_UE_2026-09-07.md) for scope and validation.

### 3.0 发布交接 / Release handoff

新镜像 `ltesystem-dep:multiue-apn-20260907` 已发布，bridge/8081/BlackSDR 保持，实际同一快照观察到 2 台注册 UE 与独立地址；用户已明确更正早前反馈，确认两台均可同时上网。`folk/multi-ue-apn` 已在服务器验证后合回 `master` 并删除；发布提交 `cd63187` 的 GitHub CI 已通过。构建日志、私有数据备份和原始会话 JSON 仅在服务器，详见 [多终端报告](MULTI_UE_2026-09-07.md)。不自动写卡或改射频参数；后续先读 git log、本文件和 AGENTS。
The new image is deployed with bridge/8081/BlackSDR preserved. Two real registered UEs and distinct addresses were observed in one current snapshot; the user explicitly corrected the earlier report and confirmed concurrent Internet access on both phones. The tested folk branch was merged into master and deleted; release commit `cd63187` passed GitHub CI. Build logs/private backups/raw session evidence stay on the server; see the report. Do not automatically program SIMs or change RF settings; read git log, this file and AGENTS before continuing.

后续项：用户确认小米仍偶发无服务；有效 ServiceRequest 的 association 集合登记遗漏仍需独立清理回归，不将它当作已确认的无线根因。2026-09-07 07:00 UTC 已发布 `ltesystem-dep:enb-log-load-20260907`，只降低 eNB 数据面日志开销，RF/USB/FPGA/网络/profile 不变。Postman 增强断言并新增六个 GET 的只读集合。详见 [稳定性报告](RADIO_STABILITY_2026-09-07.md)。
Follow-ups: the user confirmed intermittent Xiaomi service loss. Missing association-set registration on valid ServiceRequest still requires a separate cleanup regression; it is not a proven radio root cause. At 07:00 UTC, image `ltesystem-dep:enb-log-load-20260907` was deployed to reduce only eNB data-plane logging, preserving RF/USB/FPGA/network/profile. Postman assertions and a six-GET read-only collection were added. See the [stability report](RADIO_STABILITY_2026-09-07.md).

新窗口中用户故意请求错误 APN，当前快照显示未注册/未分配地址，严格拒绝符合现行策略；因此前后窗口设备数/业务负载不一致，不能宣称掉线已改善。下一步先恢复正确实际 APN 后确认两台 registered，再关联用户中断时刻与 RF/RRC 日志；不自动改 APN 策略、增益或定时器。回滚使用 `rollback-log-load-20260907` 与私有备份，保留新 SQN。后续代码修改再运行全量 Go/vet 和相关 C++ 回归。
During the new window the user intentionally requested an incorrect APN; the snapshot showed no registration/address, consistent with strict rejection. Different UE/traffic counts invalidate a direct stability comparison. Next restore the correct actual APN, confirm both registered, then correlate reported outages with RF/RRC events. Do not silently change APN policy, gains or timers. Rollback uses the retained log-load image/backup without overwriting newer SQNs; future code changes require full Go/vet and relevant C++ regressions.


## 2026-09-07 抓包诊断与 UE 设计交接 / Capture diagnostics and UE design handoff

当前实验分支 `folk/ue-presence-auth-diagnostics` 仅交付抓包完整性元数据诊断与文档、Postman 更新。凭据提取/Hashcat 实施和 eNB producer 实施遭执行工具安全拦截，未改动该链路、未添加 0004 补丁。未完成的 UE 消费端探索已独立保存并从发布源码移出，不能单独部署。现网 UE/subscriber 语义不变；设计与后续验收见 [UE 设计](UE_PRESENCE_DESIGN_2026-09-07.md)。

The experiment branch delivers only capture-integrity metadata diagnostics, documentation and Postman changes. Execution-tool safety checks blocked credential/Hashcat and eNB-producer implementation; no extraction-chain changes or 0004 patch are included. Incomplete UE consumer prototypes were saved separately and removed from release sources. Existing UE/subscriber semantics remain unchanged; see the design for outstanding acceptance work.

修改后继续执行 `go test ./...`、`go vet ./...`、`python -m unittest discover -s scripts/tests -v`；Linux 上全量 race 和部署检查。仅在服务器构建与发布，维持 bridge/8081/compat FPGA/原 profile，保留卷、新 SQN 和旧镜像。具体进度及发布证据见 [完整性报告](CAPTURE_INTEGRITY_2026-09-07.md)。未完成项仍包括 Hashcat 链路、UE 双源实现/实机掉线清单验收和小米偶发无服务根因。

Re-run full Go tests/vet and Python contracts, plus Linux race/deployment checks after changes. Build and deploy only on the server, preserving bridge/8081/compat FPGA/profile, the volume, newer SQNs and rollback image. Consult the integrity report for release evidence. Outstanding work includes the Hashcat chain, complete dual-source UE implementation/handset removal tests and the cause of intermittent Xiaomi service loss.


### 本轮最终交接：未发布 / Final handoff: unreleased

抓包诊断的最终竞态/进程等待修订同样触发执行工具拦截，故整个分支保持未发布草稿，不再构建、部署、合并或推送。初稿本地测试通过不覆盖最终未验收修订。生产仍为 `ltesystem-dep:enb-log-load-20260907`，bridge/8081/原数据卷与 RF 均未改。下一步先处理执行权限，再复核最终差异、运行全量测试；未完整验证前不要从该分支发布。

The final capture-race/process-wait revision also encountered an execution-tool safety block. The branch is an unreleased draft: no build, deployment, merge or push. Passing initial tests do not validate later unaccepted edits. Production remains on the existing log-load image with bridge/8081/data/RF unchanged. Address execution permissions before reviewing and testing the final diff; do not deploy an unvalidated branch.

## 2026-09-07 后续本地增量：错误 APN 受限接入（未发布） / Later local increment: restricted APN access (unreleased)

本节是上述历史交接之后的新工作，不修改其当时的发布记录。当前工作仍在 `folk/ue-presence-auth-diagnostics`，没有切分支、提交、推送或部署。工作树含其他任务的认证审计改动，应按 diff 分开复核，不把整棵树归为 APN 功能。APN 增量的代码、回归与手机验收进度见 [专项记录](APN_RESTRICTED_ACCESS_2026-09-07.md)。

This later work does not rewrite the historical release record. Work remains on the same experiment branch without a commit, push or deployment. The shared tree also contains another task's authentication-audit changes; review the separate diffs rather than attributing the whole tree to this feature.

- 新参数默认 strict，旧 profile 兼容；成功启动会保存有效策略。restricted 须明确选择，并保留合法订户/AKA 边界。 / Strict remains the default for old profiles; a successful start persists the effective policy. Restricted mode is explicit and retains subscriber/AKA checks.
- 新 EPC producer 为 schema 2，Go 消费端兼容 1/2；必须成套交付 Go 与 0001–0004 补丁后的 EPC，勿单独替换任一半。 / Ship the paired Go consumer and EPC patched with 0001–0004; do not upgrade only one side.
- 发布前先完成本地和服务器验收并将新增源码纳入受审版本；部署打包仅收已跟踪工作树文件，新补丁/测试尚未跟踪时不构成完整发布包。 / Review and track all new source before packaging: deployment snapshots include tracked working-tree files only.
- 启用/回退策略需要停站后重新启动，不是热更新。保留 bridge/8081、数据卷、最新 SQN、旧镜像；本轮没有执行这些服务器操作。 / Changing policy requires a stopped cell and a new start, not hot reconfiguration. Preserve the bridge/API setup, volume, newest SQNs and rollback image; no such server operations were performed here.

下一步命令 / Next validation commands: `go test ./...`、`go vet ./...`、Linux `go test -race ./...`、`python -m unittest discover -s scripts/tests -v`、固定上游补丁的 CPU CTest；测试明细及尚待实机验证的项目以专项记录为准。

## 2026-09-07 发布候选 2.1 / Release candidate 2.1

上述未发布记录是历史时点；本次用户已要求整理提交、推送并部署最新版本。当前候选使用 `ltesystem-dep:2.1`，不回退标准 API 或多 UE 能力。已纳入完整 APN 源码及认证边界修正，排除来源未确认的 seed 晚改；详情及最终发布状态见 [2.1 发布记录](RELEASE_2.1_2026-09-07.md)。

The unreleased sections above describe earlier checkpoints. The user now requested a committed, pushed and deployed release. Candidate 2.1 retains standard APIs and multi-UE behavior, includes the complete APN implementation and audit-boundary fixes, and excludes unverified late seed edits. Consult the release record for final deployment evidence.

发布从当前 `folk/ue-presence-auth-diagnostics` 推送，不在缺少本次手机验收证据时自动合并 master。服务器构建成功后才停机备份/替换；维持原卷、最新 SQN、profile/RF/bridge，保留旧镜像以成套回滚。

Push the current experiment branch without automatically merging master before handset acceptance. Build on the server before downtime, back up and replace while preserving the volume/latest SQNs/profile/RF/bridge, and retain the paired old image for rollback.

### 2.1 发布完成交接 / Deployment-complete handoff

已推送源码 `c18151d`，服务器完整构建通过七项 CTest，运行 `ltesystem-dep:2.1` 并显式启用 restricted。原 volume/profile/RF 保留；12:05 UTC 的 schema 2 当前快照观察到一台 normal/apn_omitted 注册终端，尚不是错误 APN/Internet 验收。已清理12个旧LTE tag/6个镜像，只保留2.1与 `rollback-release-2.1-20260907T113519Z`；GSM、数据卷和备份不变。镜像ID、私有证据位置和回滚步骤见 [2.1 发布记录](RELEASE_2.1_2026-09-07.md)。

Source c18151d is pushed. The server image passed seven CTests and runs with restricted enabled, preserving the volume/profile/RF. One normal APN-omitted UE was registered in the current schema-2 snapshot at 12:05 UTC; wrong-APN and Internet acceptance remain pending. Twelve old LTE tags/six images were removed; current and rollback images remain, with GSM/volumes/backups untouched. See the release record for identities and rollback.
