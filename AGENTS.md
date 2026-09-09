# AGENTS.md — lte-system 开发规范 Development Guide

> 开发机只负责代码编辑、文档与 git 管理；**不要在开发机运行射频、docker、常驻服务**。一切构建镜像、启动基站、写卡、抓包验证都在 SDR 服务器（本文示例 `192.0.2.10`）上执行。
> Dev machines only handle code editing, docs, and git management; **do not run radio, docker, or long-lived services on dev machines**. All image builds, cell startup, SIM writing, and packet capture validation run on the SDR server (example `192.0.2.10` in this doc).

## 1. 并发与 subagent Concurrency and Subagents

- 独立工作流优先并行：收到多域任务时，用 Task 工具同时起 `explore`/`general` 子代理（如：srsRAN 调研 × 旧逻辑盘点 × 布局设计），主会话只做合并与裁决。 / Prefer parallelism for independent work: on multi-domain tasks, launch `explore`/`general` subagents together via Task (e.g. srsRAN research × legacy logic inventory × layout design); the main session only merges and decides.
- 子代理 prompt 必须包含：工作目录、目标文件、输出上限（800–1500 字）、“只调研/只设计，不写实现”或相反的明确交付物，避免返工。 / Subagent prompts must include: working directory, target files, output cap (800–1500 words), and an explicit deliverable of "research/design only, no implementation" or the opposite, to avoid rework.
- 不要把同一文件的读写分给两个并行代理；合并后再做串行 edit。 / Never give reads and writes of the same file to two parallel agents; do serial edits after merging.

## 2. 交接 Handoff

- 跨会话交接只认三样：`git log --oneline -10`、本文件、[`docs/MIGRATION.md`](docs/MIGRATION.md)。新会话先读这三样再动手。 / Cross-session handoff recognizes only three things: `git log --oneline -10`, this file, and [`docs/MIGRATION.md`](docs/MIGRATION.md). New sessions read these three first before acting.
- 中断前必须留下：当前分支、`git status --short`、未完成 todo、下一步命令（如 `go test ./...`）。 / Before interrupting, always leave: current branch, `git status --short`, unfinished todos, and next commands (e.g. `go test ./...`).
- 交接信息写进 commit message 或 docs 注释，不写聊天黑话。 / Write handoff info into commit messages or docs comments, not chat slang.

## 3. 分叉试错 Folk Experiments

- 高风险改动（conf 模板、写卡参数、kill 逻辑）先建 `folk/<name>` 分支试，验证通过才合回 `master`。 / For high-risk changes (conf templates, SIM-writing params, kill logic), try them on a `folk/<name>` branch first and merge back to `master` only after validation.
- folk 分支只活一个 PR 周期，合后即删；禁止在 folk 分支上再分叉。 / folk branches live for one PR cycle only; delete after merge; forking again from a folk branch is forbidden.
- 涉及射频/写卡的 folk 必须附服务器验证日志（`docker logs` + `curl` 输出）才能合。 / folk branches touching radio/RF or SIM writing must attach server validation logs (`docker logs` + `curl` output) before merging.

## 4. 开发铁律 Core Development Rules

- 开发机可执行：`go test ./...`、`go vet ./...`、`go build`（含 `GOOS=linux` 交叉编译）、文档编辑、`git` 操作。不可执行：`docker build/run`、`./bin/lte-system` 常驻、`uhd_find_devices` 结论性判断（本机无硬件）。 / Dev machines may run: `go test ./...`, `go vet ./...`, `go build` (incl. `GOOS=linux` cross-compile), docs edits, and `git` ops. Never run: `docker build/run`, long-lived `./bin/lte-system`, or conclusive `uhd_find_devices` judgments (no hardware here).
- 读文件用 Read，查内容用 Grep/Glob，改文件用 Edit（先 Read），跑命令用 Bash（`workdir` 指到仓库根，不 `cd`）。 / Read files with Read, search with Grep/Glob, edit with Edit (Read first), run commands with Bash (`workdir` pointed at the repo root, no `cd`).
- 每个 Edit 保持最小 diff；改 API 必须同步改 [`docs/API.md`](docs/API.md)（v1 优先）+ [`docs/api/openapi.yaml`](docs/api/openapi.yaml) + 对应 `*_test.go`。 / Keep each Edit minimal; API changes must also update [`docs/API.md`](docs/API.md) (v1 first) + [`docs/api/openapi.yaml`](docs/api/openapi.yaml) + the matching `*_test.go`.
- 端点一律进 `/api/v1`（标准包络+状态码）；3.0 已按用户要求删除根路径旧接口及单条 `/api/v1/ue`，不得重新添加兼容 alias。 / All endpoints use `/api/v1` (standard envelope + status codes); 3.0 removes legacy root routes and singleton `/api/v1/ue` at the user's request. Do not reintroduce compatibility aliases.
- 保密：`wordlist.list`、真实 Ki/OPc、服务器密码不进 git（只提交 `.example`）；`firmware/uhd/*.bin` 例外允许跟踪。 / Confidentiality: `wordlist.list`, real Ki/OPc, and server passwords never enter git (commit `.example` only); `firmware/uhd/*.bin` firmware is the allowed exception.
- 仓库瘦身：运行产物（`*.log`/`*.pcap`/`bin/`/`__pycache__`/crash/生成的 `*_run.conf`）永不入库；代表性样本只收 [`docs/samples/`](docs/samples)；厂商大包不入库。 / Keep the repo slim: runtime artifacts (`*.log`/`*.pcap`/`bin/`/`__pycache__`/crashes/generated `*_run.conf`) never enter git; representative samples go to [`docs/samples/`](docs/samples) only; vendor blobs stay out.

## 5. 提交与发布 Commits and Releases

- 提交信息双语格式 `<scope>: <中文> / <English>`（如 `api: 修复 getfile 缺 id / fix getfile missing id`），一个提交只做一件事。 / Commit messages are bilingual `<scope>: <中文> / <English>` (e.g. `api: 修复 getfile 缺 id / fix getfile missing id`); one commit does one thing.
- 推远端前必跑：`go test ./...` 全绿 + `go vet ./...` + `git status` 无多余文件。 / Before pushing: `go test ./...` all green + `go vet ./...` + `git status` shows no extra files.
- 服务器发布先同步独立源快照并构建，再按 [`docs/DEPLOY.md`](docs/DEPLOY.md) 备份实际数据卷、替换并验证。镜像保留遵从用户当前要求；用户明确不要回滚镜像时，在新部署验证后定向删除旧镜像，不删数据卷。部署脚本不自动启动小区或发射；默认独立 bridge 编排，只发布管理台 8080，独立 API 需显式启用。 / Sync an isolated source snapshot and build first, back up the actual data volume, replace and verify. Follow the user's current image-retention choice; remove obsolete images only after verification and never delete data volumes. Deployment does not auto-start the cell or transmit. Default bridge publishing exposes console 8080 only; the independent API requires explicit opt-in.
- 需要新会话接手时，把本文件链接发给对方即可。 / When a new session needs to take over, just send the other party a link to this file.

---
**导航 Navigation:** [README](README.md) · [文档索引 Docs](docs/README.md) · [CONTRIBUTING](CONTRIBUTING.md)
