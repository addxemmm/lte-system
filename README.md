# lte-system（Go + srsRAN_4G）

![CI](https://github.com/addxemmm/lte-system/actions/workflows/ci.yml/badge.svg)
![License](https://img.shields.io/badge/license-MIT-green)
![Go](https://img.shields.io/badge/go-1.22-blue)
![srsRAN](https://img.shields.io/badge/srsRAN_4G-release_23_11-orange)

无状态、无数据库的 LTE 自建基站工具：一个 Go 二进制暴露 HTTP API，编排容器内的 `srsepc` / `srsenb`（srsRAN_4G）、`tcpdump`、`tshark`、`hashcat` 与 ACR1281U 写卡。前端只调 API。

Stateless, database-free LTE self-hosted base station toolkit: a single Go binary exposing an HTTP API that orchestrates `srsepc`/`srsenb` (srsRAN_4G), `tcpdump`, `tshark`, `hashcat` and ACR1281U SIM programming inside one container. No database, no frontend; API only. Docs are in Chinese and English ([`docs/`](docs)); start with [`docs/QUICKSTART.md`](docs/QUICKSTART.md).

> 工作流：开发机上改代码，SDR 服务器上构建运行。排障命令默认在服务器执行（本文示例服务器为 `192.0.2.10`，按你的实际地址替换）。
>
> Workflow: edit code on the dev machine, build and run on the SDR server. Troubleshooting commands run on the server by default (example server `192.0.2.10` in this doc; replace with your actual address).

## 功能 Features

- 标准 REST：`/api/v1`（正确状态码 + `{"code","message","data","request_id"}` 包络 + OpenAPI，见 [`docs/API.md`](docs/API.md)）
  Standard REST: `/api/v1` (correct status codes + `{"code","message","data","request_id"}` envelope + OpenAPI, see [`docs/API.md`](docs/API.md))
- 9 个稳定的根路径工具接口（已冻结，见 [`docs/API_LEGACY.md`](docs/API_LEGACY.md)）
  9 stable root-path utility endpoints (frozen, see [`docs/API_LEGACY.md`](docs/API_LEGACY.md))
- 新增 `GET /healthz`、`GET /status`、`GET /profile`；`/start` 空 body 复用上次配置（持久化见 [`docs/RULES.md`](docs/RULES.md)）
  New `GET /healthz`, `GET /status` and `GET /profile`; `/start` with an empty body reuses the last saved configuration (persistence, see [`docs/RULES.md`](docs/RULES.md))
- 灵活写卡（有写卡器时）：`imsi` 必填，`ki/op/opc/auth/amf/acc/adm/spn/sqn/qci/card/mcc/mnc/iccid` 全可选（见 [`docs/SIM.md`](docs/SIM.md)）；无写卡器时该接口不可用，不影响入网
  Flexible SIM programming (when a card reader is present): `imsi` is required, `ki/op/opc/auth/amf/acc/adm/spn/sqn/qci/card/mcc/mnc/iccid` are all optional (see [`docs/SIM.md`](docs/SIM.md)); without a card reader this endpoint is unavailable, network attach is unaffected
- SDR：USRP B210（正版 + BlackSDR 兼容板 FPGA 可切换）与 bladeRF（见 [`docs/SDR.md`](docs/SDR.md)）
  SDR: USRP B210 (genuine + BlackSDR compat-board FPGA switchable) and bladeRF (see [`docs/SDR.md`](docs/SDR.md))
- srsRAN_4G `release_23_11`，Ubuntu 22.04 镜像
  srsRAN_4G `release_23_11`, Ubuntu 22.04 image

## 仓库布局 Repository Layout

```text
cmd/server            Go 入口
internal/api          v1 标准接口 + 旧版兼容 + 中间件（鉴权/审计/request-id）
postman/              Postman 集合（26 个请求 + 断言，开箱即测）
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
scripts/              smoke.sh、deploy 脚本 / smoke.sh and deploy scripts
third_party/pysim   定制 testsim 卡逻辑（GPL，随 era-pinned pysim 使用）/ custom testsim logic
```

## 快速开始（服务器端） Quick Start (Server Side)

```bash
cd ~/lte-system
sudo docker compose -f deploy/docker/docker-compose.yml up -d --build
curl -s -X POST http://127.0.0.1:8081/stop; echo
curl -s http://127.0.0.1:8081/healthz; echo
BASE=http://127.0.0.1:8081 bash scripts/smoke.sh
```

启动基站示例（mcc `001` mnc `01`，apn 与终端一致）：

Example cell startup (mcc `001`, mnc `01`; apn must match the UE/terminal device):

```bash
curl -X POST http://127.0.0.1:8081/start -H 'Content-Type: application/json' \
  -d '{"band":"7","apn":"srsapn","mcc":"001","mnc":"01","network":"eth0","sdr":"auto","full_net_name":"MyLTE","short_net_name":"MyLTE"}'
```

完整入网流程见 [`docs/QUICKSTART.md`](docs/QUICKSTART.md)。

See [`docs/QUICKSTART.md`](docs/QUICKSTART.md) for the full network attach flow.

本地编译验证（如 Windows PowerShell）：

Local build check (e.g. Windows PowerShell):

```powershell
go test ./...
$env:GOOS="linux"; $env:GOARCH="amd64"; go build -o bin/lte-system-linux-amd64 ./cmd/server
Remove-Item Env:\GOOS; Remove-Item Env:\GOARCH
```

## 文档 Documentation

- [docs/QUICKSTART.md](docs/QUICKSTART.md) — 无写卡器 + 已写卡，直接入网（先看这个） / No card reader + programmed SIM, direct network attach (start here)
- [docs/RULES.md](docs/RULES.md) — 使用规则：无状态定义、配置持久化、操作流、多终端、升级回滚、射频纪律 / Usage rules: stateless definition, config persistence, operation flow, multi-UE, upgrade and rollback, RF discipline
- [docs/API.md](docs/API.md) — v1 标准接口 + 旧版对照 / Standard v1 API + legacy mapping ([OpenAPI](docs/api/openapi.yaml))
- [docs/DEPLOY.md](docs/DEPLOY.md) — 服务器部署/升级/备份/排障 / Server deployment, upgrade, backup and troubleshooting
- [deploy/docker/README.md](deploy/docker/README.md) — Docker 专讲：镜像三段构建、compose 逐项解释、版本迭代、构建排障 / Docker deep dive
- [docs/SIM.md](docs/SIM.md) — 灵活写卡与卡型 / Flexible SIM programming and card types
- [docs/SDR.md](docs/SDR.md) — B210（兼容板 FPGA 切换）与 bladeRF / B210 (compat-board FPGA switching) and bladeRF
- [docs/MIGRATION.md](docs/MIGRATION.md) — 旧 Python → Go 对照 / Old Python vs Go mapping
- [AGENTS.md](AGENTS.md) — 本地开发规范（subagent / handoff / folk） / Local dev guide (subagent / handoff / folk)

## 参与贡献与许可 Contributing and License

- 想一起改？先看 [CONTRIBUTING.md](CONTRIBUTING.md)（测试要求、API 兼容铁律、PR 模板）
  Want to contribute? Read [CONTRIBUTING.md](CONTRIBUTING.md) first (test requirements, API compatibility rules, PR template)
- 安全问题走 [SECURITY.md](SECURITY.md) 私密通道；**API 无鉴权，仅限可信局域网**
  For security issues use the private channel in [SECURITY.md](SECURITY.md); **the API has no auth, trusted LAN only**
- 本仓库代码 MIT（[LICENSE](LICENSE)）；镜像内含 AGPL-3.0 的 srsRAN 等第三方组件，见 [NOTICE.md](NOTICE.md) / This repo's code is MIT ([LICENSE](LICENSE)); the image contains third-party components such as srsRAN under AGPL-3.0, see [NOTICE.md](NOTICE.md)
- 版本历史见 [CHANGELOG.md](CHANGELOG.md) / See [CHANGELOG.md](CHANGELOG.md) for version history
