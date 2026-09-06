# samples 真机样本 Real-device Samples

真机样本（只读）。
Real-device samples (read-only).

- `epc-ue-attached.log` — 一次成功入网的完整 EPC 日志（151 行） / Complete EPC log of one successful attach (151 lines)
- `epc-ue-attached.log` 关键三连（`/basicinfo` 就是解析这三行）/ Key triple in `epc-ue-attached.log` (parsed by `/basicinfo`): `ESM Info: APN <你的APN>` → `Found User <IMSI>` → `get_new_ue_ipv4 pool ip addr <分配IP>`
- 调不通时拿你的实时日志和它逐段对照 / When attach fails, compare your live log against it section by section: `epc-ue-attached.log`
- `atr-new-card.txt` / `atr-old-card.txt` — 两批白卡的 ATR（`pcsc_scan` 输出），新批次已在 [`configs/sim_profiles.yaml`](../../configs/sim_profiles.yaml) 登记为 `testsim` 兼容卡 / ATRs (`pcsc_scan` output) of two white-card batches; the new batch is registered in [`configs/sim_profiles.yaml`](../../configs/sim_profiles.yaml) as `testsim` compatible card
- 有写卡器后写卡前先对 ATR / With a card writer, check the ATR before writing: `pcsc_scan`

---
**导航 Navigation:** [文档索引 Docs](../README.md) · [QUICKSTART](../QUICKSTART.md)
