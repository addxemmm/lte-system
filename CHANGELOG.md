# 更新日志 Changelog

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
