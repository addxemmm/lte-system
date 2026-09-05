#!/bin/bash
# scripts/smoke.sh — API smoke test against a running container (default 127.0.0.1:8081).
# Usage: BASE=http://192.168.100.199:8081 bash scripts/smoke.sh
set -u
BASE="${BASE:-http://127.0.0.1:8081}"

post() { curl -s -m 15 -X POST "$BASE$1" -H 'Content-Type: application/json' -d "${2:-{}}"; echo; }

echo "== /healthz =="; curl -s -m 10 "$BASE/healthz"; echo
echo "== /status ==";  curl -s -m 10 "$BASE/status"; echo
echo "== /stop ==";    post /stop '{}'
echo "== /basicinfo =="; post /basicinfo '{}'
echo "== /getfile bad id =="; post /getfile '{"fileid":99}'
echo "== uploads dry-run (expect no-file message when empty) =="
curl -s -m 10 -X POST "$BASE/userupload"; echo
echo "SMOKE DONE (start/crack/writesim need SDR hardware; see docs/API.md)"
