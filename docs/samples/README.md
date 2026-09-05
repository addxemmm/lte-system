# samples — 真机样本（只读）

- `epc-ue-attached.log` — ue3（IMSI `001012333333333`）一次成功入网的完整 EPC 日志（151 行）。
  关键三连（`/basicinfo` 就是解析这三行）：
  `ESM Info: APN skygoapn` → `Found User 001012333333333` → `get_new_ue_ipv4 pool ip addr 172.16.0.2`。
  （历史样本，当时 APN 为 `skygoapn`；现网 APN 已改为 `addxLTE`，三连结构不变。）
  调不通时拿你的实时日志和它逐段对照。
- `atr-new-card.txt` / `atr-old-card.txt` — 两批白卡的 ATR（`pcsc_scan` 输出），新批次已在 `configs/sim_profiles.yaml` 登记为 `testsim` 兼容卡。有写卡器后写卡前先对 ATR。
