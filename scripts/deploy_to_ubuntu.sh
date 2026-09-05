#!/bin/bash
# scripts/deploy_to_ubuntu.sh — build & (re)start lte-system on the SDR host.
# Usage: ./scripts/deploy_to_ubuntu.sh user@host [iface]
# Example: ./scripts/deploy_to_ubuntu.sh addx@192.168.100.199
set -e
HOST="${1:?usage: deploy_to_ubuntu.sh user@host}"
REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ssh "$HOST" 'sudo docker --version && mkdir -p ~/lte-system' || true
rsync -avz --delete \
  --exclude 'bin/' --exclude '.git/' --exclude 'var/' \
  "$REPO_DIR/" "$HOST:~/lte-system/"
ssh "$HOST" 'cd ~/lte-system && sudo docker compose -f deploy/docker/docker-compose.yml build && sudo docker compose -f deploy/docker/docker-compose.yml up -d && sleep 3 && sudo docker ps --filter name=ltesystem && curl -s -m 10 -X POST 127.0.0.1:8081/stop; echo'
