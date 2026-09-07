# 2.1 发布说明 / Release notes — 2026-09-07

> **候选已验证：本地 APN 与最新认证边界测试通过；待服务器构建与部署，手机验收未执行。**
> **Candidate validated: local APN and latest audit-boundary tests passed; server build/deployment and handset acceptance are pending.**

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

这是待执行的配置示例，不表示服务器已部署或已启用。注册、IP、承载 active 或 normal 授权决策都不是手机实际上网的证明。
This is a pending configuration example, not evidence of deployment or activation. Registration, an IP, an active bearer or a normal authorization decision does not prove handset Internet connectivity.

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

## 5. 验证证据与待办 / Evidence and pending validation

本地已完成的 APN 验证记录见 [专项记录](APN_RESTRICTED_ACCESS_2026-09-07.md)：
Completed local APN validation is recorded in the [feature report](APN_RESTRICTED_ACCESS_2026-09-07.md):

- Windows Go 1.26.2 全量 `go test -count=1 ./...`、`go vet ./...` 与 Linux/amd64 交叉编译通过。
- WSL Kali Go 1.26.5 `go test -race -count=2 ./...` 全量通过，含短命进程 reap 测试合成 wordlist fixture。
- 专项记录报告 C++ 7/7 CTest（每组重复三次）、Python/Postman 合约和真实 schema-2 producer→Go→OpenAPI 合成联动通过。

Windows Go tests/vet and Linux cross-build passed; Kali Go 1.26.5 passed two complete race runs, including the synthetic fixture for process reaping. The feature report also records seven C++ groups, repeated three times, and Python/Postman plus producer→Go→OpenAPI synthetic checks.

上述证据来自本地源码与 CPU 测试，不是已构建的 `ltesystem-dep:2.1` 服务器镜像结果。服务器构建、运行镜像冒烟以及手机验收仍待执行；认证审计修订已完成上述复验。忽略缓存日志不应打包到发布镜像或提交为私有运行数据。

These results describe local source/CPU validation, not a built server image. Server build, runtime-image smoke tests and handset acceptance remain pending, with revalidation required for the latest audit fixes. Ignored cache logs are not release payload or committed private runtime data.

手机待验收：strict 错误 APN 拒绝；restricted 错误 APN 注册后双向零数据（DNS/LAN/host/peer）；另一台正常 UE 不受影响；idle/恢复/重连仍限制；改回正确 APN 并重新附着后正常联网；分别记录实际信号图标和断线表现。

Pending handset checks: strict mismatch rejection; restricted registration with zero traffic in both directions across DNS/LAN/host/peer boundaries; an unaffected normal UE; restriction across idle/resume/reconnection; normal access after a correct-APN reattach; and observed signal indicators/service interruptions.

## 6. 待执行发布与一个回滚目标 / Pending release and one rollback target

1. 主会话复核全部候选差异、认证阻断修复和新源码清单后提交。打包规则只包含已跟踪文件，需确保新补丁和测试已纳入完整发布版本；本说明不执行 Git 提交。
2. 按 [DEPLOY](DEPLOY.md) 在服务器独立源码快照构建 `ltesystem-dep:2.1`，记录版本、源码修订、镜像 ID 和构建测试结果，再进行备份/替换；当前状态不得标记为已部署。
3. 保留实际 Compose 项目名及 `/data` 对应的原卷、最新订户数据库与 SQN、原 profile/DNS/上行、RF/频点/增益/FPGA、bridge/eth0/8081 与私有鉴权配置。识别实际卷名，不新建空卷替代原数据。
4. 部署仅替换配套 API/EPC 镜像；停站与射频启动分别按发布流程执行。备份和只读冒烟通过后，再单独安排 restricted 启用与手机验收。

The main session reviews and commits the full candidate, including new tracked sources and audit fixes. Build image 2.1 from a separate server snapshot, record source/image identity and build results, then back up and replace. Preserve the actual Compose project and data volume, current subscribers/SQNs, profile/DNS/uplink, RF settings/FPGA and bridge/eth0/API configuration. Container replacement and RF activation are separate steps; enable restricted and perform handset acceptance only through the planned release procedure.

**唯一指定回滚目标：`ltesystem-dep:enb-log-load-20260907`。** 它是本次任务给定的服务器旧镜像，实际替换前应记录并保留其镜像 ID。失败时按 DEPLOY 的回滚流程成套恢复旧 Go/EPC 镜像和原 bridge 编排，复用同一数据卷；必要时仅恢复发布前 profile 配置，不用旧备份覆盖最新卡库/SQN。旧版本回到严格 APN 行为，受限功能不可期待保留。

**The single designated rollback target is ltesystem-dep:enb-log-load-20260907**, the server's previous image supplied for this task. Record and retain its image ID before replacement. On failure, restore the paired old Go/EPC image and original bridge orchestration using the same volume. Restore only the earlier profile configuration if needed, without overwriting current subscribers/SQNs from an old backup. Expect the old strict APN behavior, not restricted access.

主会话已将 seed 的临时晚改备份到本地忽略缓存并恢复 HEAD 模板；发布使用合成模板，不提交真实身份或凭据，不改服务器现有订户库。本说明不包含真实服务器地址、IMSI 或密钥。UE 双源功能依然以 [设计文档](UE_PRESENCE_DESIGN_2026-09-07.md) 为准，未计入已实现功能。

The main session backed up late seed edits in ignored local cache and restored the HEAD template. Release the synthetic template only; do not publish real identities/credentials or change the server's existing subscriber database. This note contains no real server address, IMSI or key. Dual-source UE presence remains [design-only](UE_PRESENCE_DESIGN_2026-09-07.md).
