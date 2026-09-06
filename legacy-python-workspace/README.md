# legacy-python-workspace 旧版存档 Legacy Python Workspace

v1.x 实现（只读存档saved profile）。
v1.x implementation (read-only saved profile).

早期手工版本：HTTP 接口服务（9 接口）+ `run.sh`/`stop.sh`（配旧版 srsLTE）+ `pysim/cards.py`（`testsim` 卡逻辑）+ `conf/user_db.csv`（原始多用户种子seed）。
Early manual version: HTTP API service (9 APIs) + `run.sh`/`stop.sh` (for old srsLTE) + `pysim/cards.py` (`testsim` card logic) + `conf/user_db.csv` (original multi-user seed).

- 已删除：运行日志/pcap（体积大，代表性样本见 `docs/samples/epc-ue-attached.log`）、`__pycache__`、crash 与生成的 `*_run.conf` / Removed: runtime logs/pcap (large; representative sample in `docs/samples/epc-ue-attached.log`), `__pycache__`, crash dumps and generated `*_run.conf`
- 现役 Go 实现见 `cmd/` + `internal/`，对照表见 `docs/MIGRATION.md` / Active Go implementation in `cmd/` + `internal/`; mapping in `docs/MIGRATION.md`
- 不要在此目录上继续开发；有价值的逻辑先搬到 `internal/` 并补单测 / Do not continue development here; move valuable logic to `internal/` first with unit tests

---
**导航 Navigation:** [仓库根 Repo Root](../README.md) · [MIGRATION](../docs/MIGRATION.md)
