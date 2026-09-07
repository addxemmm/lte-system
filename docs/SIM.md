# SIM 写卡说明 SIM Programming Guide

## 0. 没有写卡器？先看这节 No Card Reader? Read This First

如果你手头已有写好的白卡（卡参数与 `user_db.csv` 对应），全程跳过 `/api/v1/simcards`：

If you already have a programmed test SIM (card parameters match `user_db.csv`), skip `/api/v1/simcards` entirely:

```csv
ue0,mil,001010123456789,00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8001,000000001234,7,dynamic
```

- 上面是 [`configs/user_db.csv.example`](../configs/user_db.csv.example) 首行的格式示例，首次 `/api/v1/cell` 时自动 seeding 到 `/data/conf/user_db.csv` / The line above shows the format of the first row in [`configs/user_db.csv.example`](../configs/user_db.csv.example), automatically seeded to `/data/conf/user_db.csv` on the first `/api/v1/cell`
- 无读卡器时调 `/api/v1/simcards` 固定返回 HTTP 503（`code: 50301`）（读卡器未连），**属正常现象，不是故障** / Calling `/api/v1/simcards` without a card reader always returns HTTP 503（`code: 50301`） (card reader not connected), **which is normal, not a fault**
- 直接按 [`docs/QUICKSTART.md`](QUICKSTART.md) 启动基站、终端入网即可；下面章节是有写卡器后才用的 / Just follow [`docs/QUICKSTART.md`](QUICKSTART.md) to start the eNodeB and attach the UE; the sections below only apply when you have a card reader

## 1. 旧硬编码问题 Legacy Hard-Coding Issues

旧实现写死 `Ki=00112233445566778899aabbccddeeff`、`OPc=63bfa50ee6523365ff14c1f45f88737d`、`ADM=3030303030303030`、`ICCID=89860123456789012345`、`card=testsim`，请求体仅收 `{"imsi":..}`。换卡/换网/换鉴权参数即需改代码，且部分卡 ICCID 出厂锁定，重写必败。

The legacy implementation hard-codes `Ki=00112233445566778899aabbccddeeff`, `OPc=63bfa50ee6523365ff14c1f45f88737d`, `ADM=3030303030303030`, `ICCID=89860123456789012345` and `card=testsim`, and the request body only accepts `{"imsi":..}`. Changing cards, networks, or auth parameters required code changes, and some cards have factory-locked ICCID so rewriting always fails.

## 2. 新灵活请求体 New Flexible Request Body

先停止小区；3.0 将订户与写卡变更和 Start 串行化，运行中返回 409，不自动停站。
Stop the cell first. Version 3.0 serializes subscriber/SIM changes with Start and returns 409 while running, without stopping the cell automatically.

除 `imsi(15位数字字符串)` 必填，其余全可选，未传取服务端默认（兼容旧值）。

`imsi` (15-digit numeric string) is required; all other fields are optional and fall back to server defaults (compatible with legacy values).

```bash
# 最小版（服务器端执行）
curl -X POST http://127.0.0.1:8081/api/v1/simcards -H 'Content-Type: application/json' -d '{"imsi":"001010123456780"}'
# 自定义 Ki/OPc 版
curl -X POST http://127.0.0.1:8081/api/v1/simcards -H 'Content-Type: application/json' -d '{"imsi":"001010123456781","ki":"00112233445566778899aabbccddeeff","opc":"63bfa50ee6523365ff14c1f45f88737d","op_type":"opc","auth":"mil","amf":"8000"}'
# 460 网络版
curl -X POST http://127.0.0.1:8081/api/v1/simcards -H 'Content-Type: application/json' -d '{"imsi":"460001234567890","mcc":"460","mnc":"00","spn":"CMCC","acc":"0200","card":"testsim"}'
# 3 位 MNC 版
curl -X POST http://127.0.0.1:8081/api/v1/simcards -H 'Content-Type: application/json' -d '{"imsi":"460031234567890","mcc":"460","mnc":"003","mnc3":true,"iccid":"auto","qci":7,"sqn":"000000001234"}'
```

`op`与`opc`互斥；`iccid:"auto"`表示不传`-s`保留出厂值；`mnc3:true`时从 IMSI 取 3 位 MNC。

`op` and `opc` are mutually exclusive; `iccid:"auto"` means omitting `-s` to keep the factory value; with `mnc3:true`, the 3-digit MNC is sliced from the IMSI.

## 3. user_db.csv 行格式 user_db.csv Row Format

`Name,Auth,IMSI,Key,OP_Type,OP/OPc,AMF,SQN,QCI,IP_alloc`，末尾留空行：

`Name,Auth,IMSI,Key,OP_Type,OP/OPc,AMF,SQN,QCI,IP_alloc`, with a trailing empty line:

```csv
ue2,mil,001010123456780,00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8000,0000000030c8,7,dynamic
```

`Name`唯一、`Auth`取`mil/xor`、`Key/OP`为 32 位 hex 且与卡一致、`AMF`为 AKA 认证管理字段，与接入控制类 `ACC` 不同、`QCI`一般 7、`IP_alloc`用`dynamic`。

`Name` must be unique, `Auth` is `mil/xor`, `Key/OP` is 32-bit hex matching the card, `AMF` is the AKA authentication management field, distinct from access-control class `ACC`, `QCI` is normally 7, and `IP_alloc` uses `dynamic`.

## 4. sim_profiles.yaml 卡型 Card Types in sim_profiles.yaml

```bash
grep -A3 'type:' configs/sim_profiles.yaml
```

`testsim`为本项目白卡（含 360 批次），`Ki`出厂预置不可改，写卡后须保证 csv 与出厂值一致；`sysmoUSIM-SJS1`可写 Ki/OPc、ICCID 可写；`sysmoISIM-SJA2`可写 Ki/OPc 但 `iccid_writable:false`，置`iccid:"auto"`，勿重写 ICCID。

`testsim` is the test SIM for this project (including the 360 batch) with factory-preset `Ki` that cannot be changed; after SIM programming, the csv must match the factory values. `sysmoUSIM-SJS1` allows writing Ki/OPc and ICCID; `sysmoISIM-SJA2` allows writing Ki/OPc but has `iccid_writable:false`, so set `iccid:"auto"` and do not rewrite the ICCID.

## 5. 写卡验证与排障（服务器端） Verify SIM Programming and Troubleshoot (Server Side)

流程：`pySim-prog.py`写卡 → `pySim-read.py -p 0`回读核对 IMSI → 检查订户是否已存在，否则追加 CSV：

Flow: write the card with `pySim-prog.py` → read it back with `pySim-read.py -p 0` to verify the IMSI → report whether the subscriber already exists, otherwise append to the csv:

```bash
sudo docker exec ltesystem bash -c "cd /opt/pysim && python3 pySim-read.py -p 0" | grep IMSI
sudo docker exec ltesystem tail -n 2 /data/conf/user_db.csv
```

ACR1281U 排障： / ACR1281U troubleshooting:

```bash
sudo docker exec ltesystem service pcscd restart
sudo docker exec ltesystem lsusb | grep -iE "acr128|072f"
sudo docker exec ltesystem timeout 6 pcsc_scan -n
```

读卡器检测三层递进（见 `internal/sim/sim.go:checkReader`）：
Reader detection runs in three layers (see `internal/sim/sim.go:checkReader`):
1. USB 层：`lsusb` 含 `ACR128` 或 ACS 厂商号 `072f` 即认定有读卡器；
   USB layer: an `ACR128` string or ACS vendor ID `072f` in `lsusb` means present;
2. PCSC 层：`pcsc_scan -n` 有 `Reader` 行即认定有读卡器，`Card inserted` 即有卡；
   PCSC layer: any `Reader` line means present, `Card inserted` means card;
3. 只有两层都不可用才用 pySim 直探，且**任何 Python 报错都判无设备**
   （以前 traceback 被误判成“卡未插入”，id 5 报成 id 2 的 bug 已修）。
   pySim direct probe is the last resort only, and **any Python error counts
   as no reader** (previously a traceback was misreported as id 5 instead of id 2).

标准接口通过 HTTP 状态码、`code` 与错误详情报告硬件问题，见 [API](API.md)。
The standard API reports hardware errors using HTTP status, `code` and details; see [API](API.md).

pysim 版本锁定：镜像用 2023-07-09 的 osmocom/pysim（`ARG PYSIM_COMMIT`，见
[`deploy/docker/Dockerfile`](../deploy/docker/Dockerfile)）+ 本仓
[`third_party/pysim/cards.py`](../third_party/pysim/cards.py) 覆盖的定制
`testsim` 卡逻辑。master 版 pysim 已重构，`pySim-prog.py` 参数和卡注册
方式都对不上，不要轻易升级。
pysim is pinned to 2023-07-09 (`ARG PYSIM_COMMIT` in the
[`Dockerfile`](../deploy/docker/Dockerfile)) with the custom `testsim`
logic overlaid from [`third_party/pysim/cards.py`](../third_party/pysim/cards.py).
Do not upgrade to pysim master: its rewritten CLI and card registry are
incompatible.

---
**导航 Navigation:** [文档索引 Docs](README.md) · [QUICKSTART](QUICKSTART.md) · [RULES](RULES.md) · [API v1](API.md) · [DEPLOY](DEPLOY.md) · [SIM](SIM.md) · [SDR](SDR.md) · [MIGRATION](MIGRATION.md)
