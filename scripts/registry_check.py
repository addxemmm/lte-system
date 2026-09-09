#!/usr/bin/env python3
"""Read-only Docker Hub checks; never print credentials, bearer tokens or bodies."""
import base64
import json
import os
from pathlib import Path
import time
import urllib.error
import urllib.parse
import urllib.request

from release_metadata import IMAGE_RE


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise ValueError("registry credential requests must not redirect")


def request(url, headers=None, missing_ok=False):
    req = urllib.request.Request(url, headers=headers or {})
    try:
        with urllib.request.build_opener(NoRedirect).open(req, timeout=30) as response:
            return response.read(), response.headers
    except urllib.error.HTTPError as error:
        if missing_ok and error.code == 404:
            return None, error.headers
        raise ValueError(f"registry request failed with HTTP {error.code}") from None
    except urllib.error.URLError:
        raise ValueError("registry network/TLS check failed") from None


def granted_actions(token, image):
    try:
        segment = token.split(".")[1]
        payload = json.loads(base64.urlsafe_b64decode(segment + "=" * (-len(segment) % 4)))
        return {action for entry in payload.get("access", [])
                if entry.get("type") == "repository" and entry.get("name") == image
                for action in entry.get("actions", [])}
    except (ValueError, IndexError, TypeError, KeyError):
        raise ValueError("unexpected Docker Hub token format; access not established") from None


class Registry:
    def __init__(self, image):
        if not IMAGE_RE.fullmatch(image):
            raise ValueError("invalid image repository")
        self.image = image
        user, secret = os.environ.get("DOCKERHUB_USERNAME", ""), os.environ.get("DOCKERHUB_TOKEN", "")
        if not user or not secret or ":" in user or any(c in user + secret for c in "\r\n"):
            raise ValueError("missing or malformed Docker Hub credentials")
        # Fixed HTTPS auth origin. Never follow a credential-bearing redirect.
        basic = base64.b64encode((user + ":" + secret).encode()).decode()
        query = urllib.parse.urlencode({"service": "registry.docker.io", "scope": f"repository:{image}:pull,push"})
        body, _ = request("https://auth.docker.io/token?" + query, {"Authorization": "Basic " + basic})
        answer = json.loads(body)
        token = answer.get("token") or answer.get("access_token")
        if not isinstance(token, str) or not {"pull", "push"} <= granted_actions(token, image):
            raise ValueError("Docker Hub did not grant both pull and push access")
        self.headers = {"Authorization": "Bearer " + token,
                        "Accept": "application/vnd.oci.image.index.v1+json, application/vnd.docker.distribution.manifest.list.v2+json, application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json"}
        request("https://registry-1.docker.io/v2/", self.headers)
        self.valid_until = time.monotonic() + max(0, int(answer.get("expires_in", 60)) - 60)

    def public(self):
        raw, _ = request("https://hub.docker.com/v2/repositories/" + self.image + "/")
        if json.loads(raw).get("is_private") is not False:
            raise ValueError("repository is not confirmed public; visibility mismatch")

    def digest(self, tag):
        import re
        if not re.fullmatch(r"[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}", tag):
            raise ValueError("invalid registry tag")
        if time.monotonic() >= self.valid_until:
            self.__init__(self.image)
        raw, headers = request(f"https://registry-1.docker.io/v2/{self.image}/manifests/{tag}", self.headers, missing_ok=True)
        if raw is None:
            return None
        digest = headers.get("Docker-Content-Digest", "")
        if not re.fullmatch(r"sha256:[0-9a-f]{64}", digest):
            raise ValueError("registry did not supply a valid manifest digest")
        return digest


def main():
    data = json.loads((Path(os.environ["RELEASE_OUT"]) / "release.json").read_text())
    registry = Registry(data["image"])
    registry.public()
    print("Docker Hub login: OK; granted scopes: pull,push; repository visibility: public")
    # Verification does not upload even a test blob.
    if os.environ.get("RELEASE_MODE") == "publish":
        for tag in (data["version"], data["source_tag"]):
            if registry.digest(tag):
                raise ValueError("release image tag already exists; use verified resume, never overwrite")
    print("Registry preflight passed; no image has been uploaded by this check")


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, json.JSONDecodeError) as error:
        raise SystemExit(str(error)) from None
