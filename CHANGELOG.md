# 更新日志 Changelog

## [2.1] - 2026-09-07（准备中 / Preparing）

- 本次使用用户指定镜像编号 `ltesystem-dep:2.1`（按 2.0→2.1 编号），同步 `VERSION` 和两种 Compose 的镜像默认值。这不是从历史 3.0 降级：保留标准 `/api/v1`、多 UE、订户和网络管理，旧根路径接口与单条 `/api/v1/ue` 不恢复；下方 3.0 历史记录保留。 / Use the user-selected image number 2.1, following their 2.0→2.1 numbering, in VERSION and both Compose defaults. This is not a downgrade from historical 3.0: retain the standard API and multi-UE/subscriber/network functionality without restoring removed legacy routes or rewriting history.
- APN restricted 显式启用，strict 默认；配套交付 schema-2 EPC producer 与兼容 schema 1/2 的 Go 消费端。SPGW 确认后才发布 normal/restricted，受限用户面双向丢弃。 / Restricted APN access is opt-in, with strict as the default. Ship the schema-2 EPC producer together with the schema-1/2 Go consumer; publish normal/restricted only after SPGW confirmation and drop restricted traffic in both directions.
- 本次候选包含工作树中既有的 PAP/CHAP 弱口令审计、抓包完整性和作业前置校验差异，独立于 APN 接入策略；UE 双源在线状态仍仅有设计、未实现。 / The candidate also includes existing PAP/CHAP audit, capture-integrity and job-preflight changes, separate from APN access policy. Dual-source UE presence remains design-only and unimplemented.
- 状态为准备中：本地 APN Go/vet、Linux race、交叉编译及专项 CPU/契约测试已通过；最新认证审计修订需复验，服务器镜像构建、部署和手机验收待执行。保留原卷、最新 SQN、RF、bridge/8081 与旧镜像 `ltesystem-dep:enb-log-load-20260907` 作为成套回滚目标。 / Preparing: local APN Go/vet, Linux race, cross-build and feature CPU/contract tests passed; latest audit fixes require revalidation, and server-image build, deployment and handset acceptance are pending. Preserve the volume, newest SQNs, RF and bridge/API setup, with the previous image as the paired rollback target.
- 认证审计发布边界：显式 IMSI 请求因缺少可靠逐 UE 凭据绑定返回 412，不泄凭据、不停小区；订户数据库读取失败为服务错误。无 IMSI 的 legacy 审计保留，ownership_verified=false。两项修复由独立任务实施，发布前复验；不宣称支持按 IMSI 审计。 / Audit release boundaries: explicit IMSI requests return 412 without credentials or cell-stop effects because per-UE binding is unavailable; database read failures are service errors. No-IMSI legacy auditing remains with ownership_verified=false. Independently implemented fixes require revalidation before release; per-IMSI auditing is not claimed.
- 详细范围、测试边界与待执行发布/回滚步骤见 [2.1 发布说明](docs/RELEASE_2.1_2026-09-07.md)。以下较早的未发布条目保留为历史上下文。 / See the [2.1 release notes](docs/RELEASE_2.1_2026-09-07.md) for scope, evidence limits and pending release/rollback steps. Earlier unreleased entries below remain as historical context.

## 未发布：2026-09-07 错误 APN 受限接入 / Unreleased restricted APN access

- 新增 `apn_mismatch_policy=strict|restricted`：默认保持严格拒绝，明确启用 restricted 后，合法订户的格式合法错误 APN 可获得受限默认承载；保留 SIM/AKA，并在 SPGW 阻断双向用户面。 / Add an explicit restricted default-bearer mode while keeping strict rejection as the default, preserving subscriber/AKA checks and dropping restricted user-plane traffic in both directions.
- 普通/受限动态地址分类、会话代次校验、失败回收与有界删除重试；正常 idle 寻呼保留多包队列。 / Separate normal/restricted address classes, validate session generations, reclaim failed sessions with bounded retries, and retain normal multi-packet paging queues.
- EPC 快照升级 schema 2，Go 同时读取旧 schema 1；公开访问策略但不把注册当作 Internet 可用性。文档、OpenAPI、Postman 和合成回归同步。 / Emit schema 2 while retaining schema-1 reads, exposing access policy without claiming Internet reachability; synchronize API documentation and synthetic contracts.
- 未构建部署镜像、未启用服务器或修改 RF/SIM。最终测试与手机验收边界见 [专项记录](docs/APN_RESTRICTED_ACCESS_2026-09-07.md)。 / No image deployment, server activation or RF/SIM modification; see the feature record for validation and handset acceptance boundaries.

## 未发布草稿：2026-09-07 抓包完整性诊断 / Unreleased capture-integrity draft

- 区分活跃 S1AP 抓包的未完成尾 record、坏格式、错误 DLT、工具不兼容与读取失败；只检查有界私有完整前缀，源文件不变。 / Distinguish incomplete live records, invalid format/DLT, incompatible decoder and read failures using a private bounded complete-record prefix without modifying the source.
- 不完整扫描不再被当作无 CHAP 证据；响应只含协议元数据，更新标准文档、OpenAPI、Postman 断言与合成回归。 / Incomplete scans no longer imply absent CHAP; return protocol metadata only, with updated docs, OpenAPI, Postman assertions and synthetic regressions.
- UE 双源清单仅交付设计，认证提取/Hashcat 未修改，现行 UE/subscriber 契约保持。 / Dual-source UE presence remains design-only; authentication extraction/Hashcat and deployed UE/subscriber semantics are unchanged.

## [3.0] - 2026-09-07

- 标准接口收敛：删除根路径旧接口和单条 `/api/v1/ue`；提供 `/api/v1/ues`、按 IMSI 查询、订户管理与网络规划；同步 OpenAPI、Postman 和迁移文档。 / Standard-only API: remove legacy root routes and singleton UE lookup; add UE collections, IMSI lookup, subscriber management and network planning, with updated OpenAPI, Postman and migration docs.
- 多终端状态来自 EPC 当前上下文原子快照，绑定进程和启动代次；区分缺失、过期、停止、注册与无线连接状态，不再拼接不同终端的历史日志。 / Current EPC snapshots are atomic and tied to process/start generation; distinguish missing, stale, stopped, registered and radio-connected states instead of combining historical logs from different devices.
- 严格校验实际 APN，显式错误 APN 不再回落放行；省略 APN 使用配置的默认值，匹配不区分大小写。 / Enforce the actual APN: explicitly incorrect values no longer silently fall back; omission selects the configured default and matching is case-insensitive.
- 修复动态 IP 回收、地址池耗尽处理和 GTP 会话关联；保持 ECM idle 地址并验证上行 TEID 与源 IPv4 的绑定。 / Fix dynamic IP reclamation, exhausted pools and GTP session correlation; retain addresses during ECM idle and validate uplink TEID/source-IPv4 bindings.
- 可配置私网 `/24` UE 地址池和终端间隔离/互访；规则限定当前网络命名空间并由启动生命周期管理。保留 bridge 和仅发布 API 端口。 / Configure a private /24 UE pool and peer isolation/access, with namespace-scoped lifecycle-owned rules. Keep bridge networking and API-only publishing.
- 订户与写卡变更必须停站，原子卡库写入保留已有 SQN，响应不返回密钥；Postman 默认空启动请求继承已验证的 DNS 和上行配置。 / Subscriber and SIM mutations require a stopped cell; atomic database updates preserve existing SQNs and redact secrets. Postman defaults to empty startup requests that retain verified DNS/uplink settings.
- 本次不引入多 APN、多 VLAN 或按组路由；编译容量不是 BlackSDR/虚拟机现场并发性能保证。详见 [多终端迁移与验证](docs/MULTI_UE_2026-09-07.md)。 / This release does not introduce multiple APNs, VLANs or group routing; compiled capacity is not a BlackSDR/VM concurrency guarantee. See the [multi-UE migration and validation report](docs/MULTI_UE_2026-09-07.md).

## 2026-09-06 局域网访问修正 / LAN access correction

- 按用户要求将 Compose 默认监听恢复为 `0.0.0.0:8081`，可直接通过服务器 IP 访问；保留现有鉴权配置及 `LTE_LISTEN` 覆盖能力。 / Restore the requested `0.0.0.0:8081` Compose default for direct server-IP access; preserve existing authentication and the `LTE_LISTEN` override.

## 2026-09-06 审查加固 / Audit hardening

- 修复子进程竞态、误杀、资源泄漏和启动取消回滚。 / Fix child-process races, foreign-process termination, resource leaks and startup cancellation.
- 串行化 SIM/订户写入，拒绝认证冲突，修复 CSV 边界。 / Serialize SIM/subscriber writes, reject credential conflicts and fix CSV boundaries.
- 严格 JSON、原子并发上传、统一 413、完整请求审计。 / Strict JSON, atomic uploads, consistent 413 responses and complete request audits.
- 独立部署快照、私有数据排除、回环默认值、可回滚镜像及受保护数据卷。 / Fresh deployment snapshots, private-data exclusions, loopback defaults, rollback images and preserved volumes.
- Windows/Linux race、mock 和服务器隔离 HTTP 验证通过；真实 RF/写卡未执行。 / Windows/Linux race, mock and isolated server HTTP validation passed; physical RF/SIM operations were not exercised.
- 详见 [审查与发布记录 / audit and release report](docs/AUDIT_2026-09-06.md)。


本项目的所有重要变更都记录在这里。版本规则：
All notable changes to this project are documented here. Versioning:
`VERSION` file + `ltesystem-dep:<VERSION>` image tag.

## [2.0] - 2026-09-05

在 Ubuntu SDR 主机上完整重建，已端到端验证（CPE 入网、鉴权、IP、NAT、DNS、抓包packet capture）。
Full rebuild on the Ubuntu SDR host, validated end-to-end (CPE attach, auth, IP, NAT, DNS, traffic capture).

- Go + srsRAN_4G (`release_23_11`)：9 个工具 API 行为保留（`message_id` 语义），新增 `GET /healthz`、`/status`、`/profile` / Go + srsRAN_4G (`release_23_11`): 9 tool API behaviors retained (`message_id` semantics), plus `GET /healthz`, `/status`, `/profile`
- 灵活的 `/writesim`（所有卡参数可选）和 `/start`（`sdr/device_args/gains/n_prb/net names/dns`、存档saved profile继承、经由 `/data/last_start.json` 空包体重用上次配置） / Flexible `/writesim` (all card params optional) and `/start` (`sdr/device_args/gains/n_prb/net names/dns`, saved profile inheritance, empty-body reuse of last config via `/data/last_start.json`)
- srsRAN_4G 兼容性修复（现场发现）：`drb.conf`→`rb.conf`、上游 `rr.conf` 格式、每频段显式 `ul_earfcn`（上游 TDD 推导损坏） / srsRAN_4G compat fixes found live: `drb.conf`→`rb.conf`, upstream `rr.conf` format, explicit per-band `ul_earfcn` (TDD derivation broken upstream)
- 虚拟机 USB 调优：B210 自动 `device_args`、默认 5MHz（`n_prb 25`）、防僵尸进程跟踪 / VM-USB tuning: B210 auto `device_args`, default 5MHz (`n_prb 25`), zombie-proof process tracking
- Docker 多段镜像（1.53GB，原 3.46GB）、FPGA stock/compat 切换、原子化配置种子seed/seeding、DOCKER-USER 转发 + MSS clamp 自动化 / Docker multi-stage image (1.53GB, was 3.46GB), FPGA stock/compat switching, atomic config seeding, DOCKER-USER forwarding + MSS clamp automation
- 文档：QUICKSTART/RULES/API 参考/SIM/SDR/MIGRATION + 金牌 EPC 样本 / Docs: QUICKSTART/RULES/API reference/SIM/SDR/MIGRATION + golden EPC sample

## [1.x] - 2023 (legacy, EOL)

v1.x Python + srsLTE，手工容器，`legacy-python-workspace/` 存档saved profile。见 [`docs/legacy/`](docs/legacy)。不再维护；无安全修复。
v1.x Python + srsLTE, manual container (tree removed, see git tag `archive/v1-python`). See [`docs/legacy/`](docs/legacy). Not maintained; no security fixes.
