# S1AP 完整性诊断修复 / S1AP integrity diagnostics

> **未发布草稿 / Unreleased draft:** 本分支内容未完成最终复核，未合并 master、未推送、未构建或部署。本轮执行工具拦截中止了后续实施；文中新增契约与 Postman 仅供审查，不代表现网已支持。
> Final review is incomplete. This branch has not been merged, pushed, built or deployed. Execution-tool safety checks halted implementation; proposed additions and Postman files do not describe new production capabilities.


## 结论与交付范围 / Findings and scope

截图中的 `42202/capture_decode_failed` 至少有一个已确认原因：2026-09-07 07:13:28 UTC 只读检查时，正在写入的 classic-PCAP 文件为 28,672 字节、DLT 150；tshark 3.6.2 已解出前部 S1AP/NAS/IPCP 元数据，但以状态 2 报告最后 packet 被截断。旧诊断将这个整体归类为解码失败。这不证明手机未发送 APN 信息，也不证明存在可恢复的认证交换。

A read-only check at 07:13:28 UTC found a 28,672-byte live classic-PCAP, DLT 150. Tshark 3.6.2 decoded earlier S1AP/NAS/IPCP protocol metadata but exited 2 on the partial final packet. The old diagnostic discarded this distinction. It is not evidence that a handset omitted APN information or that recoverable authentication exists.

本次修改限于完整性/协议存在性诊断、回归、标准 API 文档与 Postman 断言。认证内容提取与 Hashcat 作业实施，以及 eNB C++ 状态快照实施触发执行工具安全拦截，均未交付；没有改动 `internal/crack/crack.go`，没有运行真实破解作业，也没有发布不完整的 UE 新接口。

This revision changes integrity/protocol-presence diagnostics, regressions, API documentation and Postman assertions only. Execution-tool safety checks blocked authentication/Hashcat implementation and the eNB snapshot implementation; neither is delivered. The extraction implementation is unchanged, no real cracking job was run, and no incomplete UE API was released.

## 实现边界 / Implementation boundaries

- 只接受普通文件，校验文件身份；最多读 64 MiB，区分 four classic-PCAP magic/大小端、2.4 header、snaplen、DLT 和 record 长度。
- 仅将完整 record 前缀写到同目录 0600 临时文件；原文件不截断不改写，正常/失败/取消均清理临时副本。
- tshark 只请求 `frame.number`、`frame.protocols`、`chap.code`。不请求用户名、密码、challenge、response 或 hash；stderr 只返回脱敏原因分类。
- 源变化或尾 record 不完整时 `scan_complete=false`。正协议元数据可以保留，但未观察结果是 unknown，不能推导“没有认证”。
- 缺工具、参数不兼容、超时、取消、无 packet、坏格式、unsupported DLT 和读失败分开表达。文件 IO 使用协作式取消，不承诺硬墙钟上限。

Only regular bounded captures are inspected. An immutable complete-record prefix is private and removed on every return path; the original is untouched. The decoder requests protocol metadata only, and stderr is classified rather than exposed. Source changes make absence inconclusive. File IO cancellation is cooperative, not a hard wall-clock guarantee.

## UE 清单 / UE roster

现行 `/ues` 已使用 MME 原子上下文快照；缺陷不是仍然只读日志，而是上下文存在、注册、无线连接与实际可达性没有充分区分。新的双源状态方案和验收矩阵见 [UE 设计](UE_PRESENCE_DESIGN_2026-09-07.md)。该方案未部署，本次保持旧 UE/subscriber 返回契约。小米偶发无服务仍是未解决项。

The deployed roster already uses atomic MME snapshots. Context existence, registration, radio connection and reachability are different facts. See the design for proposed dual-source semantics and acceptance tests. It is not deployed, UE/subscriber contracts remain unchanged, and intermittent Xiaomi service loss remains unresolved.

## 验证与发布状态 / Verification and release status

本地合成测试覆盖格式、截断、源替换、取消、清理、错误分类及无凭据字段。全量 Go/vet、Python/Postman 离线检查在本轮首轮通过；最终复核与服务器构建/发布记录将在完成后补充。本节不代表硬件验收完成。

Synthetic tests cover framing, truncation, replacement, cancellation, cleanup, classifications and credential-free responses. The first complete Go/vet and Python/Postman offline runs passed; final verification and server build/release evidence will be appended when completed. This is not handset acceptance.

### 中止时状态 / State at implementation halt

- `go test ./...`、`go vet ./...`：诊断初稿首轮通过；之后子代理已写入实际文件 metadata 绑定和 WaitDelay 修订，但终轮执行被拦截，**这些最终修订未验收**。
- Python 14 项：首轮通过 13 项，Windows 跳过 1 项 POSIX shell 测试；Postman Node 合成契约用例通过。OpenAPI YAML 与本地 `$ref` 解析通过。
- Linux race、镜像构建、替换容器、手机验收、GitHub push：**本轮均未执行**。既有生产镜像 `ltesystem-dep:enb-log-load-20260907` 保持不变。
- 服务端本轮后续只读预检确认 bridge 单网络、仅 8081 发布、`docker_lte-data` 保留、LTE 仍运行且 GSM 停止；没有启动新射频或改参数。
- 临时实现分支保留未发布草稿。后续先解决执行权限限制，再完成独立审查、全量复测、Linux 检查和新一轮发布验收；不要直接导入草稿 Postman 并假定现网接口已改变。

The first draft passed full Go tests/vet and 13 of 14 Python checks (one Windows POSIX skip), including synthetic Postman assertions. OpenAPI parsing/references passed. Later metadata-binding/WaitDelay edits were written before the final agent turn was blocked; those revisions remain unaccepted. No Linux race run, image build/deployment, handset acceptance or GitHub push occurred in this turn. Production remains on the prior image. Resolve execution permissions and complete review/tests before release; draft Postman changes are not proof of production support.
