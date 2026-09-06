# legacy-python-workspace — v1.x 实现（只读存档）

早期手工版本：HTTP 接口服务（9 接口）+ `run.sh`/`stop.sh`（配旧版 srsLTE）+ `pysim/cards.py`（`testsim` 卡逻辑）+ `conf/user_db.csv`（原始多用户种子）。

- 已删除：运行日志/pcap（体积大，代表性样本见 `docs/samples/epc-ue-attached.log`）、`__pycache__`、crash 与生成的 `*_run.conf`
- 现役 Go 实现见 `cmd/` + `internal/`，对照表见 `docs/MIGRATION.md`
- 不要在此目录上继续开发；有价值的逻辑先搬到 `internal/` 并补单测
