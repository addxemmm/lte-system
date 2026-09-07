# 2.1 发布说明 / Release notes — 2026-09-07

> **已发布并启动：2026-09-07 12:02 UTC（香港时间 20:02）启动 2.1；12:05 UTC 复核正常。错误 APN 的手机行为验收仍待进行。**
> **Deployed and started at 12:02 UTC (20:02 Hong Kong), rechecked at 12:05 UTC on 2026-09-07. Handset acceptance of incorrect-APN behavior remains pending.**

## 1. 发布编号与范围 / Release number and scope

目标镜像为 `ltesystem-dep:2.1`。用户要求按 2.0→2.1 编排本次镜像编号，因此 `VERSION` 与 host/bridge Compose 的默认镜像同步为 2.1，`LTE_IMAGE` 仍可显式覆盖。

The target image is `ltesystem-dep:2.1`. The user selected 2.1 as the next image number after 2.0; VERSION and both Compose image defaults use this number, retaining the explicit LTE_IMAGE override.

**2.1 是用户指定的镜像发布号，不是从历史 3.0 降级，也不是恢复 2.0 旧 API。** 当前标准 `/api/v1`、多 UE 会话、订户管理、网络规划及隔离能力全部保留。根路径旧接口和单条 `/api/v1/ue` 仍已移除；镜像号、API 命名空间和遥测 schema 版本分别管理。CHANGELOG 的 3.0 及既往发布记录保持原样。

**2.1 is a user-selected image release number, not a downgrade from historical 3.0 or restoration of the old 2.0 API.** Retain the standard API, multi-UE sessions, subscriber management, network planning and isolation. Removed root routes and singleton UE lookup stay removed. Image numbering, API namespace and telemetry schema are separate; historical 3.0 records are preserved.

本次候选还包含工作树中既有的抓包完整性和 PAP/CHAP 认证审计增量，应独立复核，不把这些变化全部归因于 APN restricted。UE 双源在线状态仍是设计、尚未实现；当前 EPC 会话快照不等于 EPC/eNB 双源融合。小米偶发无服务及手机信号图标行为尚无本次实机验证结论。

The candidate also includes existing capture-integrity and PAP/CHAP audit changes, which require separate review from restricted APN access. Dual-source UE presence remains a design, not an implementation: current EPC snapshots are not fused EPC/eNB evidence. This release has no new handset evidence resolving intermittent service loss or signal-icon behavior.

## 2. APN 策略 / APN policy

- `strict` 是默认值，旧 profile 缺字段时仍为 strict，合法 SIM 的显式错误 APN 沿用拒绝行为。
- `restricted` 必须明确选择；只对通过现有订户/SIM/AKA 检查、请求格式合法但与配置不匹配的显式 APN 提供受限默认承载，使 UE 可完成注册。SPGW 双向丢弃其用户面，包括 DNS、LAN、宿主/EPC 服务和 UE 间流量；`ue_access=allow` 不解除受限策略。
- 正确 APN 和省略 APN 保留原处理。异常 APN 或认证失败不因此获得接入。
- 成功启动保存生效策略；空请求继承已保存 profile，显式 strict 覆盖保存的 restricted。只有真正空字符串使用默认值，纯空白或其他非法枚举报字段错误。
- Go 将生效策略显式传给 EPC 的 `LTE_APN_MISMATCH_POLICY`，父环境不覆盖请求。部署新镜像本身不把现有 profile 改成 restricted；策略切换需要停站后重新启动。

Strict remains the default, including profiles without the new field. Restricted is opt-in and applies only to syntactically valid explicit mismatches after existing subscriber/SIM/AKA checks. It permits a restricted default bearer and registration while SPGW drops both directions of user-plane traffic, including DNS, LAN, local services and peers. Correct/omitted APNs retain their handling. Successful startup persists the policy; an empty request inherits the profile, explicit strict overrides restricted, and whitespace or invalid enums fail validation. The Manager pins the EPC environment value. Image replacement alone does not enable restricted; a policy change requires stopping and starting the cell.

配套镜像部署完成、停站且已有有效 profile 后，启用请求的策略字段为：
After paired deployment, with the cell stopped and a valid saved profile, the policy field for POST /api/v1/cell is:

```json
{"apn_mismatch_policy":"restricted"}
```

本次已执行此显式请求并验证保存的 profile 和运行状态均为 restricted，其他原有 profile 参数保持一致。注册、IP、承载 active 或 normal 授权决策都不是手机实际上网的证明。
This explicit request was executed; both saved profile and running state report restricted, with all other original profile parameters preserved. Registration, an IP, an active bearer or a normal authorization decision does not prove handset Internet connectivity.

## 3. schema 2 必须配套 / Paired schema-2 delivery

新 EPC producer 统一输出 `schema_version=2`，配套 Go 消费端读取 schema 1/2；schema 1 维持原语义且拒绝新访问字段。发布应包含 Go 与固定上游应用 0001–0004 的 EPC，不单独替换 producer 或 consumer。

The new EPC emits schema 2; its paired Go consumer reads schemas 1 and 2. Schema 1 retains its original behavior and rejects new access fields. Ship Go and the pinned EPC with patches 0001–0004 as one release.

| 策略 Policy | 原因 Reason | APN 状态 / APN state |
|---|---|---|
| normal | apn_match / apn_omitted | apn_validated=true，selected 为实际配置 APN / selected is the configured APN |
| restricted | apn_mismatch | apn_validated=false，requested 与非空 selected 合法且不匹配 / valid explicit mismatch with nonempty selection |
| deny | session_unavailable | apn_validated=false，selected 为空或省略 / selection empty or omitted |

normal/restricted 仅在 SPGW 策略安装确认后发布；此前、失败或清理状态为 deny。未 registered 但已确认安装的会话可为 normal/restricted。v2 必需访问字段和布尔值缺失/null、出现的字符串为 null（含大小写变体）、未知或矛盾的策略均拒绝；保持原有 run/PID/时间/唯一性检查。消费端表达授权状态，不推断实时可达性。

Publish normal/restricted only after SPGW installation confirmation; before confirmation, on failure or after clearing, publish deny. A confirmed session may have an authorization decision before registration. Reject missing/null required v2 policy/boolean fields, null strings including case variants, unknown or contradictory policy values, while retaining run/PID/time/uniqueness checks. Authorization does not imply current reachability.

## 4. 既有认证审计差异与阻断项 / Existing audit changes and release blockers

既有增量包括 PAP 明文识别、CHAP 配对/字典作业、完整抓包前缀快照、字典前置检查和私有作业日志。PAP 与 CHAP 的结果属于 APN 认证证据，不是 SIM/AKA 放行条件，也不决定 restricted 数据权限。聚合 EPC 日志富化无法建立可靠的逐 UE 凭据归属。

Existing increments cover PAP recognition, CHAP pairing/dictionary jobs, immutable complete-record capture prefixes, wordlist preflight and private job logs. APN authentication evidence is separate from SIM/AKA admission and restricted user-plane authorization. Aggregate EPC enrichment provides no reliable per-UE credential binding.

发布前已修复并测试两项认证审计阻断。以下是已验证的接口边界，不声明已完成按 IMSI 审计：
Two audit blockers were fixed and tested before release. The following interface boundaries are validated; per-IMSI auditing is not implemented:

1. 显式 IMSI 请求即使指向已知订户，也因缺少可靠的逐 UE credential 绑定而返回 HTTP 412；不泄露凭据、不停止小区。
2. 读取订户数据库失败按服务错误报告，不能伪装成订户不存在或 ownership 验证成功。
3. 未指定 IMSI 的 legacy 审计仍可运行，但 `ownership_verified=false`。其中 CHAP legacy 作业保留原有停站副作用，不把显式 IMSI 的无副作用边界套用到该路径。

Explicit IMSI requests, including known subscribers, return HTTP 412 because reliable per-UE credential binding is unavailable; they disclose no credentials and do not stop the cell. Subscriber-database read failures are service errors, not missing-subscriber or verified-ownership results. Legacy auditing without an IMSI may still run with ownership_verified=false; its CHAP job retains the existing cell-stop side effect.

最新修订已通过 Windows 全量 Go/vet、API/crack 定向回归、Python/Postman 契约；Linux 全量 Go 与定向 vet 同样通过。没有指定 IMSI 也不代表凭据已绑定到某台手机；字典未命中不证明口令强度。

The final audit edits passed Windows full Go tests/vet, targeted API/crack tests, Python/Postman contracts, Linux full Go tests and targeted vet. Omitting an IMSI does not establish handset attribution, and a dictionary miss does not prove password strength.

## 5. 最终验证证据 / Final validation evidence

- Windows：最新源码全量 `go test -count=1 ./...`、`go vet ./...` 通过；Python/Postman/部署契约 14 通过、1 项 POSIX-only 跳过。
- WSL Kali Go 1.26.5：最终 `go test -race -count=1 ./...`、全量 `go vet ./...`、Python 15/15 全通过；独立收尾脚本 `ALL_VALIDATION_EXIT=0`。此前收尾有语法错误的脚本已被这次完整成功运行替代，不把它的外层 exit=2 当成功。
- APN 专项先前已完成固定上游 CPU CTest 7/7（每项重复三次）、schema-2 producer→Go→OpenAPI 合成联动；详见 [专项记录](APN_RESTRICTED_ACCESS_2026-09-07.md)。
- **服务器 Ubuntu 22.04 完整 Docker 构建 exit=0**：应用 0001–0004，七项 CTest 全通过（2.04 秒）：`nas_pdn_reject_test`、`ipv4v6_fallback_test`、`strict_apn_test`、`spgw_session_safety_test`、`ue_snapshot_test`、`restricted_apn_access_test`、`restricted_mme_lifecycle_test`。
- 新容器只读 smoke 通过；EPC/eNB/pcap 均运行，schema 2 快照 `current`。2026-09-07 12:05:35 UTC 观察到 **1 个 registered、normal/apn_omitted 会话**。这是正常默认 APN 路径的现场注册证据，不是错误 APN 或手机 Internet 验收。

Final Windows full Go tests/vet passed; Python passed 14 with one POSIX-only skip. The final Kali run passed full Go race/vet and all 15 Python tests with a clean wrapper exit. Server Ubuntu 22.04 completed the entire image build and all seven pinned-core CTests. Read-only smoke passed and the paired EPC/eNB produced a current schema-2 snapshot. At 12:05:35 UTC one normal, registered APN-omitted session was observed; this does not prove incorrect-APN behavior or handset Internet reachability.

仍待手机验收：显式错误 APN 注册与信号图标、该 UE 双向业务阻断、正常 UE 不受影响、idle/resume/重连保持限制、改回正确 APN 后恢复业务。没有新增小米偶发无服务根因或稳定性结论；UE 双源清单仍未实现。

Handset acceptance remains pending for explicit wrong-APN registration/icons, blocked data in both directions, unaffected normal UEs, idle/resume/reconnection, and restored service after a correct-APN reattach. No new claim resolves intermittent Xiaomi service loss, proves stability or implements dual-source UE presence.

## 6. 已执行发布 / Executed release

| 项目 Item | 记录 Record |
|---|---|
| GitHub | `addxemmm/lte-system`，分支 `folk/ue-presence-auth-diagnostics` |
| 构建源码 / Built source | `c18151d45ea3dce853739657a0c741a8beefe91c` |
| 镜像 / Image | `ltesystem-dep:2.1` |
| 镜像 ID / Image ID | `sha256:db64d4b16ed38bf30f04dae16c428234fde80489a381f92c042aa02981a30873` |
| 源码快照 / Source snapshot | `/home/addx/lte-releases/release-2.1-20260907T113519Z` |
| 源码包 SHA-256 / Source archive | `664210a75b4a8afdea5eb70bba6cdfe4d81095b87664e2c1cfaf7f32871ea161` |
| 保留数据卷 / Preserved volume | `docker_lte-data`，Compose project `docker` |
| 网络 / Networking | 原 bridge、eth0、API 8081；原私有鉴权状态未修改 / original bridge, eth0, API port and authentication state retained |
| 私有备份 / Private backup | `/home/addx/lte-backups/release-2.1-20260907T113519Z` |
| 数据备份 SHA-256 / Backup archive | `5361cf411da6962b5fec9c85d2fc0eb9c7acffbfc9196031e36bd6bc5b55f47f` |

使用本地 GitHub CLI 已保存的 **addxemmm** 凭据完成推送；同机另一个活跃账号并发变化，因此本次 Git 进程临时使用指定账号凭据，不输出、不落盘令牌。161 个打包文件按仓库属性核对为该提交的 Git 对象，二进制资产保留；上传后再次核验压缩包 SHA-256。新补丁和所有新测试均已纳入提交。后续发布记录提交仅补文档，不改变镜像源码标签。

The push used the locally saved addxemmm GitHub CLI credential, scoped to that process because another task changed the active account. No token was printed or written. All 161 packaged files matched the committed Git objects under repository attributes, including preserved binaries; the uploaded archive hash was verified again. All new source/tests were tracked. Subsequent evidence-only documentation commits do not alter the image's source revision label.

先构建，后停站并备份实际卷，再禁止拉取镜像地重建容器，核对镜像 ID、卷名、原 profile 和订户库字节哈希。启动 EPC 前验证卡库与停机备份一致，因此未用样例库或旧备份覆盖当前 SQN。随后只指定 restricted 启动，原频段、功率、DNS、上行、FPGA 和其他 profile 参数未改。没有写卡或改订户认证参数。

Build preceded downtime. The stopped actual volume was backed up, the container recreated without pulling, and image identity, volume, original profile and subscriber bytes were checked before EPC restart. No seed or stale backup overwrote current SQNs. Only the restricted policy was selected at startup; existing RF, DNS, uplink, FPGA and all other profile parameters were preserved. No SIM programming or subscriber-authentication mutation was performed.

## 7. 清理与回滚 / Cleanup and rollback

- 已移除 **12 个旧 LTE 标签、6 个旧 LTE 镜像 ID**。替换操作已移除旧 ltesystem 容器；没有额外残留的已停止 LTE 容器可清理。
- LTE 只保留 `ltesystem-dep:2.1` 及 **`ltesystem-dep:rollback-release-2.1-20260907T113519Z`**。
- 回滚镜像 ID 为 `sha256:3000e5584d63e21b751f2893afc34873d23a17b1f12a01c84bde792103e765ac`，即原 `enb-log-load-20260907` 镜像；旧别名已清理，应使用新的回滚标签。
- GSM 容器/镜像、全部持久卷、备份、共享基础镜像和构建缓存均保留。未执行全局 prune、强制删除或 volume 删除。
- 清理前和每次删除前均检查运行中 2.1、原回滚镜像和 restricted 状态；脱敏结果/清单保存在上述私有备份目录。

Removed 12 obsolete LTE tags and six obsolete image IDs. Container replacement removed the prior LTE instance; no additional stopped LTE containers remained. Keep only the current and one rollback LTE tag. The rollback ID is the former log-load image; its old alias was removed. GSM resources, all volumes/backups, shared bases and build caches were untouched. No global prune, forced removal or volume deletion was used; health and rollback checks preceded each tag removal.

需要回滚时按 [DEPLOY](DEPLOY.md) 设置上述回滚镜像，恢复原 bridge 编排并使用同一卷；必要时仅恢复备份 profile，**不回灌旧卡库/SQN**。恢复旧 API 后单独按原 profile 启动小区并验证。旧镜像恢复严格 APN 行为，不保留本次 restricted 能力。

Follow DEPLOY with the retained rollback tag, original bridge orchestration and same volume. Restore only the original profile if necessary; never rewind subscriber/SQN data. Start and verify the old cell separately after API recovery. The old image restores strict APN behavior, not restricted access.

来源未确认的 seed 晚改已留在本地忽略缓存、从提交排除；服务器真实卡库保持。该文件不包含真实 IMSI、密钥或令牌，原始运行数据和备份只留在服务器。当前分支已推送，尚未自动合并 master，待本次手机验收完成后另行决定。

Unverified late seed edits remain in ignored local cache and were excluded from Git; the server database was preserved. No real IMSI, key or token appears here; raw runtime evidence/backups remain on the server. The experiment branch is pushed, not automatically merged into master before handset acceptance.
