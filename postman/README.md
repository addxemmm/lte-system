# Postman Collections — LTE-System API v3

> **2.1 发布更新 / Release update:** 本集合对应已部署的 2.1 契约；合成测试与 API 冒烟不等于手机错误 APN/Internet 验收。见 [发布记录](../docs/RELEASE_2.1_2026-09-07.md)。
> This collection matches the deployed 2.1 contract; synthetic/API checks do not prove incorrect-APN handset behavior or Internet connectivity.

## 版本标识 Version identifiers

运行/发布版本保持 **2.1**。集合名中的 **API v3** 是既有 API 契约标签；`collection/v2.1.0` 是 Postman Collection JSON schema 版本。三者含义独立，本次认证接入不修改 API v3 标签或接口版本。

The runtime/release remains **2.1**. **API v3** in the collection name is the existing API contract label, while `collection/v2.1.0` is only the Postman JSON schema version. Authentication does not rename or version the API contract.


## 导入 Import

先删除或替换 Postman 中旧的 `lte-system` collection，再按用途导入：

1. `lte-system.postman_collection.json`：完整集合，共 **21 requests / 20 unique operations**。包含启动、停止、subscriber/SIM 写操作与 crack；这些请求只能人工按需执行，不要将整个集合当作无副作用 smoke run。
2. `lte-system.readonly-smoke.postman_collection.json`：只读冒烟集合，共 **6 个 GET**：`cell`、`network`、`ues`、`subscribers`、`profile`、`diagnostics/connectivity`。它不包含 health、capture、crack、RF 启停、写卡或 CRUD。

Remove or replace any older `lte-system` collection in Postman before importing. The full collection has **21 requests covering 20 unique operations** and includes mutating operations that must be run manually. The read-only smoke collection has exactly **six non-mutating GET requests**.

## 变量 Variables

- `base_url` 默认是不可路由占位符 `http://HOST:8081`；改成 API origin，不要附加 `/api/v1`。
- 服务端启用固定 Token 鉴权时，在 Postman 的 collection 或 environment scope 填写 `token`；开放模式留空。仓库默认值为空，不填写或提交真实秘密。
- 完整集合中的 DNS、IMSI、Key、OPc、SQN 默认均为 `REPLACE_WITH_*` 占位符，不含现场 IP 或凭据。只在 Postman 本地变量中填写测试环境值，不要提交。
- 日常启动使用第一个 Start 请求，其 body 精确为 `{}`，继承已验证 profile。只有首次安装且没有 profile 时才填写并使用独立首次配置模板。

`base_url` is a non-routable placeholder. Configure `token` only when authentication is enabled. Credential and deployment-specific variables are deliberately unusable placeholders; set them locally. The default Start body is exactly `{}` so it does not overwrite a verified saved profile.

## 固定 Token 鉴权 Fixed-token authentication

- `configs/app.yaml` 的 `api_token: ""` 默认关闭鉴权；文件值去除首尾空白后非空即对所有路由启用 Bearer 校验。
- 非空的 `LTE_API_TOKEN` 环境变量优先覆盖文件配置；未设置、空值或纯空白值不覆盖。Token 同样去除首尾空白。
- 配置只在 API 启动时读取；修改 YAML 或环境变量后必须重启 API。
- 本次文件 Token 支持尚未部署。匿名模式需要有效配置文件且 Token 留空；配置丢失或读取错误不应通过清空 Postman Token 来解决。
- 鉴权启用后，缺失或错误 Token 返回 HTTP `401`、响应 `code=40101`，并包含 `WWW-Authenticate: Bearer`。

Both collections define collection-level `noauth` and one collection pre-request script. Every request has no request-level auth override and inherits that collection setting. The script first removes every stale/generated `Authorization` header, resolves `token` through `pm.variables.get`, trims it, and adds exactly one `Authorization: Bearer <token>` header only when the result is non-empty. Thus an empty or whitespace-only token sends no authorization header and cannot produce `Bearer `.

Postman resolves `token` from highest to lowest priority as local, iteration data, environment, collection, then global scope. Because the imported collection defines an empty default, use a collection value or a higher-priority environment/data/local value; a global value alone is shadowed by the collection default.

## 断言 Assertions

完整集合为安全 JSON GET 提供状态、标准包络及关键 schema 断言。UE/subscriber 详情只接受契约允许的成功或 404；非法占位符产生 422 时测试会失败，避免把配置错误当成功。Diagnostics 严格区分：

- HTTP 200 必须 `code=0`，并包含 registration/PDN/CHAP/user-plane/DNS/network 证据层；
- HTTP 429 必须 `code=42901` 且 `data.reason=diagnostic_busy`；
- 其他状态均失败。

只读 smoke 的六个请求均带断言，但它只验证 API 契约、Manager 配置和已有采样证据；**不发送外网/DNS 探测，也不证明手机实际能上网、域名可解析或无线链路稳定**。小米等终端的偶发掉线仍需结合实时 eNB/EPC/射频日志诊断。

早期完整性增量记录：读取 `chap.capture` 元数据、检查布尔状态和 packet 数量，拒绝“不完整但 scan_complete=true”及凭据字段。新 UE 双源在线方案仍仅为设计；后续认证接口边界见下文和 API 文档。

Capture-integrity assertions now reject contradictory completeness flags, invalid counts and credential fields. The dual-source presence model remains design-only; see the later authentication boundaries below and in the API reference.

The assertions validate API envelopes and bounded evidence only. They do not probe Internet/DNS reachability or prove UE radio stability.

### 错误 APN 受限接入增量 / Restricted APN increment

后续本地增量为 UE 列表增加 schema 1/2 兼容断言：schema 2 区分 `normal`、`restricted`、`deny`，拒绝缺少权限字段或 APN/权限矛盾。这不是双源在线清单功能，也不证明手机已经联网。默认 Start body 仍精确为 `{}`，不自动更改已保存策略。新版本完整验收、部署且停站后，人工指定 `{"apn_mismatch_policy":"restricted"}` 才启用受限模式；旧 profile 默认为 `strict`。详见 [受限 APN 设计与验证](../docs/APN_RESTRICTED_ACCESS_2026-09-07.md)。

The later local increment validates schema 1/2 policy fields on UE lists, without introducing dual-source presence or proving connectivity. The default Start body remains `{}`; it never silently changes the saved policy. Restricted access requires an explicit selection after a validated release and a stopped cell; old profiles default to strict.

### 目标审计边界 / Targeted audit boundary

显式 IMSI 审计尚无可靠每 UE 凭据绑定：未知订户 404、订户库读取失败 500、已知目标 412，且不提取凭据、不停止小区、不启动作业。空 body 是既有整份抓包流程，始终 `ownership_verified=false`；不要把 EPC 聚合上下文当作目标归属。

Explicit IMSI audits fail closed until reliable per-UE credential binding exists: 404 for unknown subscribers, 500 for database errors and 412 for known targets, without credential extraction or cell/job effects. An empty body retains legacy capture-wide behavior and never verifies ownership.

---
**导航 Navigation:** [仓库根](../README.md) · [API v3](../docs/API.md) · [OpenAPI](../docs/api/openapi.yaml)
