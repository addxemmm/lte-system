# 小米偶发无服务与日志减负 / Intermittent Xiaomi service loss and logging load

## 已知事实 / Evidence

- 两台手机同时联网已获用户确认；小米仍偶发无服务、4G 图标消失，不只是某个域名打不开。该反馈不撤销双机并发结果，也不表示长期稳定性通过。
  The user confirmed concurrent Internet access on both phones, but Xiaomi intermittently loses service/the 4G icon. This is not merely a failing domain lookup; concurrent connectivity and long-duration stability are separate results.
- 06:48 UTC 检查时 USB 仍为 5000M，虚拟机 CPU 总体约 95% idle，没有交换或明显磁盘等待；PHY/STACK/TXRX 的实时调度优先级已生效。平均空闲率不排除虚拟化/USB 的短时调度延迟。
  At 06:48 UTC USB remained at 5000M, the VM was approximately 95% CPU-idle without swapping or notable I/O wait, and PHY/STACK/TXRX real-time priorities were active. Low average utilization does not rule out transient virtualization/USB latency.
- 06:51:39–06:53:39 UTC 的约 120 秒只读基线：eNB 日志增长 7,520,256 bytes，RF Underflow 67、Late 1514；RRC 重建请求/完成各 25，无重建拒绝。两台均在窗口前后注册，但会话代次发生变化；这些是小区合计，不强行归属小米。
  A roughly 120-second read-only baseline recorded 7,520,256 bytes of eNB log growth, 67 RF Underflows, 1514 Late reports, and 25 reestablishment requests/completions without rejects. Both UEs were registered at the endpoints, but session generations changed. These are cell-wide counts, not assumed Xiaomi-only events.
- Underflow 表示设备发射数据供应不足；实时性异常可能来自应用、CPU 调度、虚拟化或 USB，日志本身尚未被证明是根因。[UHD streaming](https://files.ettus.com/manual/page_stream.html)、[srsENB troubleshooting](https://docs.srsran.com/projects/4g/en/latest/usermanuals/source/srsenb/source/3_enb_trouble.html)。
  Underflow indicates insufficient data supplied for transmission; application scheduling, virtualization or USB can contribute. Logging has not yet been proven causal. See the official sources above.

## 单变量变更 / Single-variable change

eNB 原先所有层为 info、hex32；PHY/MAC 的逐包格式化会消耗资源，虽然日志后端异步写入。只降低数据面日志，保留 RRC/S1AP/RF 事件：
The previous all-info/hex32 eNB policy formatted PHY/MAC events per packet. Its backend writes asynchronously, but formatting/queueing still consumes resources. Reduce data-plane logs while preserving control/RF events:

```ini
[log]
all_level = warning
all_hex_limit = 0
phy_hex_limit = 0
rf_level = info
rrc_level = info
s1ap_level = info
```

显式 `phy_hex_limit=0` 规避固定上游版本的 PHY 全局继承字段不一致。EPC 日志、PCAP、日志轮转、RF 参数、USB 参数、PHY 线程、RRC 定时器、FPGA、bridge、DNS/APN 与卡库保持不变。本次不以拉长定时器掩盖无线问题，也不声称解决了虚拟化实时性。
The explicit PHY hex limit avoids a field mismatch in the pinned upstream's global inheritance. EPC logging, PCAP, rotation, RF/USB settings, PHY threads, RRC timers, FPGA, bridge, DNS/APN and subscriber data remain unchanged. This change neither masks outages with longer timers nor claims to fix virtualization timing.

对比只使用仍保留的 RF/RRC 日志计数；MAC info 中的 HARQ 重传计数在新策略下不再可比。短窗口、非自动恒定业务负载的前后对比只作初步证据，不能取代手机复测或长期 A/B。
Compare only retained RF/RRC events. MAC-info HARQ retry counts are no longer comparable after filtering. Short windows without a controlled traffic generator provide preliminary evidence, not a substitute for handset validation or long-duration A/B testing.

## Postman 与回归 / Postman and regression

- 完整集合 21 请求、20 个唯一操作；新增只读集合只有 cell/network/ues/subscribers/profile/diagnostics 六个 GET，不包含会探测 SDR 的 health 或任何启停/写卡操作。删除旧集合后重新导入，配置本地 HOST/token 变量。
  The full collection has 21 requests/20 unique operations. The new read-only collection has six GETs and excludes the SDR-probing health endpoint and all mutations. Replace old imports and configure local HOST/token values.
- 增加安全 JSON GET 的状态/包络/schema 断言；diagnostics 严格区分 HTTP 200/code0 与 HTTP 429/code42901/diagnostic_busy。只读测试不证明手机公网、DNS 或无线稳定性。
  JSON GET assertions validate status, envelopes and schema; diagnostics distinguishes its exact 200 and 429 contracts. Read-only API tests do not prove handset Internet, DNS or radio stability.
- Windows 全量 Go tests/vet 通过；Python 14 项中 13 项通过、1 项 POSIX 专用跳过。Node 执行 diagnostics 合成成功/失败用例；另用生产六个只读响应执行集合中的全部 11 个 `pm.test`，全部通过。这不是 Newman 或 Postman GUI 运行结果。
  Windows Go tests/vet passed; Python passed 13 of 14 with one POSIX-only skip. Node executed synthetic positive/negative diagnostics cases, and all 11 committed `pm.test` groups passed against six live read-only responses. This is not a Newman or Postman GUI execution claim.

## 发布与对比限制 / Deployment and comparison limits

- 源包 SHA256 `3e8045c585a4f7f00658f98fb4d424c403f1ea08aa6c9935843a6451ed2b6726`；之后仅追加文档。服务器构建镜像 `ltesystem-dep:enb-log-load-20260907`，ID `sha256:3000e5584d63e21b751f2893afc34873d23a17b1f12a01c84bde792103e765ac`。Linux 全量 race/vet 通过，Python 14 项通过 12、跳过未安装的 PowerShell/Node 两项；Windows 覆盖了这两项。
  The source archive and server-built image have the hashes above; subsequent changes only add documentation. Linux full race tests/vet passed; Python passed 12 of 14 and skipped missing PowerShell/Node cases, both covered on Windows.
- 07:00 UTC 完成替换与原参数恢复，19 个停站数据文件在替换前后哈希一致，profile 字节不变。比对确认 srsepc、srsenb、兼容 FPGA 与旧镜像逐字节一致，生成配置仅 eNB log 段改变。bridge/eth0/8081 与 GSM 保持；本次 Go 测试镜像已删除，只保留 LTE/GSM 两个业务容器。
  Replacement and restoration completed at 07:00 UTC. All 19 stopped data files matched across replacement, and profile bytes were unchanged. srsepc, srsenb and compatible FPGA are byte-identical to the old image; only the generated eNB log section differs. Bridge/eth0/8081 and GSM are preserved. The temporary Go test image was removed; only two business containers remain.
- 回滚镜像 `ltesystem-dep:rollback-log-load-20260907`；私有备份 `$HOME/lte-backups/enb-log-load-20260907`。失败回滚仅恢复原编排/profile，不覆盖最新卡库 SQN。新镜像发布后六个只读 Postman 响应断言再次全部通过。
  The rollback image and server-private backup are retained as above. Rollback restores orchestration/profile without overwriting newer SQNs. All six read-only Postman response checks passed again after deployment.

| 约 120 秒窗口 / Approx. 120-second window | 原 info 基线 / Original info | 新 warning 策略 / New warning |
|---|---:|---:|
| 日志增长 bytes / Log growth | 7,520,256 | 98,304 |
| RF Underflow | 67 | 24 |
| RF Late | 1514 | 315 |
| RRC 重建请求 / Reestablishment requests | 25 | 16 |
| RRC 重建完成 / Reestablishment completions | 25 | 15 |

**该对照不能用于证明掉线改善。** 新窗口 07:01:13–07:03:13 UTC 开始时只有一台注册，结束时另一台请求错误 APN、未注册。用户确认其正在故意测试错误 APN，业务与注册设备数量不一致；计数差异受到混杂因素影响。RF 欠载/迟到仍存在，窗口末尾尚未完成的重建也不自动判定失败。小米在正确 APN 下的偶发无服务仍未解决验收，需恢复相同负载后继续测量。
**This comparison does not establish an outage improvement.** The new window started with one registered UE and ended with the other rejected for an intentionally incorrect APN, as confirmed by the user. Traffic and registered-device counts differed, confounding the comparison. RF underflows/late reports remain; an incomplete reestablishment at the window boundary is not automatically a failed procedure. Xiaomi intermittent service loss with the correct APN remains unverified and requires comparable-load testing.

## 错误 APN 与“有信号但不上网” / Incorrect APN versus service without Internet

07:03/07:04 UTC 当前快照显示测试设备 `requested_apn=addxlte1`、`apn_validated=false`、`emm_state=deregistered`，没有 UE IPv4 或活动承载；用户确认故意测试。该观察验证显式错误值未获得数据会话，但不能解释早前正确 APN 的无线中断。
Current snapshots at 07:03/07:04 UTC showed the intentional test APN, failed APN validation, deregistered EMM state and no UE IPv4/active bearer. This confirms an explicit mismatch was not given a data session; it does not explain earlier radio interruptions with the correct APN.

小区驻留/RRC 连接不等于 EPS 注册。当前代码把初始注册与默认 PDN 建立关联，初始 APN 错误走 Attach Reject（EMM 19），内含 PDN Connectivity Reject（ESM 27）；显示无服务符合此路径。已注册后额外 PDN 请求的拒绝是另一条路径，不应混同。参见 [Cisco EMM/ESM cause mapping](https://www.cisco.com/c/en/us/td/docs/wireless/asr_5000/21-27/mme-admin/21-27-mme-admin/m_enabling_emm_esm_cause_code_mapping.html)。
Camping/RRC connection is not EPS registration. The current initial-attach path couples registration with default PDN establishment; an APN mismatch sends Attach Reject with EMM 19 and embedded PDN Connectivity Reject with ESM 27. No service is consistent with this path. Rejecting an additional PDN after successful registration is distinct; see the implementation reference above.

“错误 APN 仍注册、只阻断上网”不是当前严格拒绝策略。本轮未将错误 APN 映射到默认值，也未增加无 PDN 注册能力或隔离承载。该产品行为若要改变，须单独明确协议/接入策略及手机兼容性，不把它混入日志减负或无线稳定性修复。
Registering an incorrect APN while blocking only Internet traffic is not the current strict-rejection policy. This release neither maps incorrect APNs to the default nor adds no-PDN registration or a quarantine bearer. A different product behavior requires separately specified protocol/access policy and handset compatibility, not an implicit change within logging or stability work.
