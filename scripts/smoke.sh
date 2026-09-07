#!/bin/bash
# Read-only API smoke checks; no radio, SIM, uploads, captures or stop calls.
# Usage: BASE=http://127.0.0.1:8081 LTE_API_TOKEN=TOKEN bash scripts/smoke.sh
set -euo pipefail
BASE="${BASE:-http://127.0.0.1:8081}"
AUTH=()
if [[ -n "${LTE_API_TOKEN:-}" ]]; then
  AUTH=(-H "Authorization: Bearer $LTE_API_TOKEN")
fi
# /health probes SDR hardware; avoid it while a radio process owns the USB device.
for path in /api/v1/cell /api/v1/profile; do
  curl --fail --silent --show-error --max-time 30 "${AUTH[@]}" "$BASE$path" |
    python3 -c 'import json,sys; d=json.load(sys.stdin); assert d["code"] == 0 and d["request_id"]; assert isinstance(d["data"],dict); print(json.dumps(d))'
done
echo "SMOKE PASSED (read-only; RF/SIM operation not tested)"
