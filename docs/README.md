# docs — 文档索引 Docs Index

| 文档 Document | 内容 Description |
|---|---|
| [QUICKSTART.md](QUICKSTART.md) | **先看这个 Start here**：从部署到终端入网全流程（有已写卡/无写卡器场景） / Full flow from deploy to UE network entry (with/without SIM writer scenarios) |
| [RULES.md](RULES.md) | 使用规则：无状态/持久化/操作流/升级回滚 / Usage rules: statelessness/persistence/operation flow/upgrade and rollback |
| [API.md](API.md) | **v1 标准接口**完整参考（包络/状态码/错误码/多 UE 与订户管理） / Complete reference for the **v1 standard API** (envelope/status codes/error codes/multi-UE and subscriber management) |
| [api/openapi.yaml](api/openapi.yaml) | v1 的 OpenAPI 3.0 机器契约 / Machine-readable contract for v1 in OpenAPI 3.0 |
| [DEPLOY.md](DEPLOY.md) | 服务器部署、升级、备份、排障 / Server deploy, upgrade, backup, and troubleshooting |
| [WEB_UI.md](WEB_UI.md) | 中英管理台、可选 API 端口、鉴权与停止操作 / Bilingual console, optional API, authentication and shutdown |
| [WEB_UI_RELEASE_2026-09-09.md](WEB_UI_RELEASE_2026-09-09.md) | 2.1 管理台验收与按要求停机记录 / 2.1 console acceptance and requested shutdown |
| [RELEASING.md](RELEASING.md) | 版本、GitHub Release 与 Docker Hub 发布规划 / Versioning, GitHub Releases and Docker Hub publication plan |
| [../CHANGELOG.md](../CHANGELOG.md) | 中英变更记录 / Bilingual change history |
| [releases/TEMPLATE.md](releases/TEMPLATE.md) | 未来正式发布的双语记录模板 / Bilingual template for future formal releases |
| [RELEASE_2.1_2026-09-07.md](RELEASE_2.1_2026-09-07.md) | 2.1 已部署与清理：restricted 生效、正常路径注册证据、一个回滚与待手机验收 / Deployed 2.1, cleanup, enabled restricted policy, normal-path registration and pending handset acceptance |
| [SIM.md](SIM.md) | 写卡（有写卡器时）与 `user_db.csv` 行格式 / SIM writing (with writer) and `user_db.csv` row format |
| [SDR.md](SDR.md) | B210（正版/兼容板 FPGA 切换）与 bladeRF / B210 (genuine/compatible board FPGA switching) and bladeRF |
| [MIGRATION.md](MIGRATION.md) | 旧 Python 版 → Go 版对照 / Old Python vs Go version mapping |
| [CAPTURE_INTEGRITY_2026-09-07.md](CAPTURE_INTEGRITY_2026-09-07.md) | 活跃抓包截断的完整性诊断与交付边界 / Live-capture integrity diagnostics and delivery limits |
| [UE_PRESENCE_DESIGN_2026-09-07.md](UE_PRESENCE_DESIGN_2026-09-07.md) | 未发布的双源 UE 在线状态设计 / Unreleased dual-source UE presence design |
| [APN_RESTRICTED_ACCESS_2026-09-07.md](APN_RESTRICTED_ACCESS_2026-09-07.md) | APN 受限承载设计、CPU 回归与历史本地检查点；最终发布见 2.1 记录 / Restricted APN design, CPU regressions and historical local checkpoint; see 2.1 release evidence |
| [MULTI_UE_2026-09-07.md](MULTI_UE_2026-09-07.md) | 3.0 多终端、严格 APN、网络策略与发布验证 / 3.0 multi-UE, strict APN, network policy and release validation |
| [RADIO_STABILITY_2026-09-07.md](RADIO_STABILITY_2026-09-07.md) | 小米偶发无服务、日志减负、APN 拒绝语义与只读 Postman 验证 / Intermittent Xiaomi service loss, logging load, APN rejection semantics and read-only Postman validation |
| [UE_DIAGNOSTICS_2026-09-06.md](UE_DIAGNOSTICS_2026-09-06.md) | 手机入网、NAS 互通缺陷与射频 A–B–A 证据 / Handset registration, NAS interoperability defects and RF A–B–A evidence |
| [images/](images/) | 架构图（`ltesystem.png` 全量 / `ltesystem-easy.png` 简版）与写卡参数图 / Architecture diagrams and SIM-writing parameter figures |
| [samples/](samples/) | 真机样本：成功入网 EPC 日志、`atr-*.txt`（白卡 ATR） / Real-device samples: successful network-entry EPC logs, `atr-*.txt` (test-SIM ATR) |
| [legacy/](legacy/) | 早期中文文档（PDF/MD，仅存档，以本目录文档为准） / Early Chinese docs (PDF/MD, archived; this directory takes precedence) |

---
**导航 Navigation:** [文档索引 Docs](README.md) · [QUICKSTART](QUICKSTART.md) · [RULES](RULES.md) · [API v1](API.md) · [DEPLOY](DEPLOY.md) · [SIM](SIM.md) · [SDR](SDR.md) · [MIGRATION](MIGRATION.md)
