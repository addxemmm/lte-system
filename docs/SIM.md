# SIM 写卡说明 SIM Programming Guide

## 0. 没有写卡器？先看这节 No Card Reader? Read This First

如果你手头已有写好的白卡（卡参数与 `user_db.csv` 对应），全程跳过 `/writesim`：

If you already have a programmed test SIM (card parameters match `user_db.csv`), skip `/writesim` entirely:

```csv
ue0,mil,001010123456789,00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8001,000000001234,7,dynamic
```

- 上面是 [`configs/user_db.csv.example`](../configs/user_db.csv.example) 首行的格式示例，首次 `/start` 时自动 seeding 到 `/data/conf/user_db.csv` / The line above shows the format of the first row in [`configs/user_db.csv.example`](../configs/user_db.csv.example), automatically seeded to `/data/conf/user_db.csv` on the first `/start`
- 无读卡器时调 `/writesim` 固定返回 `message_id 2`（读卡器未连），**属正常现象，不是故障** / Calling `/writesim` without a card reader always returns `message_id 2` (card reader not connected), **which is normal, not a fault**
- 直接按 [`docs/QUICKSTART.md`](QUICKSTART.md) 启动基站、终端入网即可；下面章节是有写卡器后才用的 / Just follow [`docs/QUICKSTART.md`](QUICKSTART.md) to start the eNodeB and attach the UE; the sections below only apply when you have a card reader

## 1. 旧硬编码问题 Legacy Hard-Coding Issues

旧实现写死 `Ki=00112233445566778899aabbccddeeff`、`OPc=63bfa50ee6523365ff14c1f45f88737d`、`ADM=3030303030303030`、`ICCID=89860123456789012345`、`card=testsim`，请求体仅收 `{"imsi":..}`。换卡/换网/换鉴权参数即需改代码，且部分卡 ICCID 出厂锁定，重写必败。

The legacy implementation hard-codes `Ki=00112233445566778899aabbccddeeff`, `OPc=63bfa50ee6523365ff14c1f45f88737d`, `ADM=3030303030303030`, `ICCID=89860123456789012345` and `card=testsim`, and the request body only accepts `{"imsi":..}`. Changing cards, networks, or auth parameters required code changes, and some cards have factory-locked ICCID so rewriting always fails.

## 2. 新灵活请求体 New Flexible Request Body

除 `imsi(15位数字字符串)` 必填，其余全可选，未传取服务端默认（兼容旧值）。

`imsi` (15-digit numeric string) is required; all other fields are optional and fall back to server defaults (compatible with legacy values).

```bash
# 最小版（服务器端执行）
curl -X POST http://127.0.0.1:8081/writesim -H 'Content-Type: application/json' -d '{"imsi":"001010123456780"}'
# 自定义 Ki/OPc 版
curl -X POST http://127.0.0.1:8081/writesim -H 'Content-Type: application/json' -d '{"imsi":"001010123456781","ki":"00112233445566778899aabbccddeeff","opc":"63bfa50ee6523365ff14c1f45f88737d","op_type":"opc","auth":"mil","amf":"8000"}'
# 460 网络版
curl -X POST http://127.0.0.1:8081/writesim -H 'Content-Type: application/json' -d '{"imsi":"460001234567890","mcc":"460","mnc":"00","spn":"CMCC","acc":"0200","card":"testsim"}'
# 3 位 MNC 版
curl -X POST http://127.0.0.1:8081/writesim -H 'Content-Type: application/json' -d '{"imsi":"460031234567890","mcc":"460","mnc":"003","mnc3":true,"iccid":"auto","qci":7,"sqn":"000000001234"}'
```

`op`与`opc`互斥；`iccid:"auto"`表示不传`-s`保留出厂值；`mnc3:true`时从 IMSI 取 3 位 MNC。

`op` and `opc` are mutually exclusive; `iccid:"auto"` means omitting `-s` to keep the factory value; with `mnc3:true`, the 3-digit MNC is sliced from the IMSI.

## 3. user_db.csv 行格式 user_db.csv Row Format

`Name,Auth,IMSI,Key,OP_Type,OP/OPc,AMF,SQN,QCI,IP_alloc`，末尾留空行：

`Name,Auth,IMSI,Key,OP_Type,OP/OPc,AMF,SQN,QCI,IP_alloc`, with a trailing empty line:

```csv
ue2,mil,001010123456780,00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8000,0000000030c8,7,dynamic
```

`Name`唯一、`Auth`取`mil/xor`、`Key/OP`为 32 位 hex 且与卡一致、`AMF`与卡 ACC 对应、`QCI`一般 7、`IP_alloc`用`dynamic`。

`Name` must be unique, `Auth` is `mil/xor`, `Key/OP` is 32-bit hex matching the card, `AMF` corresponds to the card ACC, `QCI` is normally 7, and `IP_alloc` uses `dynamic`.

## 4. sim_profiles.yaml 卡型 Card Types in sim_profiles.yaml

```bash
grep -A3 'type:' configs/sim_profiles.yaml
```

`testsim`为本项目白卡（含 360 批次），`Ki`出厂预置不可改，写卡后须保证 csv 与出厂值一致；`sysmoUSIM-SJS1`可写 Ki/OPc、ICCID 可写；`sysmoISIM-SJA2`可写 Ki/OPc 但 `iccid_writable:false`，置`iccid:"auto"`，勿重写 ICCID。

`testsim` is the test SIM for this project (including the 360 batch) with factory-preset `Ki` that cannot be changed; after SIM programming, the csv must match the factory values. `sysmoUSIM-SJS1` allows writing Ki/OPc and ICCID; `sysmoISIM-SJA2` allows writing Ki/OPc but has `iccid_writable:false`, so set `iccid:"auto"` and do not rewrite the ICCID.

## 5. 写卡验证与排障（服务器端） Verify SIM Programming and Troubleshoot (Server Side)

流程：`pySim-prog.py`写卡 → `pySim-read.py -p 0`回读核对 IMSI → 存在则报 4，不存在追加 csv：

Flow: write the card with `pySim-prog.py` → read it back with `pySim-read.py -p 0` to verify the IMSI → return 4 if it already exists, otherwise append to the csv:

```bash
sudo docker exec ltesystem python3 /opt/pysim/pySim-read.py -p 0 | grep IMSI
sudo docker exec ltesystem tail -n 2 /data/conf/user_db.csv
```

ACR1281U 排障： / ACR1281U troubleshooting:

```bash
sudo docker exec ltesystem service pcscd restart
sudo docker exec ltesystem lsusb | grep -i acr128
sudo docker exec ltesystem pcsc_scan
```

无 `ACR128`先查 USB 映射与供电，有设备无卡则报 5，无读卡器报 2。

Without `ACR128`, check the USB mapping and power first; a device with no card returns 5, and no card reader returns 2.

---
**导航 Navigation:** [文档索引 Docs](README.md) · [QUICKSTART](QUICKSTART.md) · [RULES](RULES.md) · [API v1](API.md) · [旧版API Legacy](API_LEGACY.md) · [DEPLOY](DEPLOY.md) · [SIM](SIM.md) · [SDR](SDR.md) · [MIGRATION](MIGRATION.md)
