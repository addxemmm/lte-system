# Contributing

Thanks for stopping by. This is a lab tool (SDR LTE cell + SIM tooling), so
reviews focus on: no shell injection, API compatibility (`message_id`
semantics), docs updated alongside code, and tests passing.

## Workflow

1. Fork, branch from `master`: `folk/<your-name>-<topic>` (short-lived, one PR).
2. Local checks (Windows or Linux):
   ```bash
   go test ./...
   go vet ./...
   GOOS=linux GOARCH=amd64 go build -o /tmp/lte-system ./cmd/server
   ```
3. Push, open a PR against `master` with: what changed, which API/docs updated,
   and — for RF/SIM behavior changes — server validation logs
   (`docker logs` + `curl` output). RF-affecting PRs without validation logs
   will not be merged.
4. One concern per PR. Squash on merge.

## Rules

- **API compatibility**: `message` text and `message_id` are a contract.
  Changing them requires a major version bump and a `docs/MIGRATION.md` entry.
- **Docs are code**: `docs/API.md` (byte-exact with `server.go`), plus the
  relevant `QUICKSTART`/`SIM`/`SDR`/`RULES` section, must change in the same PR.
- **No secrets**: test keys only. Real Ki/OPc, passwords, tokens never enter git.
- **No runtime artifacts**: `*.log`/`*.pcap`/`bin/`/crashes never enter git;
  representative samples go to `docs/samples/`.
- **Commit messages**: `<scope>: <what>`, e.g. `api: fix getfile missing id`,
  `docs: expand API reference`, `deploy: bump srsRAN to release_24_xx`.
- Go style: `gofmt` clean for files you touch; stdlib-first, no new
  dependencies without discussion.

## Reporting bugs

Use the issue templates (`.github/ISSUE_TEMPLATE/`): include version
(`VERSION` + `git log --oneline -3`), `GET /status` + `GET /healthz` output,
relevant container logs, SDR model, and band/params used. Redact any real keys.
