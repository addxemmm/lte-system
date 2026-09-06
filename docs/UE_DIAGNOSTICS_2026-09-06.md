# 终端入网诊断 / UE registration diagnostics

## 结论与边界 / Findings and limits

本次跟进区分三个阶段：发现小区、无线连接与附着、附着后的数据承载。修复 NAS 协议缺陷不等于修复搜网或射频断流，也不等于已验证手机上网。
This follow-up separates cell discovery, radio connection/attach, and post-attach data bearers. Fixing a NAS defect does not establish that cell discovery, RF streaming, or handset Internet access has been fixed.

- 两张测试卡交换前后，iPhone SE2 均有对应入网尝试。其中一次完成 Authentication、Security Mode 和 Attach Complete；之后发生 RLC 最大重传和 RLF。另一次在初始上行 NAS 后下行重传失败。此证据不支持将两张卡都判定为永久鉴权失败。
  The iPhone SE2 attempted registration with each test card before/after a swap. One attempt completed authentication, security mode and attach, followed by maximum RLC retransmissions and radio link failure. Another failed on the downlink after initial uplink NAS. This does not support classifying both cards as permanently failing authentication.
- 小米 10 的截图没有测试小区；截至本轮分析，没有将新增接入记录可靠归属于该手机。日志存在块缓冲，未出现日志不等于从未尝试。
  Xiaomi Mi 10 screenshots did not show the test cell. No new attach record was reliably attributed to that handset during this analysis. Logs are block-buffered; missing entries are not proof that no attempt occurred.
- 用户报告手机距天线约 1 米。距离本身不足以判定信号质量或过载；天线频率特性、频偏、虚拟化调度和 USB 时序仍需独立测量。
  The reported antenna-to-phone distance is approximately one metre. Distance alone establishes neither signal quality nor overload; antenna response, frequency error, virtual scheduling and USB timing still need independent measurements.

真实 IMSI、认证密钥、原始日志与抓包只保留于服务器私有诊断目录，不进入仓库。
Real IMSIs, authentication keys, raw logs and packet captures remain in private server-side diagnostics, not this repository.

## 已有抓包定位的 NAS 缺陷 / NAS defects identified in existing captures

读取已有 S1AP PCAP，按 [官方指南](https://docs.srsran.com/projects/4g/en/latest/general/source/4_troubleshooting.html) 将 DLT 150 解码为 S1AP；没有为该分析新增抓包进程。
Existing S1AP PCAPs were decoded with DLT 150 mapped to S1AP following the [official guide](https://docs.srsran.com/projects/4g/en/latest/general/source/4_troubleshooting.html). No additional capture process was started for this analysis.

| 阶段 / Stage | 观察 / Observation |
|---|---|
| 初始请求 / Initial request | PDN type 3 (IPv4v6), PTI 10 |
| 默认承载 / Default bearer | PDN type 1 (IPv4), PTI 10, configured data APN; bearer accepted |
| 后续请求 / Additional request | Same data APN, PDN type 2 (IPv6), PTI 11, security header 2 |
| 错误拒绝 / Defective rejection | Cause 32, PTI 0, bare ESM without NAS security protection |
| 重传 / Retransmission | Same PTI/APN/type repeats at roughly 8-second intervals |

后续请求明确是数据 APN 的 IPv6 请求，不是已证实的 IMS 请求。[上游 release_23_11](https://github.com/srsran/srsRAN_4G/tree/release_23_11) 存在两个独立问题：
The additional request is explicitly for IPv6 on the data APN, not an established IMS request. [Upstream release_23_11](https://github.com/srsran/srsRAN_4G/tree/release_23_11) contains two independent issues:

1. `handle_pdn_connectivity_request` 把载荷指针误当作完整消息结构，且解包函数预期裸 ESM 而非带安全头的 NAS。响应也未进行已有安全上下文要求的封装。不能仅修改 PTI 赋值或仅删掉一次 `->msg`。
   `handle_pdn_connectivity_request` casts a payload pointer as a complete message structure, while the decoder expects plain ESM rather than the outer protected NAS envelope. The response also omits protection required by the established security context. Merely changing PTI assignment or removing one `->msg` is insufficient.
2. IPv4-only EPC 在 IPv4v6 请求降为 IPv4 时未通知 ESM cause 50。该条件应按原始请求类型判断；cause 52 表示另一种可分别建立双单栈连接的能力，不适用于这里。
   When this IPv4-only EPC downgrades an IPv4v6 request, it omits ESM cause 50. This must be conditional on the original requested type; cause 52 advertises a different capability to establish separate single-stack connections and is inappropriate here.

协议依据：[3GPP TS 24.301，§4.4.4.2、§6.2.2、§6.5.1.3、§6.5.1.5](https://www.etsi.org/deliver/etsi_ts/124300_124399/124301/12.08.00_60/ts_124301v120800p.pdf)。修复保持额外 PDN 不受支持，不增加 IPv6、IMS 或 VoLTE 能力。
Protocol basis: [3GPP TS 24.301, sections 4.4.4.2, 6.2.2, 6.5.1.3 and 6.5.1.5](https://www.etsi.org/deliver/etsi_ts/124300_124399/124301/12.08.00_60/ts_124301v120800p.pdf). The repair retains the unsupported-additional-PDN behavior; it does not add IPv6, IMS or VoLTE.

实现仅从已验证并解密的内层 ESM 固定头读取拒绝所需的 EBI/PTI，不调用存在嵌套长度缺陷的 APN/PCO 解码器。独立复核同时发现旧加解密函数只使用 8 位序号；补丁改用完整上下行 NAS COUNT，并覆盖 `0xff` 到 `0x100` 回绕。安全门禁置于生产 handler，回归直接调用该门禁而非在测试内复制条件。
The implementation reads only EBI/PTI from the authenticated, decrypted inner ESM fixed header; it does not invoke the APN/PCO decoders with nested-length defects. Independent review also found that ciphering used only the 8-bit sequence number. The patch uses full uplink/downlink NAS COUNT, with `0xff` to `0x100` coverage. The security gate is in the production handler and is called directly by the regression rather than replicated in test code.

## 射频 A–B–A 验证 / RF A–B–A experiment

保持 B7、DL EARFCN 3350、25 PRB、原增益、原固件和其余配置不变，仅比较接收帧大小。每阶段预热 20 秒后记录 120 秒；考虑日志缓冲，只比较三阶段均完整覆盖的前 100 秒。
B7, DL EARFCN 3350, 25 PRBs, gains, firmware and all other settings were held constant while changing receive frame size. Each phase used a 20-second warm-up and 120-second observation; only the first 100 seconds, fully covered in all three buffered logs, are compared.

| 阶段 / Phase | `recv_frame_size` | Underflow | Late | Overflow |
|---|---:|---:|---:|---:|
| A1 | 9232 | 31 | 461 | 0 |
| B | 1024 | 236 | 26592 | 5511 |
| A2 | 9232 | 125 | 1248 | 0 |

1024 明显恶化，已放弃并恢复 9232。A2 有终端接入流量，不能把 A1/A2 的差值单独归因于帧大小。内核在 B 阶段记录过 RT throttling，但单次日志不能证明后续一直限流。基础射频断流问题仍未闭环。
1024 performed markedly worse and was discarded; 9232 was restored. A2 included UE traffic, so A1/A2 differences cannot be attributed solely to frame size. The kernel recorded RT throttling during B, but a single message does not prove ongoing throttling afterward. Baseline RF streaming failures remain unresolved.

## 保留项 / Preserved settings

- BlackSDR 兼容固件 / BlackSDR compatibility FPGA: `UHD_FPGA=compat`.
- 活跃 FPGA SHA-256 / Active FPGA SHA-256: `8e2acce1f987d8452d845705c9c27879171724a99b87c640d0e1473e256fe69a`.
- 原始启动 profile 的字节、订户数据和配置在 A–B–A 后校验一致；不写卡、不更换 FPGA、不修改 PLMN。
  Original profile bytes, subscriber data and configurations were verified after A–B–A; no SIM programming, FPGA replacement or PLMN change.
- 保持 `0.0.0.0:8081`、现有数据卷与回滚镜像；相邻 GSM 容器不参与此次操作。
  Keep `0.0.0.0:8081`, the existing data volume and rollback images; exclude the neighboring GSM container from these operations.

## 发布验证 / Release validation

采用 GPT-6 astra 极高独立分析/复核，两个 GPT-5.6 sol 极高代理分别实现拒绝路径与 IPv4v6 降级，主会话集成与服务器验证。修复在 `folk/nas-pdn-reject` 验证后合入 `master`。
GPT-6 astra at xhigh independently analyzed/reviewed the changes. Two GPT-5.6 sol xhigh agents separately implemented rejection and IPv4v6 fallback; the primary session integrated and validated on the server. Changes were validated on `folk/nas-pdn-reject` before integration into `master`.

- 干净上游基线依次应用两个补丁；Astra 终审无阻断问题。
  Both patches applied to the clean upstream baseline; final independent review found no blocking issue.
- Windows `go test ./...`、`go vet ./...` 通过；Linux 镜像工具链的 `go test -race -count=1 ./... && go vet ./...` 全部通过。
  Windows Go tests/vet passed; the Linux image toolchain passed the full race suite and vet.
- 源码打包测试共 8 项，两平台分别跳过另一平台专用项，其余通过；新增 CRLF 补丁/C++ 源码归档测试且固件保持字节不变。
  Eight packaging tests passed where applicable, with one opposite-platform skip per OS. New CRLF patch/C++ coverage preserves firmware bytes.
- 最终增量 C++ 测试及正式全量 Docker 构建中的 `nas_pdn_reject_test`、`ipv4v6_fallback_test` 均通过。
  Both named C++ regressions passed in the final incremental check and again inside the full production image build.
- 隔离容器内故意恢复 8 位加密 COUNT、故意去掉 cause 50，对应测试分别失败；恢复正确代码后两项再次通过。此变异测试不触及正式源码、设备或生产数据。
  Deliberately reverting cipher COUNT to 8 bits and disabling cause 50 each made its regression fail. Restoring correct code made both pass again. These mutation tests used an isolated disposable container, not production source, devices or data.
- 2026-09-06 20:43（UTC+8）完成容器替换并恢复原小区，17 个数据文件在替换前后、RF 重启前 SHA-256 全部一致。五个配置文件、启动 profile 字节、FPGA、实际数据卷、相邻 GSM 容器身份及启动时间校验一致。
  Replacement completed at 20:43 UTC+8 on 2026-09-06 and the original cell resumed. All 17 data-file hashes matched before/after replacement and before RF restart. Five configurations, profile bytes, FPGA, actual data volume, and the neighboring GSM container identity/start time were unchanged.
- Windows 开发机直连 LAN API 返回 HTTP 200。没有运行会主动探测 SDR 的 health 接口；状态验证使用 `/api/v1/cell`。
  Direct LAN API access from the Windows development host returned HTTP 200. Status validation used `/api/v1/cell`, not the health endpoint that actively probes the SDR.

```text
Image: ltesystem-dep:nas-pdn-20260906
Image ID: sha256:451e805078bb7716ac77cf7d8a3694d04c938946db8c4ee1c386109c6b291fd5
Rollback: ltesystem-dep:rollback-nas-pdn-20260906
Previous ID: sha256:c935785a8e43deb87ba32ad23f1edd67cc8baba63e6cb91f989b097c281ed874
Source archive SHA256: 30b591c11f4249b06ec9375c9043de1b56b005916fc2afd109ce030303f2e0b9
Source: /home/addx/lte-releases/nas-pdn-final-20260906
Private backup: /home/addx/lte-backups/nas-pdn-20260906/data.tgz
Private validation logs: /home/addx/lte-audit-20260906/nas-*.log

lte-system listening on 0.0.0.0:8081 (data=/data)
GET /api/v1/cell -> 200
POST /api/v1/cell -> 200
running=true epc=true enb=true pcap=true band=7
FPGA/config/profile/volume/GSM preservation: PASS
```

回滚镜像和归档已保留；本次发布正常完成，没有实际触发回滚。源码归档之后只补充发布记录，镜像代码/配置输入不变。终端更新后的实际搜网、附着、数据连通性仍待重试验证，不以 CTest 或进程存活代替真机结果。
The rollback image and archive are retained; rollback was not exercised because deployment succeeded. Only release documentation was completed after source export; image code/config inputs were unchanged. Post-update handset discovery, attach and data connectivity still require a real retry; CTest success and process liveness are not substitutes for handset results.
