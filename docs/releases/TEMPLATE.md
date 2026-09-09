# <VERSION> 发布记录 / Release notes

> 模板，非已发布版本。发布前替换占位符并删除不适用项；未执行的检查明确标注，不以计划代替证据。
> Template, not a published release. Replace placeholders and remove inapplicable items; label unperformed checks rather than presenting plans as evidence.

## 中文

填写本版本的中文摘要、主要变化、兼容性和已知限制。保留本节标题，发布校验要求中文与英文两节同时存在。

## English

Write the English summary, main changes, compatibility and known limitations here. Keep both language headings: release validation requires both sections.

## 状态与标识 / Status and identity

| 字段 / Field | 值 / Value |
|---|---|
| 状态 / Status | Draft / Prerelease / Published |
| 日期 / Date | YYYY-MM-DD HH:MM:SS UTC |
| `VERSION` | <VERSION> |
| Git tag | v<VERSION> |
| 完整 commit / Full commit | <SHA> |
| CI/build evidence | <URL> |
| 镜像仓库 / Image repository | namespace/lte-system (Public; verify intended visibility) |
| Docker Hub 唯一版本标签 / Single version tag | <VERSION> (no SHA/latest alias) |
| 分发 digest / Distribution digest | sha256:<DIGEST>, verified remotely |
| 平台 / Platform | linux/amd64 (only if validated) |
| 源码/材料/校验和 / Source, materials, checksums | <ATTACHMENTS> |
| SBOM/provenance | <ATTACHMENTS OR NOT GENERATED> |

## 变化 / Changes

- 新增 / Added: ...
- 修复 / Fixed: ...
- 部署默认值 / Deployment defaults: ...
- 破坏性变化 / Breaking changes: ...

## 升级与配置 / Upgrade and configuration

中文：适用旧版本、端口暴露、Token 配置、环境变量、数据格式变化和明确升级步骤。

English: supported previous versions, port exposure, token configuration, environment variables, data changes and explicit upgrade steps.

## 验证 / Validation

| 检查 / Check | 结果与证据 / Result and evidence |
|---|---|
| Go tests / vet / race | <PASS/FAIL/NOT RUN + URL> |
| Python / frontend JS / shell | <RESULT + URL> |
| 完整镜像构建与七项 CTest / Full build and seven CTests | <RESULT + URL> |
| API 默认关闭、双口、鉴权 / Listener and authentication checks | <RESULT + URL> |
| UI 语言、主题、移动、键盘 / Language, theme, mobile, keyboard | <RESULT + URL> |
| 数据完整性 / Data integrity | <RESULT + REDACTED EVIDENCE> |
| 漏洞、秘密、许可与对应源码 / Vulnerabilities, secrets, licenses and source | <REVIEW RESULT> |
| RF、手机入网、Internet / RF, handset, Internet | NOT RUN unless separately evidenced |

## 已知限制 / Known limitations

中文：列出未验证平台、硬件和场景；说明外部依赖及残余问题。

English: list unverified platforms, hardware/scenarios, external dependencies and remaining issues.

## 部署、恢复与保留 / Deployment, recovery and retention

中文：Release 发布不代表服务器已部署或小区已启动。记录单独部署授权、实际运行 digest、数据卷/备份检查及恢复兼容性。遵从当前本地镜像保留要求，不自动下载旧镜像或删除卷。

English: publishing a Release does not mean the server is deployed or the cell is running. Record separate deployment approval, actual running digest, verified data/backups and recovery compatibility. Respect local retention policy without automatically pulling old images or deleting volumes.

## 发布检查 / Publication checklist

- [ ] VERSION、Git tag、UI 和 OCI 标签一致 / Version identities agree.
- [ ] 精确 commit 已审核，所有必需门禁通过 / Exact commit reviewed; required gates pass.
- [ ] 预期仓库可见性和接收者源码访问已确认 / Intended visibility and recipient source access confirmed.
- [ ] 远端 digest 与发布附件校验和已核实 / Remote digest and attachment checksums verified.
- [ ] 中英 notes、源码/构建材料、SBOM/provenance 状态完整 / Notes, source/materials and attestation status recorded.
- [ ] 附件准备完成后才完成 Release / Finish the Release only after preparing all attachments.
- [ ] 不改写历史版本，不自动部署或启动小区 / No overwritten formal version or automatic deployment/cell startup.
