# third_party — 第三方 vendoring 第三方文件 / Vendored Third-Party Files

## pysim/cards.py — 定制白卡逻辑 Custom test-SIM logic

- 来源 Provenance：2023 年线上写卡环境的实测文件，从服务器
  `~/lte-system/legacy-python-workspace/pysim/cards.py` 抢救回来
  (rescued from the 2023 working system; never tracked in git — it
  overlaid an unversioned submodule checkout).
- 许可 License：**GPL-2.0**（文件头保留原作者版权
  Sylvain Munaut / Harald Welte and contributors）。
- 作用 Purpose：在 era-pinned pysim 上提供 `testsim` 卡型（含 360 批次
  白卡 ATR），见 `configs/sim_profiles.yaml`。
  Provides the `testsim` card type on the pinned pysim.
- 配对版本 Pinned against：osmocom/pysim
  `7d13845285ccdd2f8a310c19bdbeb685fd1a205e`（2023-08，
  `pySim/legacy/` 布局 + `pySim-prog.py` 支持 `-t/-A/--acc/-o/-k`），
  由 `Dockerfile` 覆盖到 `/opt/pysim/pySim/legacy/cards.py`。
  Overlaid onto `/opt/pysim/pySim/legacy/cards.py` by the `Dockerfile`.
- 校验 Verify：`sha256sum third_party/pysim/cards.py` 应为
  `51f103e98b002f9ba44320593eac6b1063c9e8657ee67505f79cb73d239415ca`。
