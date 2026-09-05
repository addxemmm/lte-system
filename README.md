# lte-system（Go + srsRAN_4G）

无状态、无数据库的 LTE 自建基站工具：一个 Go 二进制暴露 HTTP API，编排容器内的 `srsepc` / `srsenb`（srsRAN_4G）、`tcpdump`、`tshark`、`hashcat` 与 ACR1281U 写卡。前端只调 API。

> 分工：**本地（Windows）只做代码编辑与 git 管理；构建、运行、射频验证一律在 Ubuntu 服务器（192.168.100.199）上执行。**
>
> 当前状态：**无写卡器，主白卡已写好**（ue3 / IMSI `001012333333333` / MCC `001` MNC `01`），开箱按 `docs/QUICKSTART.md` 直接入网，跳过 `/writesim`。

## 功能

- 9 个兼容旧 Flask 版的接口：`start / stop / basicinfo / crackapn / getcrackresult / userupload / passwordupload / getfile / writesim`（`message_id` 语义不变，见 `docs/API.md`）
- 新增 `GET /healthz`、`GET /status`
- 灵活写卡（有写卡器时）：`imsi` 必填，`ki/op/opc/auth/amf/acc/adm/spn/sqn/qci/card/mcc/mnc/iccid` 全可选（见 `docs/SIM.md`）；无写卡器时该接口不可用，不影响入网
- SDR：USRP B210（正版 + BlackSDR 兼容板 FPGA 可切换）与 bladeRF（见 `docs/SDR.md`）
- srsRAN_4G `release_23_11`，Ubuntu 22.04 镜像

## 仓库布局

```text
cmd/server            Go 入口
internal/api          9 接口 + healthz/status
internal/lte          srsRAN 启停 + conf 模板渲染 + band 表
internal/sdr          UHD/bladeRF/ACR1281 探测
internal/sim          灵活写卡 + user_db.csv
internal/crack        tshark CHAP + hashcat
internal/parser       EPC 日志解析
internal/sysop        无 shell 注入的进程管理
internal/config       env+yaml 配置
configs/              app.yaml.example、user_db.csv.example、sib/rr/drb、sim_profiles.yaml
deploy/docker/        Dockerfile、docker-compose.yml、entrypoint.sh、select-uhd-fpga.sh
firmware/uhd|bladerf  B210 FPGA（stock/compat）与 bladeRF 说明
docs/                 QUICKSTART / API / DEPLOY / SIM / SDR / MIGRATION + images/samples/legacy
scripts/              smoke.sh、deploy_to_ubuntu.sh
legacy-python-workspace/  旧 Python 实现（只读存档）
```

## 快速开始（服务器端）

```bash
cd ~/lte-system
sudo docker compose -f deploy/docker/docker-compose.yml up -d --build
curl -s -X POST http://127.0.0.1:8081/stop; echo
curl -s http://127.0.0.1:8081/healthz; echo
BASE=http://127.0.0.1:8081 bash scripts/smoke.sh
```

启动基站示例（主卡 ue3：mcc `001` mnc `01`，apn 与手机一致）：

```bash
curl -X POST http://127.0.0.1:8081/start -H 'Content-Type: application/json' \
  -d '{"band":"40","apn":"skygoapn","mcc":"001","mnc":"01","network":"eth0","sdr":"auto"}'
```

完整入网流程见 `docs/QUICKSTART.md`。

本地仅编译验证（Windows）：

```powershell
go test ./...
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o bin/lte-system-linux-amd64 ./cmd/server
Remove-Item Env:\GOOS; Remove-Item Env:\GOARCH
```

## 文档

- `docs/QUICKSTART.md` — 无写卡器 + 已写卡，直接入网（先看这个）
- `docs/API.md` — 接口与 `message_id` 全表 + curl
- `docs/DEPLOY.md` — 服务器部署/升级/备份/排障
- `docs/SIM.md` — 灵活写卡与卡型
- `docs/SDR.md` — B210（兼容板 FPGA 切换）与 bladeRF
- `docs/MIGRATION.md` — 旧 Python → Go 对照
- `AGENTS.md` — 本地开发规范（subagent / handoff / folk）
