# user_db.csv — HSS 用户数据库（srsRAN EPC 用）

格式（srsRAN 原生，`#` 开头为注释）：

```csv
Name,Auth,IMSI,Key,OP_Type,OP/OPc,AMF,SQN,QCI,IP_alloc
```

## 当前主卡（已写好、无需写卡器）

```csv
ue3,mil,001012333333333,00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8001,000000001234,7,dynamic
```

- IMSI `001012333333333` → MCC `001`、MNC `01`，`/start` 时就填 `mcc=001 mnc=01`
- `Key`/`OPc` 必须与卡内预置完全一致，否则鉴权失败（表现为 attach 后 `UE Authentication Rejected`）
- `SQN` 掉线重连失败时可递增一位后重启 EPC 再试

## 加卡

- 有写卡器：调 `/writesim`（自动追加，见 `docs/SIM.md`）
- 无写卡器：按上面格式手写一行 → 调 `/userupload` 上传，或直接替换服务器 `/data/conf/user_db.csv` 后重启容器
- `Name` 必须唯一；末尾保留一个空行（srsRAN 解析要求）

## 相关文件

- `configs/user_db.csv.example` — 带注释的种子文件，首行即主卡 ue3
- `configs/app.yaml.example` 的 `sim_defaults` — `/writesim` 缺省值，与主卡参数对齐
- `sib.conf` / `rr.conf` / `rb.conf` — srsRAN_4G 小区静态配置（`rb.conf` 即原 srsLTE 的 `drb.conf`，上游改名；`qci 7/9` 承载与旧版一致）
- `configs/sim_profiles.yaml` — 卡型说明
