# SIM 写卡说明

## 1. 旧硬编码问题

旧实现写死 `Ki=00112233445566778899aabbccddeeff`、`OPc=63bfa50ee6523365ff14c1f45f88737d`、`ADM=3030303030303030`、`ICCID=89860123456789012345`、`card=testsim`，请求体仅收 `{"imsi":..}`。换卡/换网/换鉴权参数即需改代码，且部分卡 ICCID 出厂锁定，重写必败。

## 2. 新灵活请求体

除 `imsi(15位数字字符串)` 必填，其余全可选，未传取服务端默认（兼容旧值）。

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

## 3. user_db.csv 行格式

`Name,Auth,IMSI,Key,OP_Type,OP/OPc,AMF,SQN,QCI,IP_alloc`，末尾留空行：

```csv
ue2,mil,001010123456780,00112233445566778899aabbccddeeff,opc,63bfa50ee6523365ff14c1f45f88737d,8000,0000000030c8,7,dynamic
```

`Name`唯一、`Auth`取`mil/xor`、`Key/OP`为 32 位 hex 且与卡一致、`AMF`与卡 ACC 对应、`QCI`一般 7、`IP_alloc`用`dynamic`。

## 4. sim_profiles.yaml 卡型

```bash
grep -A3 'type:' configs/sim_profiles.yaml
```

`testsim`为本项目白卡（含 360 批次），`Ki`出厂预置不可改，写卡后须保证 csv 与出厂值一致；`sysmoUSIM-SJS1`可写 Ki/OPc、ICCID 可写；`sysmoISIM-SJA2`可写 Ki/OPc 但 `iccid_writable:false`，置`iccid:"auto"`，勿重写 ICCID。

## 5. 写卡验证与排障（服务器端）

流程：`pySim-prog.py`写卡 → `pySim-read.py -p 0`回读核对 IMSI → 存在则报 4，不存在追加 csv：

```bash
sudo docker exec ltesystem python3 /opt/pysim/pySim-read.py -p 0 | grep IMSI
sudo docker exec ltesystem tail -n 2 /data/conf/user_db.csv
```

ACR1281U 排障：

```bash
sudo docker exec ltesystem service pcscd restart
sudo docker exec ltesystem lsusb | grep -i acr128
sudo docker exec ltesystem pcsc_scan
```

无 `ACR128`先查 USB 映射与供电，有设备无卡则报 5，无读卡器报 2。
