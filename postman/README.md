# Postman Collections — LTE-System API v3

> **未发布草稿 / Unreleased draft:** 本分支内容未完成最终复核，未合并 master、未推送、未构建或部署。本轮执行工具拦截中止了后续实施；文中新增契约与 Postman 仅供审查，不代表现网已支持。
> Final review is incomplete. This branch has not been merged, pushed, built or deployed. Execution-tool safety checks halted implementation; proposed additions and Postman files do not describe new production capabilities.


## 导入 Import

先删除或替换 Postman 中旧的 `lte-system` collection，再按用途导入：

1. `lte-system.postman_collection.json`：完整集合，共 **21 requests / 20 unique operations**。包含启动、停止、subscriber/SIM 写操作与 crack；这些请求只能人工按需执行，不要将整个集合当作无副作用 smoke run。
2. `lte-system.readonly-smoke.postman_collection.json`：只读冒烟集合，共 **6 个 GET**：`cell`、`network`、`ues`、`subscribers`、`profile`、`diagnostics/connectivity`。它不包含 health、capture、crack、RF 启停、写卡或 CRUD。

Remove or replace any older `lte-system` collection in Postman before importing. The full collection has **21 requests covering 20 unique operations** and includes mutating operations that must be run manually. The read-only smoke collection has exactly **six non-mutating GET requests**.

## 变量 Variables

- `base_url` 默认是不可路由占位符 `http://HOST:8081`；改成 API origin，不要附加 `/api/v1`。
- 服务端启用 `LTE_API_TOKEN` 时填写 `token`；开放模式保持空值。
- 完整集合中的 DNS、IMSI、Key、OPc、SQN 默认均为 `REPLACE_WITH_*` 占位符，不含现场 IP 或凭据。只在 Postman 本地变量中填写测试环境值，不要提交。
- 日常启动使用第一个 Start 请求，其 body 精确为 `{}`，继承已验证 profile。只有首次安装且没有 profile 时才填写并使用独立首次配置模板。

`base_url` is a non-routable placeholder. Configure `token` only when authentication is enabled. Credential and deployment-specific variables are deliberately unusable placeholders; set them locally. The default Start body is exactly `{}` so it does not overwrite a verified saved profile.

## 断言 Assertions

完整集合为安全 JSON GET 提供状态、标准包络及关键 schema 断言。UE/subscriber 详情只接受契约允许的成功或 404；非法占位符产生 422 时测试会失败，避免把配置错误当成功。Diagnostics 严格区分：

- HTTP 200 必须 `code=0`，并包含 registration/PDN/CHAP/user-plane/DNS/network 证据层；
- HTTP 429 必须 `code=42901` 且 `data.reason=diagnostic_busy`；
- 其他状态均失败。

只读 smoke 的六个请求均带断言，但它只验证 API 契约、Manager 配置和已有采样证据；**不发送外网/DNS 探测，也不证明手机实际能上网、域名可解析或无线链路稳定**。小米等终端的偶发掉线仍需结合实时 eNB/EPC/射频日志诊断。

新增完整性断言：读取 `chap.capture` 元数据、检查布尔状态和 packet 数量，拒绝“不完整但 scan_complete=true”及凭据字段。旧 UE 清单语义保持不变，新 UE 在线方案仅为设计；本次未修复或验证 Hashcat 作业链路。

Capture-integrity assertions now reject contradictory completeness flags, invalid counts and credential fields. UE semantics remain unchanged; the new presence model is design-only. Hashcat jobs were not changed or validated in this revision.

The assertions validate API envelopes and bounded evidence only. They do not probe Internet/DNS reachability or prove UE radio stability.

---
**导航 Navigation:** [仓库根](../README.md) · [API v3](../docs/API.md) · [OpenAPI](../docs/api/openapi.yaml)
