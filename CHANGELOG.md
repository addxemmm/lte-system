# Changelog

All notable changes to this project are documented here. Versioning:
`VERSION` file + `ltesystem-dep:<VERSION>` image tag.

## [2.0] - 2026-09-05

Full rebuild on the Ubuntu SDR host, validated end-to-end (CPE attach, auth,
IP, NAT, DNS, traffic capture).

- Go + srsRAN_4G (`release_23_11`): 9 legacy Flask APIs preserved 1:1
  (`message_id` semantics), plus `GET /healthz`, `/status`, `/profile`
- Flexible `/writesim` (all card params optional) and `/start`
  (`sdr/device_args/gains/n_prb/net names/dns`, profile inheritance,
  empty-body reuse of last config via `/data/last_start.json`)
- srsRAN_4G compat fixes found live: `drb.conf`→`rb.conf`, upstream `rr.conf`
  format, explicit per-band `ul_earfcn` (TDD derivation broken upstream)
- VM-USB tuning: B210 auto `device_args`, default 5MHz (`n_prb 25`),
  zombie-proof process tracking
- Docker multi-stage image (1.53GB, was 3.46GB), FPGA stock/compat switching,
  atomic config seeding, DOCKER-USER forwarding + MSS clamp automation
- Docs: QUICKSTART/RULES/API reference/SIM/SDR/MIGRATION + golden EPC sample

## [1.x] - 2023 (legacy, EOL)

Python/Flask + srsLTE, manual container, `legacy-python-workspace/` archive.
See `docs/legacy/`. Not maintained; no security fixes.
