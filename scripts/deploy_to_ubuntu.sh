#!/bin/bash
# Export tracked working-tree source and build on the SDR host; NEVER restart the cell.
# Usage: scripts/deploy_to_ubuntu.sh user@TARGET
# New source files must be git-added first. Requires remote Docker group membership.
set -euo pipefail
HOST="${1:?usage: deploy_to_ubuntu.sh user@TARGET}"
REPO_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ARCHIVE="$(mktemp "${TMPDIR:-/tmp}/lte-source-XXXXXXXX.tgz")"
trap 'rm -f -- "$ARCHIVE"' EXIT
python3 "$REPO_DIR/scripts/package_source.py" --root "$REPO_DIR" --output "$ARCHIVE"
SHA="$(sha256sum "$ARCHIVE" | cut -d ' ' -f 1)"
NAME="$(basename "$ARCHIVE")"
RELEASE_DIR="lte-releases/${NAME%.tgz}"
scp -o BatchMode=yes "$ARCHIVE" "$HOST:$NAME"
ssh -o BatchMode=yes "$HOST" "set -eu; cd; printf '%s  %s\n' '$SHA' '$NAME' | sha256sum -c -; mkdir -p lte-releases; mkdir '$RELEASE_DIR'; tar --no-same-owner --no-same-permissions -xzf '$NAME' -C '$RELEASE_DIR'; rm -f -- '$NAME'; echo RELEASE_DIR=\$HOME/$RELEASE_DIR; cd '$RELEASE_DIR'; docker compose -p docker -f deploy/docker/docker-compose.bridge.yml build"
