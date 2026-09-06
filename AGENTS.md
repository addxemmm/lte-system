# AGENTS.md — lte-system 开发规范

> 开发机只负责代码编辑、文档与 git 管理；**不要在开发机运行射频、docker、常驻服务**。一切构建镜像、启动基站、写卡、抓包验证都在 SDR 服务器（本文示例 `192.0.2.10`）上执行。

## 1. 并发与 subagent

- 独立工作流优先并行：收到多域任务时，用 Task 工具同时起 `explore`/`general` 子代理（如：srsRAN 调研 × 旧逻辑盘点 × 布局设计），主会话只做合并与裁决。
- 子代理 prompt 必须包含：工作目录、目标文件、输出上限（800–1500 字）、“只调研/只设计，不写实现”或相反的明确交付物，避免返工。
- 不要把同一文件的读写分给两个并行代理；合并后再做串行 edit。

## 2. handoff（交接）

- 跨会话交接只认三样：`git log --oneline -10`、本文件、`docs/MIGRATION.md`。新会话先读这三样再动手。
- 中断前必须留下：当前分支、`git status --short`、未完成 todo、下一步命令（如 `go test ./...`）。
- 交接信息写进 commit message 或 docs 注释，不写聊天黑话。

## 3. folk（分叉试错）

- 高风险改动（conf 模板、写卡参数、kill 逻辑）先建 `folk/<name>` 分支试，验证通过才合回 `master`。
- folk 分支只活一个 PR 周期，合后即删；禁止在 folk 分支上再分叉。
- 涉及射频/写卡的 folk 必须附服务器验证日志（`docker logs` + `curl` 输出）才能合。

## 4. 开发铁律

- 开发机可执行：`go test ./...`、`go vet ./...`、`go build`（含 `GOOS=linux` 交叉编译）、文档编辑、`git` 操作。不可执行：`docker build/run`、`./bin/lte-system` 常驻、`uhd_find_devices` 结论性判断（本机无硬件）。
- 读文件用 Read，查内容用 Grep/Glob，改文件用 Edit（先 Read），跑命令用 Bash（`workdir` 指到仓库根，不 `cd`）。
- 每个 Edit 保持最小 diff；改 API 必须同步改 `docs/API.md` + 对应 `*_test.go`。
- 保密：`wordlist.list`、真实 Ki/OPc、服务器密码不进 git（只提交 `.example`）；`firmware/uhd/*.bin` 例外允许跟踪。
- 仓库瘦身：运行产物（`*.log`/`*.pcap`/`bin/`/`__pycache__`/crash/生成的 `*_run.conf`）永不入库；代表性样本只收 `docs/samples/`；厂商大包不入库。

## 5. 提交与发布

- 提交信息：`<scope>: <what>`（如 `api: fix getfile missing id`），一个提交只做一件事。
- 推远端前必跑：`go test ./...` 全绿 + `go vet ./...` + `git status` 无多余文件。
- 服务器发布走 `scripts/deploy_to_ubuntu.sh`，先在服务器 `git pull` 再 `docker compose up -d --build`，回滚用 `docker compose` 上一个 image tag。
- 需要新会话接手时，把本文件链接发给对方即可。
