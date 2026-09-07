# 手机数据与 bridge 网络诊断 / Handset data and bridge network diagnostics

## 取证结论 / Observed findings

2026-09-07 在 SDR 服务器做只读检查，未通过诊断接口停止小区、改写 SIM 或更换 FPGA。以下是运行时观察，不是仅凭截图的推测。
Read-only checks on the SDR server on 2026-09-07 did not stop the cell, program SIMs or replace FPGA firmware. These findings come from runtime evidence, not the screenshot alone.

| 检查 / Check | 观察 / Observation | 含义 / Meaning |
|---|---|---|
| 默认路由 / Default route | host 模式有有效出口，IPv4 forwarding=1 / Host mode had a valid uplink and forwarding=1 | 不是完全缺少转发 / Forwarding was not entirely missing |
| NAT/filter | UE MASQUERADE 与双向 ACCEPT 已有包计数 / UE MASQUERADE and both ACCEPT directions had packet counters | 对应规则被命中，不等于所有手机流量正常 / Rules were hit, not proof of complete handset connectivity |
| UE 数据 / UE data | 现有 SGi 抓包中多个 UE 地址仅有 DNS 上行；另一个地址已有 TCP 双向数据 / Existing SGi capture showed DNS-only uplink for several UE addresses and bidirectional TCP for another | LTE 用户面至少部分可用；不把所有会话混为一台手机 / User plane was partly working; sessions are not assumed to be one handset |
| DNS | 配置的 8.8.8.8 从服务器查询也超时；1.1.1.1 同样超时；当前 LAN resolver 可响应 / Configured 8.8.8.8 timed out even from the server; so did 1.1.1.1; the LAN resolver responded | 有直接证据的 DNS 故障 / Direct evidence of a DNS failure |
| 上游连通 / Upstream access | 使用可用解析路径，Apple captive HTTP 与 Apple HTTPS 均为 200 / With working resolution, Apple captive HTTP and Apple HTTPS returned 200 | 上游对这些测试目标可达，不代表全部网站 / These upstream test targets were reachable, not necessarily every site |
| MSS | mangle 表中无预期规则；旧 argv 为 `iptables -A -t mangle FORWARD ...` / Expected mangle rule absent; old argv was malformed | 独立确认的实现错误 / Independently confirmed implementation defect |
| CHAP | 专用接口返回 412，被当作“未连接” / Specialized endpoint returned 412, mistaken for disconnection | CHAP 前置条件与 LTE 注册/上网是不同事项 / CHAP preconditions differ from LTE registration and Internet access |

当前 LAN resolver 和可达的公共 DNS 地址返回相同的 `198.18.x.x` 结果，符合上游 fake-IP 代理的表现；这是推断，不把它当 DNS 攻击或透明代理配置已被审查。Apple HTTPS/HTTP 测试可通过，而某个其它 HTTPS 目标曾返回 TLS EOF，因此不以单一测试泛化整个互联网。
The LAN resolver and reachable public resolver addresses returned matching `198.18.x.x` answers, consistent with an upstream fake-IP proxy. This is an inference, not a finding of a DNS attack or a review of the proxy configuration. Apple HTTPS/HTTP tests passed while another HTTPS target returned TLS EOF; one target does not characterize the whole Internet.

抓包正在增长，读取可能以尾部不完整结束；统计仅来自已经成功解码的部分，不表示完整、最新或当前逐 UE 状态。没有输出应用载荷、DNS 域名历史、CHAP 凭据或 SIM 密钥。
The capture was growing and could end with a partial trailing record. Counts cover only successfully decoded records, not a complete/current per-UE state. No application payloads, DNS browsing history, CHAP credentials or SIM keys were reported.

用户随后确认：手机可以通过 IP 访问公网，但域名访问失败。这与 DNS 查询无响应的证据一致；不应继续把当前症状描述成“搜不到小区”或“缺少 NAT”。
The user subsequently confirmed that the handset could reach the public network by IP but not by domain name. This matches the unanswered DNS queries; the current symptom should not be described as an undiscoverable cell or absent NAT.

## 变更范围 / Change scope

- bridge 编排独立于 host 回滚入口，固定容器出口 `eth0`，只发布 API TCP 8081；默认仍允许 LAN 通过宿主地址访问。
  Bridge orchestration is separate from the host rollback entry, fixes the uplink as `eth0`, and publishes only API TCP 8081, retaining default LAN access through the host address.
- 网络策略 `auto` 与实际出口分离；修复 iptables 参数顺序、错误处理、精确转发和 owned-rule 回滚。不 flush 宿主防火墙、不修改全局 FORWARD 策略。
  Keep the `auto` policy separate from the resolved uplink; fix iptables argument order, error handling, scoped forwarding and owned-rule rollback. Never flush host firewall rules or change the global FORWARD policy.
- 当前部署只显式调整 profile 的网络策略与 DNS，原来的 Band 8、射频增益、设备参数、USB、BlackSDR compat 固件和数据卷保持不变。通用源码不写入实际 LAN resolver 地址。
  This deployment explicitly adjusts only the profile's network policy and DNS, preserving Band 8, gains, device arguments, USB, BlackSDR compat firmware and data volume. The actual LAN resolver address is not embedded in generic source configuration.
- 新的只读 connectivity 诊断区分小区进程、注册/承载证据、网络配置、用户面采样和 CHAP 可观察性。不因缺少 CHAP 判断 iPhone 未联网，不会为诊断自动运行破解或停站。
  New read-only connectivity diagnostics separate cell processes, registration/bearer evidence, network configuration, user-plane sampling and CHAP observability. Missing CHAP does not mean an iPhone is disconnected; diagnostics do not automatically run cracking or stop the cell.

## 验证记录 / Validation record

为先恢复域名访问，在 bridge 发布前单独修复当前 host 部署的 DNS，并恢复原来的运行状态。原 profile、配置与日志保存在服务器私有 `lte-backups/dns-fix-20260907`；eNB 配置字节完全不变。重新接入后的现有 SGi 抓包观察到 **70 次 DNS 查询、70 次 DNS 响应**，以及 2,980 个上行包、3,474 个下行包和 3,006 个 TCP 数据包。读取活跃文件仍可能遇到不完整尾部，计数是已解码部分的证据。
To restore domain access first, DNS was fixed on the existing host deployment before the bridge release, then its prior running state was restored. The original profile, configs and logs are preserved in the server-private `lte-backups/dns-fix-20260907`; eNB configuration bytes were unchanged. Existing SGi capture after reattachment showed **70 DNS queries and 70 DNS responses**, with 2,980 uplink packets, 3,474 downlink packets and 3,006 TCP data packets. An active file can still have an incomplete trailing record; counts describe the decoded portion.

用户已确认：关闭 Wi-Fi、重新接入后，iPhone 的 Safari 可以通过域名打开网页。这验证了本次 DNS 修复，但当时容器仍为 host，不能写成 bridge 验证通过。
The user confirmed that, with Wi-Fi disabled and after reattachment, iPhone Safari could open pages by domain name. This verifies the DNS fix; the container was still using host networking, so it is not evidence of a successful bridge migration.

### Bridge 发布 / Bridge release

2026-09-07 11:13（UTC+8）发布成功；当前镜像 `ltesystem-dep:bridge-diagnostics-20260907`，ID `sha256:4f0e42c888dd9277e3a30cec95cc2c6947050956b0eb2411b68ad9939602ba48`。服务器源码快照为 `lte-releases/bridge-diagnostics-final-20260907`，归档 SHA-256 为 `ea0f064cc45a62926ceb327633937395e6f9805dd619edd4a65e8c48cdc656d8`。此归档包含构建源，本文的发布后验证记录在其后补充。
Release succeeded at 11:13 UTC+8 on 2026-09-07. Current image: `ltesystem-dep:bridge-diagnostics-20260907`, ID `sha256:4f0e42c888dd9277e3a30cec95cc2c6947050956b0eb2411b68ad9939602ba48`. Server source snapshot: `lte-releases/bridge-diagnostics-final-20260907`; archive SHA-256: `ea0f064cc45a62926ceb327633937395e6f9805dd619edd4a65e8c48cdc656d8`. That archive contains the build source; this document's post-release evidence was appended afterwards.

- `docker inspect` 确认网络为用户定义 bridge `docker_lte-uplink`，而非 host；容器出口 `eth0`，profile 为 `network=auto`。
  Docker inspection confirms user-defined bridge `docker_lte-uplink`, not host; container uplink is `eth0`, with `network=auto` persisted.
- 仅发布 `0.0.0.0:8081 -> 8081/tcp`；GTP 仍绑定容器内部 `127.0.1.x:2152`。开发机通过 LAN 地址成功调用 cell API。
  Only `0.0.0.0:8081 -> 8081/tcp` is published; GTP remains on container-internal `127.0.1.x:2152`. The development machine successfully reached the cell API over LAN.
- 相同的 `docker_lte-data` 卷被复用，重建前后静止状态下 **17 个数据文件哈希一致**；FPGA、eNB/静态配置一致；GSM 容器 ID、镜像和状态不变。
  The same `docker_lte-data` volume is reused; **17 data-file hashes matched** across idle container replacement. FPGA and eNB/static configs matched, and GSM container ID, image and state were unchanged.
- bridge 内默认路由、SGi、IPv4 转发、NAT、两条精确 FORWARD 规则和双向 MSS 规则均验证通过。容器自身 Apple HTTP/HTTPS 返回 200；LTE/EPC/eNB 已恢复运行。
  The bridge default route, SGi, IPv4 forwarding, NAT, two scoped FORWARD rules and bidirectional MSS rules all verified. Container-side Apple HTTP/HTTPS returned 200; LTE/EPC/eNB were restored to running.
- 回滚组合为 `ltesystem-dep:rollback-bridge-20260907` + 原 host 编排 + `lte-backups/bridge-diagnostics-20260907` 中的 profile。该 profile 已包含前一步修复的 DNS；回滚不覆盖新 SQN。私有备份、日志、原始抓包均未上传 GitHub。
  Rollback pairs `ltesystem-dep:rollback-bridge-20260907` with the original host orchestration and profile under `lte-backups/bridge-diagnostics-20260907`. This profile already includes the preceding DNS repair; rollback does not overwrite newer SQN state. Private backups, logs and raw captures were not uploaded to GitHub.
- 已清理本轮的两个 Go 测试镜像和一个失败候选镜像；只保留 GSM/LTE 两个业务容器。未做全局镜像、构建缓存或卷清理。
  Removed this run's two Go test images and one failed candidate image; only the GSM/LTE service containers remain. No global image, build-cache or volume pruning was performed.

### 回归与边界 / Regressions and boundaries

- Windows `go test ./...`、`go vet ./...` 通过；源码打包测试 8 项通过（Windows 下 POSIX 专用项跳过）。Postman JSON 及 12 个原始请求体解析通过。
  Windows `go test ./...` and `go vet ./...` passed; the eight-test packaging suite passed with its POSIX-only case skipped on Windows. Postman JSON and 12 raw request bodies parsed successfully.
- 服务器完整编译 srsRAN，`nas_pdn_reject_test`、`ipv4v6_fallback_test` 均通过；最终 Linux `go test -race -count=1 ./...`、`go vet ./...` 全部通过。
  The server fully rebuilt srsRAN; `nas_pdn_reject_test` and `ipv4v6_fallback_test` both passed. Final Linux `go test -race -count=1 ./...` and `go vet ./...` passed throughout.
- 首轮 Linux 回归发现“绝对路径工具不存在”被误报为抓包解码失败；已兼容 `os.ErrNotExist`、添加回归并重新构建，不是忽略失败重试。
  The first Linux regression found an absent absolute-path tool misclassified as a capture decode failure. Added `os.ErrNotExist` handling and regressions, then rebuilt; the failure was fixed rather than ignored.
- auto 仅覆盖主 IPv4 路由表；复杂策略路由/ECMP 需单独验证。诊断是有界聚合证据，不是逐 UE 实时状态。生命周期锁等待不包含在严格的墙钟 SLA 内；并发诊断有单槽限制，忙时 429。
  Auto covers the main IPv4 route table; complex policy routing/ECMP requires separate validation. Diagnostics provide bounded aggregate evidence, not per-UE live state. Lifecycle-lock waiting is not covered by a strict wall-clock SLA; one diagnostic slot limits concurrency, returning 429 when busy.

首次 bridge 发布后检查时抓包尚为空，正确返回 `not_collected/capture_empty`，没有误报“iPhone 不支持 CHAP”。随后一次只读诊断已观察到 Attach Complete 与 EPS bearer 激活，并从活跃 SGi 抓包保留了 **1,639 个上行包、1,755 个下行包、91 次 DNS 查询及 84 次 DNS 响应**。结果为 `partial_decode/scan_complete=false`，不是完整会话统计，也不能据查询与响应差值计算丢包率。已向用户请求 bridge 下的手机浏览器复测；聚合网络证据不能替代该人工结果。
At the first bridge postflight check, captures were still empty and correctly returned `not_collected/capture_empty`, not an assertion that an iPhone lacks CHAP support. A later read-only diagnostic observed Attach Complete and EPS bearer activation, retaining **1,639 uplink packets, 1,755 downlink packets, 91 DNS queries and 84 DNS responses** from the active SGi capture. The result is `partial_decode/scan_complete=false`, not complete session statistics; the query/response difference is not a packet-loss measure. A handset-browser retest on bridge has been requested; aggregate network evidence does not replace that manual result.

**最终人工复测：用户已确认 bridge 模式下，iPhone 关闭 Wi-Fi 后仍可正常通过域名打开网页。** 这与新诊断观察到的双向用户面与 DNS 回包一致；不据此声称所有其它手机或语音/IMS功能已经验证。
**Final manual retest: the user confirmed that, on bridge with Wi-Fi disabled, iPhone could still open pages normally by domain name.** This agrees with the diagnostic's bidirectional user-plane and DNS-response evidence, without claiming validation of all other handsets or voice/IMS features.

## 参考 / References

- bridge 的 NAT、网络隔离与已发布端口行为 / Bridge NAT, isolation and published ports: [Docker bridge documentation](https://docs.docker.com/engine/network/drivers/bridge/).
- 固定接口名称需要 Compose >= 2.36.0 / Fixed interface names require Compose >= 2.36.0: [Compose interface_name](https://docs.docker.com/reference/compose-file/services/#interface_name).
- S1AP pcap 解码设置与局限 / S1AP pcap decoding configuration: [srsRAN 4G troubleshooting](https://docs.srsran.com/projects/4g/en/latest/general/source/4_troubleshooting.html).
