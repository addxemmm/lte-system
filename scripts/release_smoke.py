#!/usr/bin/env python3
"""Ephemeral management-only image acceptance. Run on a dedicated CI builder."""
import json
import os
from pathlib import Path
import subprocess
import time
import uuid


def docker(*args, check=True):
    result = subprocess.run(["docker", *args], capture_output=True, text=True)
    if check and result.returncode:
        # Avoid dumping commands, environment, or process output containing tokens.
        raise RuntimeError("release image check: Docker operation failed")
    return result


def http(name, path, port=8080, auth=False):
    args = ["exec", name, "curl", "-sS", "--max-time", "3", "-w", "\n%{http_code}"]
    if auth:
        args += ["-H", "Authorization: Bearer release-smoke-only-not-a-secret"]
    result = docker(*args, f"http://127.0.0.1:{port}{path}", check=False)
    if result.returncode:
        return 0, ""
    body, status = result.stdout.rsplit("\n", 1)
    return int(status), body


def main():
    if os.environ.get("GITHUB_ACTIONS") != "true":
        raise ValueError("image smoke requires a dedicated GitHub Actions runner")
    image = os.environ["TEST_IMAGE"]
    out = Path(os.environ["RELEASE_OUT"])
    metadata = json.loads((out / "release.json").read_text())
    labels = json.loads(docker("inspect", image, "--format", "{{json .Config.Labels}}").stdout)
    for key, expected in (("version", metadata["version"]), ("revision", metadata["revision"]), ("source", metadata["source"])):
        if labels.get("org.opencontainers.image." + key) != expected:
            raise ValueError("OCI label differs from release identity: " + key)
    for authenticated in (False, True):
        name = "lte-release-smoke-" + uuid.uuid4().hex[:12]
        try:
            docker("run", "-d", "--name", name, "--network", "none", "--cap-drop=ALL",
                   "--security-opt=no-new-privileges", "--read-only", "--tmpfs", "/data:rw,nosuid,noexec,size=32m",
                   "--entrypoint", "/usr/local/bin/lte-system",
                   "-e", "LTE_UI_LISTEN=127.0.0.1:8080", "-e", "LTE_LISTEN=127.0.0.1:8081",
                   "-e", "LTE_EXPOSE_API=" + str(authenticated).lower(),
                   "-e", "LTE_API_TOKEN=" + ("release-smoke-only-not-a-secret" if authenticated else ""), image)
            for _ in range(30):
                status, body = http(name, "/ui-config.json")
                if status == 200:
                    break
                time.sleep(1)
            assert status == 200, "management process failed to start"
            cfg = json.loads(body)
            assert set(cfg) == {"version", "api_exposed", "api_port", "ui_port", "auth_required"}
            assert cfg["version"] == metadata["version"]
            assert cfg["api_exposed"] is authenticated and cfg["auth_required"] is authenticated
            assert http(name, "/")[0] == 200
            assert http(name, "/api/v1/cell")[0] == (401 if authenticated else 200)
            direct = http(name, "/api/v1/cell", port=8081)[0]
            assert direct == (401 if authenticated else 0), "direct API listener policy failed"
            if authenticated:
                for port in (8080, 8081):
                    status, body = http(name, "/api/v1/cell", port=port, auth=True)
                    assert status == 200 and json.loads(body)["code"] == 0
            processes = docker("top", name, "-eo", "comm").stdout.splitlines()[1:]
            assert processes and all(p.strip() == "lte-system" for p in processes), "unexpected background process"
        finally:
            docker("rm", "-f", name, check=False)
    docker("run", "--rm", "--network", "none", "--entrypoint", "test", image,
           "-s", "/usr/share/lte-system/sources/srsran-source.tar.gz")
    for path in ("/opt/pysim/pySim/cards.py", "/usr/share/lte-system/THIRD_PARTY_NOTICES.md"):
        docker("run", "--rm", "--network", "none", "--entrypoint", "test", image, "-s", path)
    inventory = docker("run", "--rm", "--network", "none", "--entrypoint", "dpkg-query", image,
                       "-W", "-f=${Package}\t${Version}\n").stdout
    (out / "packages-dpkg.txt").write_text(inventory, encoding="utf-8")
    (out / "image-smoke.txt").write_text("PASS: OCI identity; default/API opt-in; metadata; token; UI; only Go process; corresponding source.\nNo RF/USB/host ports/cell startup.\n", encoding="utf-8")
    print("Management-only image smoke: PASS")


if __name__ == "__main__":
    main()
