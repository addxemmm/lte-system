#!/usr/bin/env python3
"""One-time, explicitly requested 2.1 alias migration; never delete a manifest."""
import hashlib
import json
import os
from pathlib import Path
import tempfile
import urllib.error
import urllib.request

from registry_check import NoRedirect, Registry
from release_publish import api, run

IMAGE = "addxemmm/lte-system"
VERSION = "2.1"
REVISION = "a449d7cc57e79b5497525a6e12ebec3ab6737f7e"
ALIAS = "sha-" + REVISION
DIGEST = "sha256:09887dcaa527ea5dd503a84705ae11fda0e77b56096186407680b139d150d35e"


def hub_request(path, method="GET", payload=None, token=None):
    # Tag-only Hub API. Never send DELETE to a registry manifest/digest endpoint.
    if method == "DELETE" and path != f"/v2/repositories/{IMAGE}/tags/{ALIAS}/":
        raise ValueError("only the exact legacy SHA alias may be deleted")
    headers = {"Content-Type": "application/json"}
    if token:
        headers["Authorization"] = "Bearer " + token
    data = json.dumps(payload).encode() if payload is not None else None
    req = urllib.request.Request("https://hub.docker.com" + path, data=data,
                                 headers=headers, method=method)
    try:
        with urllib.request.build_opener(NoRedirect).open(req, timeout=30) as response:
            body = response.read()
            return json.loads(body) if body else None
    except urllib.error.HTTPError as error:
        raise ValueError(f"Hub {method} failed: HTTP {error.code}; check token permissions") from None
    except urllib.error.URLError:
        raise ValueError("Hub network/TLS request failed") from None


def verify_registry(registry):
    if registry.digest(VERSION) != DIGEST:
        raise ValueError("retained 2.1 tag digest changed; no deletion allowed")
    alias_digest = registry.digest(ALIAS)
    if alias_digest not in (None, DIGEST):
        raise ValueError("legacy alias has a different digest; no deletion allowed")
    return alias_digest


def migrate(registry, delete=False):
    alias_digest = verify_registry(registry)
    if not alias_digest:
        print("2.1 retained at original digest; legacy SHA alias already absent")
        return
    if not delete:
        print("Verified original 2.1 image; legacy SHA alias exists; no changes made")
        return
    auth = hub_request("/v2/auth/token", "POST", {
        "identifier": os.environ["DOCKERHUB_USERNAME"],
        "secret": os.environ["DOCKERHUB_TOKEN"],
    })
    token = auth.get("access_token")
    if not isinstance(token, str) or not token:
        raise ValueError("Hub login did not return an access token")
    # Recheck immediately before deletion. Never touch VERSION or shared manifest.
    if not verify_registry(registry):
        print("Legacy alias already removed; no changes made")
        return
    hub_request(f"/v2/repositories/{IMAGE}/tags/{ALIAS}/", "DELETE", token=token)
    if verify_registry(registry) is not None:
        raise ValueError("deletion requested but alias still visible; recheck later, do not delete manifest")
    print("Removed only legacy SHA alias; 2.1 retained at original digest")


def main():
    if os.environ.get("GITHUB_ACTIONS") != "true" or os.environ.get("GITHUB_REPOSITORY") != IMAGE:
        raise ValueError("requires this repository's dedicated maintenance workflow")
    if os.environ.get("DOCKERHUB_IMAGE") != IMAGE:
        raise ValueError("configured repository differs from approved migration")
    ref = api(f"repos/{IMAGE}/git/ref/tags/v{VERSION}")
    release = api(f"repos/{IMAGE}/releases/tags/v{VERSION}")
    if ref["object"].get("type") != "commit" or ref["object"].get("sha") != REVISION:
        raise ValueError("original Git tag identity changed")
    if release["draft"] or release["target_commitish"] != REVISION:
        raise ValueError("original published Release identity changed")
    with tempfile.TemporaryDirectory() as folder:
        for name in ("release.json", "release.json.sha256"):
            run("gh", "release", "download", "v" + VERSION, "--repo", IMAGE,
                "--pattern", name, "--dir", folder)
        raw = (Path(folder) / "release.json").read_bytes()
        checksum = (Path(folder) / "release.json.sha256").read_text().strip()
        if checksum != hashlib.sha256(raw).hexdigest() + "  release.json":
            raise ValueError("published metadata checksum differs")
        data = json.loads(raw)
        for key, value in {"version": VERSION, "revision": REVISION, "image": IMAGE, "digest": DIGEST}.items():
            if data.get(key) != value:
                raise ValueError("published metadata identity differs")
    registry = Registry(IMAGE)
    registry.public()
    migrate(registry, delete=os.environ.get("DELETE_LEGACY_ALIAS") == "true")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError) as error:
        raise SystemExit(str(error)) from None
