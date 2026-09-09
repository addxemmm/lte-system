#!/usr/bin/env python3
"""Validate a reviewed release ref; prepare nonsecret release metadata."""
import json
import os
from pathlib import Path
import re
import subprocess

VERSION_RE = re.compile(r"(?:2\.1|(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)\.(?:0|[1-9]\d*)(?:-(?:rc|beta|alpha)\.[1-9]\d*)?)\Z")
IMAGE_RE = re.compile(r"[a-z0-9]+(?:[._-][a-z0-9]+)*/[a-z0-9]+(?:[._-][a-z0-9]+)*\Z")


def validate(version, image, ref, sha, tag_sha=None):
    if not VERSION_RE.fullmatch(version):
        raise ValueError("VERSION must be legacy 2.1 or a supported SemVer")
    if not IMAGE_RE.fullmatch(image):
        raise ValueError("DOCKERHUB_IMAGE must be a lowercase namespace/repository without a tag")
    if not re.fullmatch(r"[0-9a-f]{40}", sha):
        raise ValueError("full source commit SHA required")
    tag = "v" + version
    if ref not in ("refs/heads/master", "refs/tags/" + tag):
        raise ValueError("release must run on master or the exact VERSION tag")
    if tag_sha is not None and tag_sha != sha:
        raise ValueError("existing version tag points to another commit; never move it")
    return {"version": version, "tag": tag, "revision": sha, "image": image,
            "source_tag": "sha-" + sha, "prerelease": "-" in version,
            "platform": "linux/amd64", "visibility": "public"}


def git(*args):
    return subprocess.check_output(["git", *args], text=True).strip()


def main():
    version = Path("VERSION").read_text().strip()
    sha = git("rev-parse", "HEAD")
    tag_ref = "refs/tags/v" + version
    probe = subprocess.run(["git", "show-ref", "--verify", "--quiet", tag_ref])
    if probe.returncode not in (0, 1):
        raise ValueError("failed to inspect existing tag")
    tag_sha = git("rev-parse", tag_ref + "^{commit}") if probe.returncode == 0 else None
    data = validate(version, os.environ["DOCKERHUB_IMAGE"], os.environ["GITHUB_REF"], sha, tag_sha)
    subprocess.run(["git", "merge-base", "--is-ancestor", sha, "refs/remotes/origin/master"], check=True)
    if git("status", "--porcelain", "--untracked-files=no"):
        raise ValueError("tracked source must be clean")
    notes = Path("docs/releases") / (version + ".md")
    body = notes.read_text(encoding="utf-8")
    if "## 中文" not in body or "## English" not in body:
        raise ValueError("version release notes must include Chinese and English sections")
    data["source"] = "https://github.com/" + os.environ["GITHUB_REPOSITORY"]
    data["build_url"] = data["source"] + "/actions/runs/" + os.environ["GITHUB_RUN_ID"]
    data["attestations"] = "Not generated; package inventory is not an SBOM or signed provenance"
    out = Path(os.environ["RELEASE_OUT"])
    out.mkdir(parents=True, exist_ok=True)
    (out / "release.json").write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
    (out / "release-notes.md").write_text(body, encoding="utf-8")
    with open(os.environ["GITHUB_OUTPUT"], "a", encoding="utf-8") as output:
        for key in ("version", "tag", "revision", "source_tag"):
            output.write(f"{key}={data[key]}\n")
    print("Validated release identity:", data["tag"], sha, data["image"])


if __name__ == "__main__":
    main()
