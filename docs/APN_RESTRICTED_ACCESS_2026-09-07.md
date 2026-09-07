# 错误 APN 受限接入 / Restricted access for an APN mismatch

> 状态：本地实现与 CPU 回归验证完成；未发布、未部署或启用到服务器，手机实机验收待执行。
> Status: local implementation and CPU regressions complete; not released, deployed or enabled on a server. Handset acceptance remains pending.

## 目标 / Goal

合法订户完成原有 SIM/AKA 鉴权后，格式合法但与配置不匹配的显式 APN 可以进入受限默认承载：保留 EPS 注册，在 SPGW 丢弃双向用户面数据。正确 APN 和省略 APN 仍按原规则处理。非法 SIM、鉴权失败和格式异常 APN 仍拒绝。

After the existing subscriber/AKA checks, a syntactically valid explicit APN mismatch may obtain a restricted default bearer and complete EPS registration, while SPGW drops user-plane traffic in both directions. Matching and omitted APNs retain their normal handling. Invalid subscribers, authentication failures and malformed APNs remain rejected.

LTE 包含无 PDN 注册能力，但依赖 UE 与 MME 双方支持；此实现不宣称提供该能力，而是使用受限默认承载。信号格、4G 图标或“无互联网”提示由手机实现决定，须实机验收。

LTE has an optional registered-without-PDN capability requiring both peers to support it. This implementation instead uses a restricted default bearer. Signal bars, the 4G icon and no-Internet indicators are handset decisions requiring separate acceptance.

协议依据 / Protocol reference: [ETSI TS 124 301 V17.12.0 §5.5.1.2.4](https://www.etsi.org/deliver/etsi_ts/124300_124399/124301/17.12.00_60/ts_124301v171200p.pdf#page=131).

## 配置与生效边界 / Configuration and activation

- `apn_mismatch_policy: strict` 为默认，沿用错误 APN 的 Attach Reject；旧 profile 缺少此字段时使用 strict。
- `apn_mismatch_policy: restricted` 明确启用受限接入；参数非法时启动校验失败，不静默回落。
- 参数属于每次启动配置，保存到 profile；空请求继承 profile，显式 strict 可覆盖已保存的 restricted。
- Manager 把有效策略显式传给 EPC 的 `LTE_APN_MISMATCH_POLICY`；父进程同名变量不覆盖启动请求。

Strict is the backward-compatible default. Restricted must be explicitly selected and is persisted with the successful startup profile. Invalid values fail validation. The Manager supplies the effective EPC environment value explicitly.

在完整新版本通过验收并部署后，已停站且存在有效 profile 的系统可使用以下启动 body；这不是当前服务器已支持的声明，也不是在线热更新命令：

After a validated full release is deployed, a stopped cell with a valid saved profile can use this startup body; this does not claim current server support or hot reconfiguration:

```json
{"apn_mismatch_policy":"restricted"}
```

端点 / Endpoint: `POST /api/v1/cell`。运行中的小区保留原有 409 行为。此文档不自动停止或启动小区。

## 数据边界 / User-plane boundary

受限策略从会话建立时绑定，成功确认之前保持 deny；不是先允许转发再异步添加防火墙。受限会话的公网、上游 LAN、EPC/宿主服务、DNS、其他 UE 流量均在 SPGW 双向丢弃。`ue_access=allow` 也不解除该限制。

The policy is bound at session creation, with deny before confirmation, rather than asynchronous post-admission firewall updates. Restricted traffic is dropped in both directions before host/LAN/DNS/peer access. UE peer-access settings do not override restriction.

idle/恢复保留原策略；旧会话队列和响应按当前所有权/代次校验。普通与受限地址类别在一次 EPC run 内分开，防止跨策略地址复用；同类回收仍保留。restricted 模式仅在可用动态地址超过 1 个时预留最多 16 个受限地址，同时留下至少一个普通动态地址；静态订户地址不放入受限池。受限池耗尽时拒绝新承载，不回退到普通联网权限。strict 模式不预留受限池。

Idle/resume retains the policy, and queued/control work is checked against current ownership/generation. Restricted mode reserves at most 16 available dynamic addresses, leaving at least one normal dynamic address and excluding static reservations. Same-class recycling is retained; exhaustion denies new restricted bearers rather than granting normal access. Strict mode reserves no restricted pool.

## 状态契约 / Telemetry contract

新 EPC producer 发布 schema 2，配套 Go 消费端继续支持 schema 1；旧消费端不应单独搭配新 producer。

The new producer emits schema 2. The paired Go consumer accepts schema 1 and 2; upgrade producer and consumer together.

| access_policy | access_reason | apn_validated | selected_apn |
|---|---|---|---|
| normal | apn_match / apn_omitted | true | 实际下发的配置 APN / selected configured APN |
| restricted | apn_mismatch | false | 实际下发的配置 APN / selected configured APN |
| deny | session_unavailable | false | 空/省略 / empty or omitted |

`requested_apn` 保留手机实际请求。restricted 不把错误请求伪装为 APN 校验成功；registered/active 也不等于具备 Internet 访问权限。schema 2 缺失、null、未知或矛盾的权限字段由消费端拒绝。

Keep the actual requested APN. A restricted session never claims an APN match, and registration/activity is not Internet permission. Missing, null, unknown or inconsistent schema-2 policy fields are invalid.

## 验收矩阵 / Acceptance matrix

1. strict：保留原拒绝行为；restricted：合法错误 APN 走完整 Attach/ESM information/Attach Complete。
2. 未授权 SIM、鉴权失败、异常 APN 不获接入；正确和省略 APN 的正常 UE 不受限制策略误伤。
3. 实际用户面出口对受限会话双向零写入，普通会话作为对照；覆盖 DNS、LAN、peer 和本机边界。
4. idle/恢复、重连、旧队列/响应、地址回收、受限池耗尽及安装/发送失败保持限制。
5. 全量 Go/vet、Linux race、原五组及新增 C++ 回归、OpenAPI/Postman 契约检查。
6. 实机单独验证两台手机并发、注册状态、信号显示、错误 APN 数据受限、改回正确 APN 后重新附着并恢复联网；本地合成回归不替代该步骤。

## 验证状态 / Validation status

- 修改前 Go 的 LTE/parser/API 三包基线通过。
- 修改前固定上游 + 0001–0003 在 WSL 完整编译 srsepc，原五组 CTest 全部通过。
- Windows Go 1.26.2：最终 `go test -count=1 ./...`、`go vet ./...` 全部通过，Linux/amd64 交叉编译通过。
- WSL Go 1.26.5：`go test -race -count=2 ./...` 全部通过。顺带修正 Linux 短命进程测试缺少合成 wordlist fixture 的问题；仅为临时测试文件，不改该模块业务逻辑。
- 固定上游 + 0001–0004 的 clean LF apply 检查通过，`srsepc` 编译通过；原五组和新增两组 CTest 共 **7/7 通过**，最终每项连续重复三次也全部通过。
- 新 C++ 覆盖错误/正确/省略 APN 的确认与 Attach Complete、SMC/ESM information 两分支；正常 600 次回收、受限 16 地址耗尽、上下行真实 pipe/loopback 出口、DNS/LAN/host/peer 目标边界、队列容量与过期代次、迟到响应、删除重试及 Create/confirm/ICS 失败通知一次性。
- Python 合约/部署脚本 mock：Linux **15/15 通过**；Windows 14 项通过，1 项 POSIX 专属测试按平台跳过。Postman 新增 25 个正反例分别检验完整和只读集合，默认启动 body 仍为 `{}`。
- OpenAPI 23 个合成正反例通过；真实 C++ schema-2 producer 导出的三种策略样本，经 Go 消费后再通过 OpenAPI 校验。
- `git diff --check` 通过，0001–0003 未改写。新 0004 的 SHA-256：`c3eda146f75c96b66c47dc30cfa14b6dba067e12cb53160371cfbe420a0dccfb`。

Local Windows Go tests/vet, Linux cross-build, two Linux race runs, seven C++ regression groups (each repeated three times), Python/Postman and the actual producer→Go→OpenAPI synthetic contract all passed. Tests use synthetic identities and short-lived local endpoints, not handset traffic.

### 环境差异与证据 / Environment and evidence

C++ 在独立 WSL 临时源码树、GCC 15.3/CMake/Ninja 与本地解包的依赖 sysroot 上执行；现代编译器基线使用 `ENABLE_WERROR=OFF`，没有运行生成的 srsepc 服务，也没有在开发机构建 Docker 镜像。此结果不替代 Ubuntu 部署镜像构建或实际射频/手机验收。

C++ validation used an isolated WSL source tree and dependency sysroot. The modern-compiler baseline disables warnings-as-errors; the EPC executable was built but never launched. This does not replace the deployment-image build or RF/handset acceptance.

本地未跟踪证据保存在仓库忽略目录 `.codex-go-cache-apn-restricted-20260907/`：`restricted-cpp.log`、`final-cpp-repeat.log`、`final-all-go.log`、`wsl-kali-go-race-final.log`、`windows-go-vet-final.log`、`windows-linux-amd64-build-final.log`、`producer-v2.json`、`producer-api-v2.json`。测试源及回归断言是可复现依据；缓存日志不纳入发布包。

剩余发布步骤：复核并提交完整新增源码，在服务器构建配套镜像并执行成套发布；停站后明确启用 restricted，再验收两台手机的注册/信号显示、错误 APN 数据阻断及正确 APN 恢复。本轮没有执行这些操作，也没有变更实际 profile、SIM、SQN、增益、频点或现网服务。

No deployment, RF change, SIM programming or production data modification is performed by this document or its CPU tests.
