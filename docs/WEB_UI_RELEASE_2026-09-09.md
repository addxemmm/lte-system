# 2.1 Web 管理台测试发布 / Web console test release

## 已完成 / Delivered

2026-09-09 已在 Ubuntu SDR 服务器部署前后端同容器管理台，运行版本保持 **2.1**。本次只启动管理服务，不启动 LTE 小区，不执行 SDR 探测，也不改动 GSM 服务。

The embedded frontend and backend were deployed together on the Ubuntu SDR server on 2026-09-09. Runtime version remains **2.1**. This deployment starts management services only: no LTE cell startup, SDR probing, or GSM service changes.

- 测试入口 / Test console: `http://HOST:8080`
- 测试独立 API / Test direct API: `http://HOST:8081`
- 镜像 / Image: `ltesystem-dep:2.1`
- Image ID: `sha256:afb2118aaecaedb44f5a2a14aee6f0dd4157967d99ff2405dfb95f023b7a7502`
- Source archive SHA-256: `7697670a9d0f866cbddc83b30616ed48b11e136826807df63d0acd5ed5ac7553`
- OCI revision: `289cb31-worktree-7697670a9d0f`

该标识是构建当时基于 `289cb31` 的未提交工作树快照，不是一个 Git commit。实现随本记录纳入 Git 历史；后续提交不改变已有镜像的来源标识。发布后的验收记录可晚于镜像中的源快照；本地 Image ID 也不是 Docker Hub 分发 digest。

The revision identifies the build-time uncommitted working-tree snapshot based on `289cb31`, not a Git commit. The implementation enters Git history with this record; later commits do not change the existing image's source identity. Post-deployment acceptance records may be newer than the image's source snapshot. A local image ID is not a Docker Hub distribution digest.

## 当前状态：已停止 / Current status: stopped

2026-09-09 **03:11:57 UTC（香港时间 11:11:57）**，按用户要求正常停止 `ltesystem` 容器，退出码 **0**。宿主机 8080/8081 无监听，因此上述测试入口当前离线。保留容器、`ltesystem-dep:2.1` 镜像及原数据卷，没有重新构建、上传 Docker Hub 或创建正式 GitHub Release。

At **03:11:57 UTC (11:11:57 Hong Kong)** on 2026-09-09, `ltesystem` was stopped as requested with exit code **0**. Host ports 8080/8081 have no listeners, so the test endpoints above are offline. The container, `ltesystem-dep:2.1` image and original data volume remain. No image rebuild, Docker Hub upload or formal GitHub Release was performed.

停止前后 GSM 容器 ID、启动时间和 running 状态一致；停止后检查显示其健康状态为 `unhealthy`。这是独立健康告警，本次未诊断或改动 GSM，也未将它记为健康通过。

GSM's container ID, start time and running status were unchanged across the LTE stop. The post-stop check reported GSM as `unhealthy`; this separate health alert was not investigated or changed in this task and is not recorded as a passing health check.

未来版本与发布流程见 [发布管理](RELEASING.md) 和 [变更记录](../CHANGELOG.md)。

See [release management](RELEASING.md) and the [changelog](../CHANGELOG.md) for future versioning and publication.

## 功能与默认行为 / Features and defaults

六个页面：概览、小区控制、UE 设备、授权订户只读列表/详情、网络诊断、设置与帮助。包含中英文切换、深浅主题、信号塔/网络拓扑 SVG、响应式布局、减少动画偏好、确认弹窗和请求错误提示。界面不会编造 RSSI、RSRP、在线数量或 Internet 连通状态。

Six pages cover overview, cell control, UE devices, read-only authorized subscribers, diagnostics, and settings/help. Features include Chinese/English, light/dark themes, SVG tower/topology illustrations, responsive layouts, reduced-motion support, confirmations, and request errors. The UI never invents RSSI/RSRP, session counts, or Internet reachability.

默认 `expose_api: false` 不创建独立 API socket，只开放管理台及有限同源网关。此次按用户要求启用测试 override，同时开放 8080/8081。保持现场原有空 Token 配置，因此当前测试实例允许匿名管理访问；应限制在可信网络内。非空配置 Token 的行为在独立临时容器中验证，没有将测试凭据带入正式数据卷。

By default, `expose_api: false` creates no independent API socket; only the console and its limited same-origin gateway are exposed. The requested test override enables both ports. The existing empty-token configuration was preserved, so this test instance permits anonymous management access and should remain on a trusted network. Nonempty-token behavior was tested in a separate disposable container, never in the live data volume.

## 验证证据 / Validation

- Windows：全量 `go test -count=1 ./...`、`go vet ./...` 通过；Python 27 项，26 通过、1 项 POSIX-only 测试按平台跳过。
- Linux：全量 Go race 测试、Go vet 通过；Python 27 项全部通过，包含 POSIX shell 路径验证。前端新增测试覆盖 ES module 语法、CSP、API 请求头、中止、完整响应超时、旧 401 不清除新 Token，以及启动确认接线。
- 独立候选容器：默认未启用 API 时，即使映射 8081 也不接受 API 请求；双端口共享 Bearer 校验；公开元数据只有五个字段；未知 Host、跨源及无管理标记的写请求被拒绝。
- 浏览器：中英文、深浅主题、390×844 移动宽度、无横向页面溢出、草稿跨语言切换保留、显式/存档启动取消、Token 刷新后重新输入、错误 Token 后恢复、订户详情焦点与 Escape 关闭。
- 数据：备份中所有既有文件的 SHA-256 与部署后的数据卷逐一匹配，包括存档 profile 和订户文件/SQN。
- 运行：容器内只有 Go 管理进程，无 EPC/eNodeB/tcpdump；UI/API 均报告小区停止。LTE 替换前后 GSM 容器 ID 与启动时间一致，健康状态正常。

Windows Go tests/vet passed; Python reported 26 passes and one platform-specific skip. Linux Go race/vet passed. Candidate tests proved optional-listener behavior, shared authentication, nonsecret metadata and Host/Origin restrictions. Browser checks covered languages/themes, mobile layout, draft preservation, cancelled starts, token re-entry/recovery and keyboard-accessible subscriber details. Every backed-up data file matched after deployment; the cell remained stopped, and GSM identity/start time remained unchanged during replacement.

本次没有启动射频，没有进行手机注册、信号强度或 Internet 验收；CPU/mock/UI 测试不替代这些硬件验证。

No RF, handset registration, signal-strength or Internet acceptance was performed. CPU/mock/UI checks do not substitute for hardware validation.

## 启动与保留策略 / Startup and retention

现场沿用已播种的数据卷，并使用管理模式覆盖入口：先执行容器内的 `select-uhd-fpga compat`（只复制镜像文件，不探测硬件），随后 `exec /usr/local/bin/lte-system`。跳过 PC/SC、SDR 探测和自动小区启动；新空卷应先按正常部署流程初始化。

The already-seeded data volume is retained. The management override selects the container-local compatibility FPGA file without probing hardware, then executes the Go binary. It skips PC/SC, SDR probes and automatic cell startup. Initialize a new empty volume through the normal deployment workflow first.

已移除临时预览容器、候选 tag、三份中间候选镜像及旧 LTE 镜像。服务器只保留当前 LTE/GSM 运行镜像，不保留回滚镜像；保留数据备份、源快照、验收证据及约 0.5 GB 可复用构建缓存。未删除任何数据卷或 GSM 文件。

The preview container, candidate alias, three intermediate images and old LTE image were removed. Only current LTE/GSM runtime images remain; no rollback image is retained. Data backups, source/evidence and approximately 0.5 GB of reusable build cache remain. No data volume or GSM file was deleted.

配置和操作说明见 [Web UI guide](WEB_UI.md)、[API](API.md)、[deployment](DEPLOY.md) 与 [Postman](../postman/README.md)。
