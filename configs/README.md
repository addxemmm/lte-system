# user_db.csv 用户数据库 User Database

HSS 用户数据库（srsRAN EPC 用）。
HSS user database (for srsRAN EPC).

格式（srsRAN 原生，`#` 开头为注释）：
Native srsRAN format (lines starting with `#` are comments):

```csv
Name,Auth,IMSI,Key,OP_Type,OP/OPc,AMF,SQN,QCI,IP_alloc
```

## 示例卡与种子文件 Example Cards and Seed File

种子文件自带多个测试用户（如 `ue0/ue1/ue3…`，含常用测试 IMSI），首发开箱即用：
The seed file ships multiple test users (e.g. `ue0/ue1/ue3…` with common test IMSIs), ready out of the box:

```csv
ue3,mil,001012333333333,00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8001,000000001234,7,dynamic
```

- IMSI 前 5 位即 MCC/MNC（如 `00101` → `/start` 填 `mcc=001 mnc=01`）/ First 5 digits of IMSI are MCC/MNC (e.g. `00101` → `/start` with `mcc=001 mnc=01`)
- `Key`/`OPc` 必须与卡内预置完全一致，否则鉴权失败（表现为 attach 后 `UE Authentication Rejected`） / `Key`/`OPc` must exactly match the card preset, otherwise authentication fails (attach then `UE Authentication Rejected`)
- `SQN` 掉线重连失败时可递增一位后重启 EPC 再试 / If reconnect fails after drop, increment `SQN` by one and restart the EPC, then retry

## 加卡 Adding Cards

加卡。
Adding cards.

- 有写卡器：调 `/writesim`（自动追加，见 [`docs/SIM.md`](../docs/SIM.md)） / With a card writer: call `/writesim` (auto-append, see [`docs/SIM.md`](../docs/SIM.md))
- 无写卡器：按上面格式手写一行 → 调 `/userupload` 上传，或直接替换服务器 `/data/conf/user_db.csv` 后重启容器 / Without a writer: hand-write one line in the format above → upload via `/userupload`, or replace `/data/conf/user_db.csv` on the server and restart the container
- `Name` 必须唯一；末尾保留一个空行（srsRAN 解析要求） / `Name` must be unique; keep one trailing empty line (required by srsRAN parsing)

## 相关文件 Related Files

相关文件。
Related files.

- [`configs/user_db.csv.example`](user_db.csv.example) — 带注释的多用户种子文件 / Commented multi-user seed file
- [`configs/app.yaml.example`](app.yaml.example) 的 `sim_defaults` — `/writesim` 缺省值，与示例卡参数对齐 / `sim_defaults` in [`configs/app.yaml.example`](app.yaml.example) — defaults for `/writesim`, aligned with the example card
- `sib.conf` / `rb.conf` — srsRAN_4G 小区静态配置（`rb.conf` 即原 srsLTE 的 `drb.conf`，上游改名；`qci 7/9` 承载与旧版一致），跟随镜像 / srsRAN_4G static cell config (`rb.conf` is the former srsLTE `drb.conf`, renamed upstream; `qci 7/9` bearers same as before), shipped with the image
- `rr.conf` — 参考渲染文件；live 版本由 [`internal/lte`](../internal/lte) 在每次 `/start` 时按频段渲染（含显式 `ul_earfcn`，模板见 [`internal/lte/manager.go`](../internal/lte/manager.go)） / Reference rendered file; the live version is rendered per band by [`internal/lte`](../internal/lte) on each `/start` (with explicit `ul_earfcn`, template in [`internal/lte/manager.go`](../internal/lte/manager.go))
- [`configs/sim_profiles.yaml`](sim_profiles.yaml) — 卡型说明 / Card-type descriptions

---
**导航 Navigation:** [仓库根 Repo Root](../README.md) · [SIM](../docs/SIM.md) · [QUICKSTART](../docs/QUICKSTART.md)
