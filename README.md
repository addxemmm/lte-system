# lte-system（Go + srsRAN_4G）

![CI](https://github.com/addxemmm/lte-system/actions/workflows/ci.yml/badge.svg)
![License](https://img.shields.io/badge/license-MIT-green)
![Go](https://img.shields.io/badge/go-1.22-blue)
![srsRAN](https://img.shields.io/badge/srsRAN_4G-release_23_11-orange)

> **English**: stateless LTE lab-cell toolkit — a single Go binary exposing an
> HTTP API that orchestrates `srsepc`/`srsenb` (srsRAN_4G), `tcpdump`, `tshark`,
> `hashcat` and ACR1281U SIM programming inside one container. No database,
> no frontend; API only. Docs are primarily in Chinese (`docs/`); start with
> `docs/QUICKSTART.md` (browser translate works fine).

无状态、无数据库的 LTE 自建基站工具：一个 Go 二进制暴露 HTTP API，编排容器内的 `srsepc` / `srsenb`（srsRAN_4G）、`tcpdump`、`tshark`、`hashcat` 与 ACR1281U 写卡。前端只调 API。

> 工作流：开发机上改代码，SDR 服务器上构建运行。排障命令默认在服务器执行（本文示例服务器为 `192.0.2.10`，按你的实际地址替换）。

## 功能

- 9 个稳定的工具接口：`start / stop / basicinfo / crackapn / getcrackresult / userupload / passwordupload / getfile / writesim`（响应语义见 `docs/API.md`）
- 新增 `GET /healthz`、`GET /status`、`GET /profile`；`/start` 空 body 复用上次配置（持久化见 `docs/RULES.md`）
- 灵活写卡（有写卡器时）：`imsi` 必填，`ki/op/opc/auth/amf/acc/adm/spn/sqn/qci/card/mcc/mnc/iccid` 全可选（见 `docs/SIM.md`）；无写卡器时该接口不可用，不影响入网
- SDR：USRP B210（正版 + BlackSDR 兼容板 FPGA 可切换）与 bladeRF（见 `docs/SDR.md`）
- srsRAN_4G `release_23_11`，Ubuntu 22.04 镜像

## 仓库布局

```text
cmd/server            Go 入口
internal/api          9 接口 + healthz/status/profile
internal/lte          srsRAN 启停 + conf 模板渲染 + band 表
internal/sdr          UHD/bladeRF/ACR1281 探测
internal/sim          灵活写卡 + user_db.csv
internal/crack        tshark CHAP + hashcat
internal/parser       EPC 日志解析
internal/sysop        无 shell 注入的进程管理
internal/config       env+yaml 配置
configs/              app.yaml.example、user_db.csv.example、sib/rr/rb、sim_profiles.yaml
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

启动基站示例（mcc `001` mnc `01`，apn 与终端一致）：

```bash
curl -X POST http://127.0.0.1:8081/start -H 'Content-Type: application/json' \
  -d '{"band":"7","apn":"srsapn","mcc":"001","mnc":"01","network":"eth0","sdr":"auto","full_net_name":"MyLTE","short_net_name":"MyLTE"}'
```

完整入网流程见 `docs/QUICKSTART.md`。

本地编译验证（如 Windows PowerShell）：

```powershell
go test ./...
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o bin/lte-system-linux-amd64 ./cmd/server
Remove-Item Env:\GOOS; Remove-Item Env:\GOARCH
```

## 文档

- `docs/QUICKSTART.md` — 无写卡器 + 已写卡，直接入网（先看这个）
- `docs/RULES.md` — 使用规则：无状态定义、配置持久化、操作流、多终端、升级回滚、射频纪律
- `docs/API.md` — 接口与 `message_id` 全表 + curl
- `docs/DEPLOY.md` — 服务器部署/升级/备份/排障
- `deploy/docker/README.md` — Docker 专讲：镜像三段构建、compose 逐项解释、版本迭代、构建排障
- `docs/SIM.md` — 灵活写卡与卡型
- `docs/SDR.md` — B210（兼容板 FPGA 切换）与 bladeRF
- `docs/MIGRATION.md` — 旧 Python → Go 对照
- `AGENTS.md` — 本地开发规范（subagent / handoff / folk）

## 参与贡献与许可

- 想一起改？先看 `CONTRIBUTING.md`（测试要求、API 兼容铁律、PR 模板）
- 安全问题走 `SECURITY.md` 私密通道；**API 无鉴权，仅限可信局域网**
- 本仓库代码 MIT（`LICENSE`）；镜像内含 AGPL-3.0 的 srsRAN 等第三方组件，见 `NOTICE.md`
- 版本历史见 `CHANGELOG.md`
