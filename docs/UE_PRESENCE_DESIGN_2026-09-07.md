# UE 在线清单设计 / UE presence design

**状态：设计方案未实施、未发布；现网 API 契约不变。** 所需 eNB C++ 生产端尚未交付，消费端探索草稿不作为完整功能部署。本次仅形成设计文档，不修改 UE 源码，不编译、不连接服务器、不改变运行配置。

**Status: design only, not implemented or released; the deployed API contract is unchanged.** The required eNB C++ producer is outstanding. Consumer prototypes are not a deployable feature. This handoff changes documentation only, with no UE source edits, builds, server access or runtime changes.

## 1. 现状与边界 / Current evidence and limits

已发布清单来自 MME 生命周期内的结构化上下文快照，并非单纯匹配历史日志。快照仍会包含正常空闲的 registered/idle UE，以及注销、拒绝后保留的 NAS 上下文；“存在上下文”不等于“当前在线”。`Disconnecting` 日志也可能只是成功重建后删除旧 RNTI。

The deployed list uses structured MME context snapshots, not merely historical log matching. Registered-idle UEs and retained deregistered/rejected NAS contexts may remain present. Context existence is not current connectivity; deleting an old RNTI after successful reestablishment can also produce a disconnect log.

EMM 注册状态与 ECM 连接状态应分别解释：正常 idle 不代表关机或离线，连接状态也不证明互联网可达。协议依据：[ETSI TS 123 401 V18.5.0 §4.6](https://www.etsi.org/deliver/etsi_ts/123400_123499/123401/18.05.00_60/ts_123401v180500p.pdf)。

EMM registration and ECM connectivity are distinct. Normal idle is not proof of power-off or absence, and a connection is not proof of Internet access. The cited specification also permits temporary differences between UE and network connection state.

## 2. 拟议视图 / Proposed views

| 视图 / View | 拟议含义 / Proposed meaning |
|---|---|
| `connected`（默认 / default） | 当前双源确认的无线连接与有效承载；释放后移出。Current radio binding and bearer confirmed by both sources; removed after observed release. |
| `registered` | MME 注册集合，包含正常 idle；不宣称每台当前可达。MME registration set, including normal idle, without a reachability claim. |
| `all` | 当前 MME 上下文的诊断集合，显式区分注册、空闲、拒绝等；不是历史在线名单。Current MME contexts with explicit diagnostic states, not a historical online roster. |

历史事件另行展示，不回填当前状态。缺失、过期、无效来源返回 unknown/unavailable，在线数量为未知而非 0；只有完整、当前的空集合才表示 0。拟议订户 `registered` 与 `active` 分别表达注册和双源连接证据，未知使用 null；这些均不是现网新契约。

Keep history separate. Missing, stale or invalid sources produce unknown/unavailable counts, not zero; only a complete current empty view proves zero. Proposed subscriber registration and active flags describe different evidence, with null for unknown. These semantics are not deployed API changes.

## 3. 双源契约 / Dual-source contract

保留 MME schema 1。拟新增 eNB 所属 STACK 线程采集的 RRC/S1AP 快照，包含 RNTI、双 S1AP ID、连接状态及 run/PID/启动代次/序列/采集时间。同线程复制值，通过有界后台写出、同目录临时文件及原子替换发布；写出线程不自行刷新采集时间。

Keep MME schema 1. Capture eNB RRC/S1AP values on their owning STACK thread, including RNTI, both S1AP IDs, state and producer identity/sequence/timestamps. Publish immutable copies through a bounded writer and same-directory atomic replacement. Writer activity must not manufacture fresh capture timestamps.

仅按同一 run 与双 S1AP ID 唯一关联，并分别核验生产进程；不比较两端各自本地的 SCTP association 数值，不按数组下标、IP 或日志字符串拼接。5 秒是来源新鲜度门槛，不是 UE 存活 TTL。静默关机没有即时检测承诺；本方案不新增探测、活动 TTL 或网络定时器。

Join uniquely by the same run and both S1AP IDs, validating each producer separately. Do not equate endpoint-local SCTP association numbers or join by array index, IP or logs. Five seconds measures source freshness, not handset life. No immediate silent-power-off detection, probing or new network timers are promised.

## 4. 审查发现 / Review findings

- 有效 ServiceRequest 缺 association 集合登记；旧释放消息须校验当前 tuple，保留旧 alias 清理防护。eNB 的 T301 本地删除可能先于 MME 状态更新，故仅筛 MME connected 仍不足。
  Valid ServiceRequest lacks association-set registration. Validate release tuples and retain stale-alias protection. Local eNB deletion can precede MME state changes.
- 消费端需拒绝重复绑定、保留 RNTI、缺失/null ID；显式合法 ID 0 与缺字段分开。两源读取完成后复核新鲜度，未知数量及订户三态需独立测试。
  Reject ambiguous bindings, reserved RNTIs and missing/null IDs while preserving explicitly valid zero IDs. Recheck freshness after both reads; test unknown counts and subscriber tri-state semantics.

## 5. 后续验收 / Future acceptance

CPU 回归须覆盖：双 UE 中仅一台释放；正常 idle 后恢复；Detach/APN 拒绝；RLF/T301 本地删除；旧 RNTI 迁移与延迟 ReleaseComplete；SCTP 中断、进程崩溃/停止、旧 run/PID、过期来源及重复 tuple。确认另一 UE、保留 PDN 地址和新连接不受误清理。

CPU regressions must cover independent two-UE removal, idle/resume, detach/rejection, local radio deletion, RNTI migration, delayed releases, SCTP loss, process stop/crash, stale identities and duplicate tuples. Preserve the other UE, retained PDN address and replacement connection.

待完整生产端与消费端成套验证后，再单独安排实机验收：记录每台手机中断时刻与当前结构化证据，验证移除及重接；并发上网和长期稳定性分别报告。当前仅交付本设计，不将既有双机上网成功当作本方案验收。

Only a complete, tested producer/consumer pair should proceed to separately scheduled handset acceptance. Correlate reported outages with structured evidence, verify removal and reconnection, and report concurrent Internet access separately from long-term stability. Existing dual-handset success does not validate this unimplemented design.
