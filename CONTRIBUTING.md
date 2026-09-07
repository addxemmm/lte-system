# 贡献指南 Contributing

感谢关注。这是一个实验室工具（SDR LTE 小区 + SIM 工具），评审重点：无 shell 注入、标准 API 契约（`code`/HTTP 状态语义）、代码与文档同步更新、测试通过。
Thanks for stopping by. This is a lab tool (SDR LTE cell + SIM tooling), so reviews focus on: no shell injection, standard API contracts (`code`/HTTP status semantics), docs updated alongside code, and tests passing.

## 工作流 Workflow

1. Fork，从 `master` 建分支：`folk/<your-name>-<topic>`（短命分支，一个 PR）。 / Fork, branch from `master`: `folk/<your-name>-<topic>` (short-lived, one PR).
2. 本地检查（Windows 或 Linux）： / Local checks (Windows or Linux):
   ```bash
   go test ./...
   go vet ./...
   GOOS=linux GOARCH=amd64 go build -o /tmp/lte-system ./cmd/server
   ```
3. 推送并向 `master` 提 PR，写明：改了什么、更新了哪些 API/文档；涉及射频/写卡行为变化必须附服务器验证日志（`docker logs` + `curl` 输出）。无验证日志的影响射频的 PR 不会合入。 / Push, open a PR against `master` with: what changed, which API/docs updated, and — for RF/SIM behavior changes — server validation logs (`docker logs` + `curl` output). RF-affecting PRs without validation logs will not be merged.
4. 一个 PR 只做一件事。合并时 squash。 / One concern per PR. Squash on merge.

## 规则 Rules

- **API 契约 API contract**：3.0 仅提供标准 `/api/v1`；破坏性变更须升级大版本并记录迁移。 / Version 3.0 exposes only standard `/api/v1`; breaking changes require a major version and migration notes.
- **文档即代码 Docs are code**：[`docs/API.md`](docs/API.md)（与 `server.go` 逐字节一致）及相关的 `QUICKSTART`/`SIM`/`SDR`/`RULES` 章节必须在同一 PR 内修改。 / **Docs are code**: [`docs/API.md`](docs/API.md) (byte-exact with `server.go`), plus the relevant `QUICKSTART`/`SIM`/`SDR`/`RULES` section, must change in the same PR.
- **无密钥 No secrets**：只用测试密钥。真实 Ki/OPc、密码、token 永不入库。 / **No secrets**: test keys only. Real Ki/OPc, passwords, tokens never enter git.
- **无运行产物 No runtime artifacts**：`*.log`/`*.pcap`/`bin`/crash 永不入库；代表性样本放到 [`docs/samples/`](docs/samples)。 / **No runtime artifacts**: `*.log`/`*.pcap`/`bin/`/crashes never enter git; representative samples go to [`docs/samples/`](docs/samples).
- **提交信息 Commit messages**：提交信息双语格式 `<scope>: <中文> / <English>`，如 `api: 修复 getfile 缺 id / fix getfile missing id`、`docs: 补全 API 参考 / expand API reference`、`deploy: 升级 srsRAN 到 release_24_xx / bump srsRAN to release_24_xx`。 / **Commit messages** are bilingual `<scope>: <中文> / <English>`, e.g. `api: 修复 getfile 缺 id / fix getfile missing id`, `docs: 补全 API 参考 / expand API reference`, `deploy: 升级 srsRAN 到 release_24_xx / bump srsRAN to release_24_xx`.
- Go 风格：动过的文件 `gofmt` 干净；标准库优先，不经讨论不加新依赖。 / Go style: `gofmt` clean for files you touch; stdlib-first, no new dependencies without discussion.

## 上报缺陷 Reporting Bugs

用 issue 模板（`.github/ISSUE_TEMPLATE/`）：附版本（`VERSION` + `git log --oneline -3`）、`GET /api/v1/cell` + `GET /api/v1/profile` 输出、相关容器日志、SDR 型号、用过的频段/参数。真实密钥一律脱敏。
Use the issue templates (`.github/ISSUE_TEMPLATE/`): include version (`VERSION` + `git log --oneline -3`), `GET /api/v1/cell` + `GET /api/v1/profile` output, relevant container logs, SDR model, and band/params used. Redact any real keys.

---
**导航 Navigation:** [README](README.md) · [AGENTS](AGENTS.md) · [SECURITY](SECURITY.md)
