# Web 控制台 / Web UI

Web 控制台与 Go API 编译为同一个 `/usr/local/bin/lte-system`，静态文件位于 `internal/webui/static` 并由 `go:embed` 打包。镜像没有 Node 或 nginx 构建/运行层；修改前端后必须重新编译 Go 二进制。

The Web UI and Go API compile into the same `/usr/local/bin/lte-system` binary. Assets under `internal/webui/static` are packaged with `go:embed`. The image has no Node or nginx build/runtime layer; frontend changes require a new Go binary.

版本号保持 **2.1**。本页说明通用部署契约；2026-09-09 测试实例已完成部署，详见 [验收记录](WEB_UI_RELEASE_2026-09-09.md)。

The version remains **2.1**. This page describes the general deployment contract. The 2026-09-09 test deployment is recorded in the linked acceptance report.

## 1. 两个监听面 / Two listener surfaces

| 场景 Scenario | Web 控制台 / Console | 独立完整 API / Direct full API | 说明 Notes |
|---|---|---|---|
| bridge 默认 / default | `HOST:${LTE_UI_PORT:-8080}` → `:8080` | 不监听、不发布 / not listening or published | 推荐部署；只发布控制台口 / recommended |
| bridge 测试 / test | `HOST:${LTE_UI_PORT:-8080}` → `:8080` | `HOST:${LTE_API_PORT:-8081}` → `:8081` | `docker-compose.test.yml` 显式启用 |
| host 默认 / default | `${LTE_UI_LISTEN:-0.0.0.0:8080}` | 不监听 / not listening | host 网络没有 `ports` 隔离层 |
| host 测试 / test | `LTE_UI_LISTEN` | `LTE_LISTEN`（默认 `0.0.0.0:8081`） | 设 `LTE_EXPOSE_API=true`；不使用 `ports` |

`ui_listen_addr`（默认 `:8080`）始终启动控制台；`listen_addr`（默认 `:8081`）只定义独立完整 API 地址，只有 `expose_api: true` 才绑定。`LTE_UI_LISTEN`、`LTE_LISTEN` 分别覆盖地址；`LTE_EXPOSE_API` 只接受不区分大小写的 `true` 或 `false`，其它非空值会令服务启动失败。

`ui_listen_addr` (default `:8080`) always starts the console. `listen_addr` (default `:8081`) defines the direct full-API address and binds only when `expose_api: true`. `LTE_UI_LISTEN` and `LTE_LISTEN` override the addresses. `LTE_EXPOSE_API` strictly accepts case-insensitive `true` or `false`; any other nonempty value fails startup.

只有在 `expose_api=true` 时，控制台与独立 API 端口相同才被拒绝。bridge 测试中 `LTE_UI_PORT` 与 `LTE_API_PORT` 也必须不同，否则 Docker 无法同时发布两个宿主端口。`EXPOSE` 只是镜像元数据，不是防火墙；实际边界由应用是否绑定监听器、Compose `ports` 和宿主防火墙共同决定。

Equal console/API listen ports are rejected only when `expose_api=true`. In bridge tests, `LTE_UI_PORT` and `LTE_API_PORT` must also differ or Docker cannot publish both host ports. `EXPOSE` is image metadata, not a firewall; the actual boundary is the application listener, Compose publishing and host firewall.

## 2. 控制台网关范围 / Console gateway scope

控制台的同源 `/api/v1` **不是完整 API 代理**。它只允许 UI 使用的管理查询与小区启停：

The console's same-origin `/api/v1` is **not a full API proxy**. It permits only the management reads and cell lifecycle calls used by the UI:

- `GET /api/v1/cell`，`POST /api/v1/cell`，`DELETE /api/v1/cell`
- `GET /api/v1/network`
- `GET /api/v1/ues`、`GET /api/v1/ues/{imsi}`
- `GET /api/v1/subscribers`、`GET /api/v1/subscribers/{imsi}`
- `GET /api/v1/profile`
- `GET /api/v1/health`
- `GET /api/v1/diagnostics/connectivity`

网关不提供 crack、SIM 写卡、订户变更/上传、抓包文件或其它 capture/upload 路径；这些接口只在显式启用的独立 `:8081` 完整 API 上可用。控制台 `POST`/`DELETE` 还要求同源检查和 `X-LTE-UI: 1`。网关不注入服务端令牌，所有转发请求继续经过原 API Bearer Token 门禁、审计和请求 ID 中间件。

The gateway does not expose crack, SIM programming, subscriber mutation/upload, capture-file or other capture/upload routes. Those remain available only through the explicitly enabled direct full API on `:8081`. Console `POST`/`DELETE` calls additionally require same-origin validation and `X-LTE-UI: 1`. The gateway never injects a server-side token; forwarded requests still pass through the original Bearer-token, audit and request-ID middleware.

`GET /ui-config.json` 始终由控制台口提供，并设置 `no-store`。它只有五个非敏感字段：`version`、`api_exposed`、`api_port`、`ui_port`、`auth_required`；不含 Token、卡密、内部配置或日志。

`GET /ui-config.json` is always available on the console listener with `no-store`. It contains exactly five non-sensitive fields: `version`, `api_exposed`, `api_port`, `ui_port` and `auth_required`; it contains no token, SIM secret, internal configuration or log data.

## 3. 浏览器鉴权 / Browser authentication

非空 YAML `api_token` 启用鉴权；非空 `LTE_API_TOKEN` 在启动时覆盖 YAML。Compose 传入的空 `LTE_API_TOKEN` 不会清除文件 Token。关闭鉴权需同时清空环境与所选配置文件。Token 优先级：

A nonempty YAML `api_token` enables authentication; a nonempty `LTE_API_TOKEN` overrides YAML at startup. Compose's empty `LTE_API_TOKEN` does not erase a file token. Clear both sources to disable authentication. Token precedence is:

1. 非空 `LTE_API_TOKEN` / nonempty environment value
2. 所选 YAML 的非空 `api_token` / selected YAML value
3. 空值表示匿名 / empty means anonymous

浏览器在控制台对话框中输入 Token，前端仅保存在当前页面内存，并给同源 API 请求添加 `Authorization: Bearer TOKEN`；刷新后需重新输入。不要把 Token 放进 URL、`ui-config.json`、镜像 build arg 或 Git。匿名模式下，同一网络中能访问控制台的用户可执行网关允许的小区启停操作；即使使用 Host 校验，也建议在非单用户环境启用强 Token。

The browser accepts the token in the console dialog, keeps it only in current-page memory and adds `Authorization: Bearer TOKEN` to same-origin API requests; a refresh requires re-entry. Never put a token in a URL, `ui-config.json`, image build argument or Git. In anonymous mode, anyone who can reach the console can invoke permitted cell start/stop operations. Enable a strong token outside a single-user network even with Host validation.

## 4. Host、DNS 与 TLS / Host, DNS and TLS

`ui_allowed_hosts` 是 YAML 列表，条目只写主机名，不含 scheme 或端口：

`ui_allowed_hosts` is a YAML list. Each entry is a hostname only, without scheme or port:

```yaml
ui_allowed_hosts:
  - lte-console.example.internal
```

空列表默认只接受字面量 IP 地址或 localhost。用自定义 DNS 名称访问时必须把该名称显式加入列表。独立 legacy/full API 的 Host 行为不变。Host allowlist 用于降低 DNS rebinding 风险，但不是 Token 或网络访问控制的替代品，尤其不要把匿名控制台直接开放到不可信网络。

An empty list accepts literal IP addresses or localhost by default. Add every custom DNS name explicitly when accessing the console by name. Host behavior of the independent legacy/full API is unchanged. The Host allowlist reduces DNS-rebinding risk but does not replace a token or network access control; do not expose an anonymous console directly to an untrusted network.

Origin 检查是严格的：当前实现按请求是否直接 TLS 判断 `http`/`https`，不信任 `X-Forwarded-Proto`。普通 TLS 终止反向代理会让浏览器发送 `https` Origin、而 Go 后端看到 HTTP，状态变更请求会返回 403。上线 TLS 反代前必须验证端到端 Origin；当前版本不要依赖伪造/转发 `X-Forwarded-Proto` 绕过。反代时 UI 与网关仍须同一 origin，且不得扩展网关路由范围。

Origin validation is strict: the current implementation derives `http`/`https` from direct TLS on the request and does not trust `X-Forwarded-Proto`. With an ordinary TLS-terminating reverse proxy, the browser sends an `https` Origin while the Go backend sees HTTP, so state-changing requests return 403. Validate Origin end to end before deploying a TLS proxy; this release does not use forwarded-protocol headers as a bypass. The UI and gateway must remain same-origin and the proxy must not broaden gateway routes.

## 5. Compose 启停 / Compose start and stop

默认 bridge（仅控制台口）：

Default bridge (console port only):

```bash
COMPOSE=deploy/docker/docker-compose.bridge.yml
docker compose -p "$PROJECT" -f "$COMPOSE" config --quiet
docker compose -p "$PROJECT" -f "$COMPOSE" up -d --no-build --force-recreate
curl -fsS http://127.0.0.1:${LTE_UI_PORT:-8080}/ui-config.json
docker compose -p "$PROJECT" -f "$COMPOSE" stop
```

bridge 双端口测试（必须同时列出两个文件，顺序不能反）：

Bridge dual-port test (include both files in this order):

```bash
COMPOSE_BASE=deploy/docker/docker-compose.bridge.yml
COMPOSE_TEST=deploy/docker/docker-compose.test.yml
test "${LTE_UI_PORT:-8080}" != "${LTE_API_PORT:-8081}"
docker compose -p "$PROJECT" -f "$COMPOSE_BASE" -f "$COMPOSE_TEST" config --quiet
docker compose -p "$PROJECT" -f "$COMPOSE_BASE" -f "$COMPOSE_TEST" up -d --no-build --force-recreate
curl -fsS http://127.0.0.1:${LTE_UI_PORT:-8080}/ui-config.json
BASE=http://127.0.0.1:${LTE_API_PORT:-8081} bash scripts/smoke.sh
docker compose -p "$PROJECT" -f "$COMPOSE_BASE" -f "$COMPOSE_TEST" stop
```

启用 Token 时给 curl 加 Bearer header，并把同一私有 `LTE_API_TOKEN` 传给 smoke。容器创建只启动 Go 管理服务，**不会自动启动 LTE 小区**；是否启动小区必须由操作员在控制台明确确认。

When authentication is enabled, add the Bearer header to curl and pass the same private `LTE_API_TOKEN` to smoke. Container creation starts only the Go management service and **does not automatically start the LTE cell**; an operator must explicitly confirm any cell start in the console.

### 跳过硬件探测的管理启动 / Management boot without hardware probes

正常 `/entrypoint.sh` 保持原 LTE 行为：选择 FPGA、播种 `/data`、启动 `pcscd`，并探测 UHD/bladeRF。与另一容器共享 SDR/USB 时，可用私有 override 直接执行 Go 二进制：

The normal `/entrypoint.sh` retains full LTE behavior: FPGA selection, `/data` seeding, `pcscd` startup and UHD/bladeRF probes. When SDR/USB is shared with another container, a private override can execute only the Go binary:

```yaml
services:
  lte-system:
    entrypoint: ["/usr/local/bin/lte-system"]
```

该 override 仅用于**已确认、已播种且继续挂载同一实际 `lte-data` 卷**的部署。它跳过静态 `sib.conf`/`rb.conf` 刷新、`wordlist.list` 首次播种、FPGA 选择、PC/SC 和启动时无线探测；不要用于新的空卷。使用前从现有容器确认实际卷名，并只读核对 `/data/conf`、订户库、profile/日志等预期数据。需要首次播种或完整 LTE/写卡行为时仍使用基础文件的正常 entrypoint。直接 Go 启动也不会自动启动小区。

Use this override only with a **confirmed, already seeded deployment that keeps mounting the same actual `lte-data` volume**. It skips static `sib.conf`/`rb.conf` refresh, first-run `wordlist.list` seeding, FPGA selection, PC/SC and startup radio probes; do not use it with a new empty volume. Resolve the actual volume from the existing container and inspect expected `/data/conf`, subscriber, profile and log state read-only first. Use the normal base entrypoint for initial seeding or full LTE/SIM behavior. Direct Go startup still does not start a cell automatically.

---
## 停止小区与停止系统 / Stop the cell versus stop the system

- 控制台「停止小区」只停止当前管理器的小区及其子进程，Web/API 仍可访问。
  The console's Stop Cell action stops the manager's cell and child processes, leaving Web/API available.
- 停止整个系统应停止已核对名称/ID 的 LTE 容器；给管理器预留 210 秒优雅退出时间。停止后 Web/API 离线，但容器、镜像和数据卷仍保留。
  To stop the whole system, stop the LTE container after verifying its name/ID, allowing 210 seconds for graceful shutdown. Web/API then go offline; the container, image and data volume remain.

```bash
# Server only; verify that this is the LTE container, not GSM.
docker inspect -f '{{.Name}} {{.Id}} {{.Config.Image}} {{.State.Status}}' ltesystem
docker stop --timeout 210 ltesystem
docker inspect -f '{{.State.Status}} exit={{.State.ExitCode}}' ltesystem
ss -ltn '( sport = :8080 or sport = :8081 )'
```

如使用自定义名称/端口，请先调整检查命令。不要用共享 Compose 项目的 `down`、`--remove-orphans` 或 `down -v` 代替定向停止；这些操作可能影响 GSM 或数据卷。停止操作不会上传镜像、创建 Release 或自动恢复服务。需要恢复时，应另行确认原配置和数据挂载，再执行已核对容器的启动操作。

Adapt checks for custom names/ports. Do not substitute `down`, `--remove-orphans` or `down -v` on a shared Compose project: these may affect GSM or data volumes. Stopping neither publishes an image/Release nor automatically restores service. Before a separately requested restart, verify the original configuration and data mounts.

2026-09-09 测试实例现已停止，见 [验收与停机记录](WEB_UI_RELEASE_2026-09-09.md)。

The 2026-09-09 test instance is now stopped; see the [acceptance/shutdown record](WEB_UI_RELEASE_2026-09-09.md).

---
**导航 Navigation:** [README](../README.md) · [部署](DEPLOY.md) · [Docker](../deploy/docker/README.md) · [API](API.md) · [发布管理](RELEASING.md)
