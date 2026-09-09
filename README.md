# lte-system（Go + srsRAN_4G）

![CI](https://github.com/addxemmm/lte-system/actions/workflows/ci.yml/badge.svg)
![License](https://img.shields.io/badge/license-MIT-green)
![Go](https://img.shields.io/badge/go-1.22-blue)
![srsRAN](https://img.shields.io/badge/srsRAN_4G-release_23_11-orange)

无状态、无数据库的 LTE 自建基站工具：一个 Go 二进制内嵌 Web 控制台与 HTTP API，编排容器内的 `srsepc` / `srsenb`（srsRAN_4G）、`tcpdump`、`tshark`、`hashcat` 与 ACR1281U 写卡。无需 Node/nginx runtime。

Stateless, database-free LTE self-hosted base station toolkit: one Go binary embeds the Web console and HTTP API while orchestrating `srsepc`/`srsenb` (srsRAN_4G), `tcpdump`, `tshark`, `hashcat` and ACR1281U SIM programming in one container. No Node/nginx runtime. Docs are bilingual ([`docs/`](docs)); start with [`docs/WEB_UI.md`](docs/WEB_UI.md) and [`docs/QUICKSTART.md`](docs/QUICKSTART.md).

> 工作流：开发机上改代码，SDR 服务器上构建运行。排障命令默认在服务器执行（本文示例服务器为 `192.0.2.10`，按你的实际地址替换）。
>
> Workflow: edit code on the dev machine, build and run on the SDR server. Troubleshooting commands run on the server by default (example server `192.0.2.10` in this doc; replace with your actual address).

## 版本与发布 Versioning and releases

当前版本仍为 **2.1**。2026-09-09 管理台测试验收后已按要求停止 LTE 容器，保留镜像和数据；详见 [验收与停机记录](docs/WEB_UI_RELEASE_2026-09-09.md)。[变更记录](CHANGELOG.md)、[发布管理方案](docs/RELEASING.md) 与 [双语 Release 模板](docs/releases/TEMPLATE.md) 区分源码提交、测试部署和正式镜像发布。Docker Hub 自动同步尚未启用，普通 Git push 不会上传镜像或部署服务器。

The current version remains **2.1**. Following console acceptance on 2026-09-09, the LTE container was stopped as requested, retaining its image and data; see the [acceptance/shutdown record](docs/WEB_UI_RELEASE_2026-09-09.md). The [changelog](CHANGELOG.md), [release plan](docs/RELEASING.md) and [bilingual Release template](docs/releases/TEMPLATE.md) distinguish source commits, test deployments and official image publication. Docker Hub automation is not enabled; an ordinary Git push neither uploads an image nor deploys the server.

## 功能 Features

- 内嵌 Web 控制台默认监听 8080，提供非敏感 `/ui-config.json` 和同源管理网关；网关只开放管理只读与小区启停，不代理 crack/SIM/upload/capture。完整独立 API `:8081` 默认关闭，测试/旧客户端须显式启用。这是部署默认值的 breaking change，详见 [Web UI](docs/WEB_UI.md)。
  Embedded Web console on port 8080 with non-sensitive `/ui-config.json` and a same-origin management gateway. The gateway permits only management reads and cell start/stop, not crack/SIM/upload/capture. The direct full API on `:8081` is disabled by default and must be enabled explicitly for tests/legacy clients. This is a breaking deployment default; see [Web UI](docs/WEB_UI.md).
- 可选固定 API Token：私有 YAML 的 `api_token` 非空时启用 Bearer 鉴权，空值开放访问；兼容非空 `LTE_API_TOKEN` 覆盖，版本保持 2.1。配置与 Postman 用法见 [API 文档](docs/API.md)。
  Optional static API token: a nonempty YAML `api_token` enables Bearer authentication; empty allows anonymous access. Nonempty `LTE_API_TOKEN` remains an override. Version stays 2.1; see the API guide for configuration and Postman usage.
- 标准 REST：`/api/v1`（正确状态码 + `{"code","message","data","request_id"}` 包络 + OpenAPI，见 [`docs/API.md`](docs/API.md)）
  Standard REST: `/api/v1` (correct status codes + `{"code","message","data","request_id"}` envelope + OpenAPI, see [`docs/API.md`](docs/API.md))
- 多 UE 会话集合、订户管理、APN 校验和 UE 网络策略；旧根路径接口与单条 `/api/v1/ue` 已移除。
  Multi-UE sessions, subscriber management, APN validation and UE network policies; legacy root routes and singleton `/api/v1/ue` are removed.
- `GET /api/v1/cell`、`GET /api/v1/profile`；`POST /api/v1/cell` 的 `{}` 请求复用上次配置（见 [`docs/RULES.md`](docs/RULES.md)）。
  `GET /api/v1/cell`, `GET /api/v1/profile`; `POST /api/v1/cell` with `{}` reuses the saved profile (see [`docs/RULES.md`](docs/RULES.md)).
- 灵活写卡（有写卡器时）：`imsi` 必填，`ki/op/opc/auth/amf/acc/adm/spn/sqn/qci/card/mcc/mnc/iccid` 全可选（见 [`docs/SIM.md`](docs/SIM.md)）；无写卡器时该接口不可用，不影响入网
  Flexible SIM programming (when a card reader is present): `imsi` is required, `ki/op/opc/auth/amf/acc/adm/spn/sqn/qci/card/mcc/mnc/iccid` are all optional (see [`docs/SIM.md`](docs/SIM.md)); without a card reader this endpoint is unavailable, network attach is unaffected
- SDR：USRP B210（正版 + BlackSDR 兼容板 FPGA 可切换）与 bladeRF（见 [`docs/SDR.md`](docs/SDR.md)）
  SDR: USRP B210 (genuine + BlackSDR compat-board FPGA switchable) and bladeRF (see [`docs/SDR.md`](docs/SDR.md))
- srsRAN_4G `release_23_11`，Ubuntu 22.04 镜像
  srsRAN_4G `release_23_11`, Ubuntu 22.04 image

## 仓库布局 Repository Layout

```text
cmd/server            Go 入口
internal/api          v1 标准接口 + 中间件（鉴权/审计/request-id）
internal/webui        go:embed 控制台 + 有限同源管理网关
postman/              仅标准接口 Postman 集合 + 断言
internal/lte          srsRAN 启停 + conf 模板渲染 + band 表
internal/sdr          UHD/bladeRF/ACR1281 探测
internal/sim          灵活写卡 + user_db.csv
internal/crack        tshark CHAP + hashcat
internal/parser       EPC 日志解析
internal/sysop        无 shell 注入的进程管理
internal/config       env+yaml 配置
configs/              app.yaml.example、user_db.csv.example、sib/rr/rb、sim_profiles.yaml
deploy/docker/        完整/增量 Dockerfile、bridge/host/test Compose、entrypoint
firmware/uhd|bladerf  B210 FPGA（stock/compat）与 bladeRF 说明
docs/                 QUICKSTART / API / DEPLOY / SIM / SDR / MIGRATION + images/samples/legacy
scripts/              smoke.sh、deploy 脚本 / smoke.sh and deploy scripts
third_party/pysim   定制 testsim 卡逻辑（GPL，随 era-pinned pysim 使用）/ custom testsim logic
```

## 快速开始（服务器端） Quick Start (Server Side)

```bash
cd ~/lte-system
sudo docker compose -f deploy/docker/docker-compose.bridge.yml up -d --build
curl --fail http://127.0.0.1:8080/ui-config.json; echo
# Open http://HOST:8080. Container creation does not start the LTE cell.
```

完整 API 双端口测试须显式叠加 override；`LTE_UI_PORT` 与 `LTE_API_PORT` 不得相同：

Direct full-API testing explicitly adds the override; `LTE_UI_PORT` and `LTE_API_PORT` must differ:

```bash
sudo docker compose \
  -f deploy/docker/docker-compose.bridge.yml \
  -f deploy/docker/docker-compose.test.yml up -d --build
BASE=http://127.0.0.1:${LTE_API_PORT:-8081} bash scripts/smoke.sh
```

启动基站示例（mcc `001` mnc `01`，apn 与终端一致）：

Example cell startup (mcc `001`, mnc `01`; apn must match the UE/terminal device):

```bash
curl -X POST http://127.0.0.1:8081/api/v1/cell -H 'Content-Type: application/json' \
  -d '{"band":"7","apn":"srsapn","mcc":"001","mnc":"01","network":"auto","sdr":"auto","full_net_name":"MyLTE","short_net_name":"MyLTE"}'
```

完整入网流程见 [`docs/QUICKSTART.md`](docs/QUICKSTART.md)。

See [`docs/QUICKSTART.md`](docs/QUICKSTART.md) for the full network attach flow.

bridge 快速开始需要 Compose >= 2.36.0；已有 host 实例先按 [DEPLOY](docs/DEPLOY.md#7-bridge-迁移--bridge-migration) 备份迁移，不要直接照抄全参数覆盖当前频段/APN/DNS。确认 DNS 在实际网络可达，容器 DNS 和手机 PCO DNS 是不同配置。
The bridge quick start requires Compose >= 2.36.0. For an existing host deployment, first follow [DEPLOY](docs/DEPLOY.md#7-bridge-迁移--bridge-migration); do not overwrite current band/APN/DNS using a full example request. Validate DNS reachability; container DNS and handset PCO DNS are separate settings.

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
- [docs/API.md](docs/API.md) — 标准接口完整参考 / Standard API reference ([OpenAPI](docs/api/openapi.yaml))
- [docs/DEPLOY.md](docs/DEPLOY.md) — 服务器部署/升级/备份/排障 / Server deployment, upgrade, backup and troubleshooting
- [docs/WEB_UI.md](docs/WEB_UI.md) — 控制台路由、端口、Token、Host/Origin 与管理专用启动 / Console routes, ports, token, Host/Origin and management-only boot
- [deploy/docker/README.md](deploy/docker/README.md) — Docker 专讲：镜像三段构建、compose 逐项解释、版本迭代、构建排障 / Docker deep dive
- [docs/SIM.md](docs/SIM.md) — 灵活写卡与卡型 / Flexible SIM programming and card types
- [docs/SDR.md](docs/SDR.md) — B210（兼容板 FPGA 切换）与 bladeRF / B210 (compat-board FPGA switching) and bladeRF
- [docs/MIGRATION.md](docs/MIGRATION.md) — 旧 Python → Go 对照 / Old Python vs Go mapping
- [AGENTS.md](AGENTS.md) — 本地开发规范（subagent / handoff / folk） / Local dev guide (subagent / handoff / folk)

## 参与贡献与许可 Contributing and License

- 想一起改？先看 [CONTRIBUTING.md](CONTRIBUTING.md)（测试要求、API 兼容铁律、PR 模板）
  Want to contribute? Read [CONTRIBUTING.md](CONTRIBUTING.md) first (test requirements, API compatibility rules, PR template)
- 安全问题走 [SECURITY.md](SECURITY.md) 私密通道；建议启用强 Token 并限制可信管理网。匿名模式下 Host allowlist 不替代鉴权；普通 TLS 终止反代还需验证严格 Origin 行为。
  For security issues use the private channel in [SECURITY.md](SECURITY.md). Enable a strong token and restrict the management network. In anonymous mode, the Host allowlist does not replace authentication; ordinary TLS termination also requires validation of strict Origin behavior.
- 本仓库代码 MIT（[LICENSE](LICENSE)）；镜像内含 AGPL-3.0 的 srsRAN 等第三方组件，见 [NOTICE.md](NOTICE.md) / This repo's code is MIT ([LICENSE](LICENSE)); the image contains third-party components such as srsRAN under AGPL-3.0, see [NOTICE.md](NOTICE.md)
- 版本历史见 [CHANGELOG.md](CHANGELOG.md) / See [CHANGELOG.md](CHANGELOG.md) for version history
