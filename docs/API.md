# LTE-System API（Go `:8081`）

约定：9 个旧接口全部 `POST JSON`，统一响应 `{"status":bool,"message_id":int,"message":str,...}`，语义沿用旧 README。新增 `GET /healthz`、`GET /status` 无 `message_id`。

## 1. /start 启动抓包

`band` 仅限 `1/3/5/7/8/34/39/40/41`，越界按默认处理。

```bash
curl -X POST http://127.0.0.1:8081/start -H 'Content-Type: application/json' -d '{"band":"40","apn":"addxLTE","mcc":"001","mnc":"01","network":"eth0"}'
# 成功: {"status":true,"message_id":1,"message":"Start successfully"}
```

| id | message | 说明 |
|---|---|---|
|1|Start successfully|成功|
|2|is running|已在运行|
|3|Incomplete parameters|缺 band/apn/mcc/mnc/network|
|4|device is not connected...|未检测到 USRP|
|0|Start Failed...|其他失败，看日志|

新增可选：`sdr/device_args/tx_gain/rx_gain/n_prb/full_net_name/short_net_name`，不传沿用服务端默认：

```bash
curl -X POST http://127.0.0.1:8081/start -H 'Content-Type: application/json' -d '{"band":"41","apn":"addxLTE","mcc":"460","mnc":"00","network":"eth0","sdr":"uhd","tx_gain":80,"rx_gain":40,"n_prb":25,"full_net_name":"addxLTE","short_net_name":"addxLTE"}'
```

`full_net_name`/`short_net_name` 是手机上显示的运营商名（NITZ 下发，1–32 字符，默认 `srsRAN`）；`/status` 会回显本次生效的 `net_name`。

## 2. /stop /basicinfo /crackapn /getcrackresult

```bash
curl -X POST http://127.0.0.1:8081/stop -d '{}'
# {"status":true,"message_id":1,"message":"Stop successfully."} | 2 Not running. | 0 Stop failed.
curl -X POST http://127.0.0.1:8081/basicinfo -d '{}'
# {"status":true,"message_id":1,"message":"Getting information success.","apn":"addxLTE","imsi":"001012333333333","ip":"172.16.0.2"}
curl -X POST http://127.0.0.1:8081/crackapn -d '{}'
curl -X POST http://127.0.0.1:8081/getcrackresult -d '{}'
# 成功追加 {"apn":..,"imsi":..,"ip":..,"username":"mi6test","password":"cmwap"}
```

`basicinfo: 2`未运行/`3`无终端；`crackapn: 2` hashcat运行中/`3`无UE数据/`4`无CHAP/`5`停服失败；`getcrackresult: 2`破解中/`3`字典无密码/`4`无CHAP/`5`无UE数据。

## 3. 上传 / 下载

```bash
curl -X POST http://127.0.0.1:8081/userupload -F userdb=@user_db.csv
curl -X POST http://127.0.0.1:8081/passwordupload -F wordlist=@wordlist.list
# {"status":true,"message_id":1,"message":"upload success"} | 2 no file | 0 upload failed
curl -X POST http://127.0.0.1:8081/getfile -H 'Content-Type: application/json' -d '{"fileid":0}' -OJ
# 失败: {"status":false,"message_id":2,"message":"Error id"} | 3 Cant not find the file
```

fileid 映射：

| fileid | filename | 说明 |
|---|---|---|
|0|`lte_data.pcap`|SGi 业务流量|
|1|`srsLTE_enb_s1ap.pcap`|eNB S1AP|
|2|`srsLTE_enb.pcap`|eNB 空口|
|3|`srsLTE_epc.pcap`|EPC|

## 4. /writesim 写卡（有写卡器时才用）

> 当前无写卡器：该接口固定返回 `message_id 2`，直接跳过，用 `QUICKSTART.md` 流程入网即可。

```bash
curl -X POST http://127.0.0.1:8081/writesim -H 'Content-Type: application/json' -d '{"imsi":"001010123456780"}'
# 1 Succeed. | 2 读卡器未连 | 4 卡已存在 | 5 未插卡 | 3 写卡成但csv失败 | 6 参数非法 | 0 Failed.
```

新增全可选：`ki/op/opc/op_type/auth/amf/acc/adm/spn/name/sqn/qci/card/mcc/mnc/iccid`，详见 `SIM.md`。

## 5. 新增状态接口 / 错误速查

```bash
curl http://127.0.0.1:8081/healthz; curl http://127.0.0.1:8081/status
# {"ok":true,"running":true,"sdr":{...}} | {"running":true,"epc":true,"enb":true,"band":"40","apn":"addxLTE"}
```

速查：`0`均为需查日志的通用失败；`2`多为“状态冲突”（已运行/未运行/id错/无文件/无读卡器）；`3`多为“缺数据”（缺参/无UE/无文件/csv失败）。
