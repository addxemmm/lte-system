#!/usr/bin/env python3
"""Publish a tested image once; resume from existing matching digests, not rebuilds."""
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import tempfile
import urllib.parse
import zipfile

from registry_check import Registry


def run(*args):
    result = subprocess.run(args, capture_output=True, text=True)
    if result.returncode:
        # Do not print credentials, derived tokens or arbitrary server responses.
        raise ValueError("release operation failed: " + args[0] + " " + args[1])
    return result.stdout.strip()


def api(path, missing=False):
    result = subprocess.run(["gh", "api", path], capture_output=True, text=True)
    if result.returncode:
        if missing and "HTTP 404" in result.stderr:
            return None
        raise ValueError("GitHub API operation failed")
    return json.loads(result.stdout)


def api_write(path, payload, method="POST"):
    result = subprocess.run(["gh", "api", path, "--method", method, "--input", "-"],
                            input=json.dumps(payload), capture_output=True, text=True)
    if result.returncode:
        raise ValueError("GitHub release mutation failed")
    return json.loads(result.stdout)


def same_identity(expected, actual):
    return all(expected.get(key) == actual.get(key) for key in
               ("version", "tag", "revision", "image", "platform", "visibility", "source"))


def labels_match(labels, data):
    return all(labels.get("org.opencontainers.image." + key) == data[key]
               for key in ("version", "revision", "source"))


def resume_source(data, digest):
    if digest:
        return "registry", data["image"] + "@" + digest
    match = re.fullmatch(re.escape(data["source"]) + r"/actions/runs/([0-9]+)", data.get("build_url", ""))
    if not match:
        raise ValueError("original build run identity is invalid")
    return "artifact", match.group(1)


def file_digest(path):
    result = hashlib.sha256()
    with open(path, "rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            result.update(block)
    return result.hexdigest()


def checksums(out):
    lines = []
    for path in sorted(out.iterdir()):
        if path.is_file() and path.name != "SHA256SUMS":
            lines.append(hashlib.sha256(path.read_bytes()).hexdigest() + "  " + path.name)
    (out / "SHA256SUMS").write_text("\n".join(lines) + "\n", encoding="utf-8")


EVIDENCE_FILES = {"release.json", "release-notes.md", "tested-image-id.txt", "image-smoke.txt", "source.tar.gz", "packages-dpkg.txt", "image-archive.sha256", "SHA256SUMS"}


def extract_evidence(bundle, out):
    with zipfile.ZipFile(bundle) as archive:
        if set(archive.namelist()) != EVIDENCE_FILES or len(archive.namelist()) != len(EVIDENCE_FILES):
            raise ValueError("unexpected evidence archive entries")
        # Validate the whole immutable bundle before replacing any local evidence.
        files = {name: archive.read(name) for name in EVIDENCE_FILES}
    checked = set()
    for line in files["SHA256SUMS"].decode().splitlines():
        value, name = line.split("  ", 1)
        if name not in EVIDENCE_FILES - {"SHA256SUMS"} or name in checked:
            raise ValueError("invalid evidence checksum entry")
        if hashlib.sha256(files[name]).hexdigest() != value:
            raise ValueError("release evidence checksum mismatch")
        checked.add(name)
    if checked != EVIDENCE_FILES - {"SHA256SUMS"}:
        raise ValueError("release checksum coverage is incomplete")
    for name, content in files.items():
        (out / name).write_bytes(content)


def main():
    if os.environ.get("GITHUB_ACTIONS") != "true":
        raise ValueError("publication requires the dedicated GitHub Actions workflow")
    mode = os.environ["RELEASE_MODE"]
    if mode not in ("publish", "resume"):
        raise ValueError("explicit publish/resume mode required")
    out = Path(os.environ["RELEASE_OUT"])
    data = json.loads((out / "release.json").read_text())
    expected = dict(data)
    repo = os.environ["GITHUB_REPOSITORY"]
    tag = data["tag"]
    registry = Registry(data["image"])
    registry.public()
    # Recheck Git tag immediately before any publication (annotated tags supported).
    ref = api(f"repos/{repo}/git/ref/tags/{urllib.parse.quote(tag, safe='')}", missing=True)
    if ref:
        obj = ref["object"]
        for _ in range(8):
            if obj["type"] != "tag":
                break
            obj = api(f"repos/{repo}/git/tags/{obj['sha']}")["object"]
        if obj["type"] != "commit" or obj["sha"] != data["revision"]:
            raise ValueError("remote Git tag points to a different commit")
    # Listing sees drafts too, including drafts without a materialized Git tag.
    releases = json.loads(run("gh", "api", "--paginate", "--slurp", f"repos/{repo}/releases?per_page=100"))
    found = [r for page in releases for r in page if r["tag_name"] == tag]
    if len(found) > 1:
        raise ValueError("ambiguous release records")
    release = found[0] if found else None
    if release and release.get("target_commitish") != data["revision"]:
        raise ValueError("existing release does not name this exact source SHA")
    digest = registry.digest(data["version"])
    image = os.environ["TEST_IMAGE"]
    bundle_name = "build-evidence-" + data["revision"] + ".zip"

    if mode == "resume":
        if not release:
            raise ValueError("resume requires existing release evidence")
        names = {a["name"] for a in release["assets"]}
        if bundle_name not in names:
            raise ValueError("release evidence is incomplete; restore original build artifacts before resuming")
        with tempfile.TemporaryDirectory() as download:
            run("gh", "release", "download", tag, "--repo", repo, "--pattern", bundle_name, "--dir", download)
            extract_evidence(Path(download) / bundle_name, out)
        data = json.loads((out / "release.json").read_text())
        if not same_identity(expected, data):
            raise ValueError("existing release source identity differs from selected ref")
        if data.get("digest") not in (None, digest):
            raise ValueError("recorded digest differs from registry")
        kind, source = resume_source(data, digest)
        if kind == "registry":
            image = source
            run("docker", "pull", image)
        else:
            # Failure after evidence upload but before the first push: reuse the
            # original run's transport archive, never rebuild under the same tag.
            with tempfile.TemporaryDirectory() as download:
                run("gh", "run", "download", source, "--repo", repo, "--name", "tested-image", "--dir", download)
                archive = Path(download) / "image.tar.gz"
                if file_digest(archive) != (out / "image-archive.sha256").read_text().strip():
                    raise ValueError("original image archive checksum mismatch")
                run("docker", "load", "--input", str(archive))
            image = "lte-release-test:" + source
    elif digest:
        raise ValueError("image already exists; select resume to verify it without rebuilding/overwriting")

    labels = json.loads(run("docker", "inspect", image, "--format", "{{json .Config.Labels}}"))
    if not labels_match(labels, data):
        raise ValueError("image OCI identity does not match selected source")
    image_id = run("docker", "inspect", image, "--format", "{{.Id}}")
    if image_id != (out / "tested-image-id.txt").read_text().strip():
        raise ValueError("image is not the exact smoke-tested image")
    if release and not release["draft"]:
        if mode != "resume" or not ref or not digest:
            raise ValueError("published release is immutable to this workflow")
        with tempfile.TemporaryDirectory() as download:
            run("gh", "release", "download", tag, "--repo", repo, "--pattern", "release.json", "--dir", download)
            published = json.loads((Path(download) / "release.json").read_text())
            if not same_identity(data, published) or published.get("digest") != digest:
                raise ValueError("completed release metadata differs from its image")
        print("Release already complete; digest/source verified; no changes made")
        return
    if not release:
        release = api_write(f"repos/{repo}/releases", {
            "tag_name": tag, "target_commitish": data["revision"], "draft": True,
            "name": tag + " — LTE System / LTE 系统", "prerelease": data["prerelease"],
            "body": (out / "release-notes.md").read_text(encoding="utf-8")})

    def upload_once(path):
        # Never --clobber: an interrupted update must not destroy prior evidence.
        # Draft tag/list indexes may lag creation. The returned numeric ID is
        # authoritative and does not depend on a newly materialized tag/index.
        current = api(f"repos/{repo}/releases/{release['id']}")
        if any(asset["name"] == path.name for asset in current["assets"]):
            with tempfile.TemporaryDirectory() as download:
                run("gh", "release", "download", tag, "--repo", repo, "--pattern", path.name, "--dir", download)
                if hashlib.sha256((Path(download) / path.name).read_bytes()).digest() != hashlib.sha256(path.read_bytes()).digest():
                    raise ValueError("existing immutable release attachment differs: " + path.name)
        else:
            run("gh", "release", "upload", tag, str(path), "--repo", repo)

    if mode != "resume":
        checksums(out)
        bundle = out.parent / bundle_name
        with zipfile.ZipFile(bundle, "w", compression=zipfile.ZIP_DEFLATED) as archive:
            for name in sorted(EVIDENCE_FILES):
                entry = zipfile.ZipInfo(name, date_time=(2020, 1, 1, 0, 0, 0))
                archive.writestr(entry, (out / name).read_bytes(), compress_type=zipfile.ZIP_DEFLATED)
        upload_once(bundle)

    # A draft with original build evidence exists before the first registry mutation.
    registry.public()
    image_tag = data["version"]
    existing = registry.digest(image_tag)
    if existing:
        if mode != "resume" or existing != digest:
            raise ValueError("refusing to overwrite an existing image tag")
    else:
        destination = data["image"] + ":" + image_tag
        run("docker", "tag", image, destination)
        run("docker", "push", destination)
        uploaded = registry.digest(image_tag)
        if not uploaded or (digest and uploaded != digest):
            raise ValueError("uploaded image digest mismatch")
        digest = uploaded
        # Digest lives in the registry; original build evidence stays immutable.
    assert digest and registry.digest(data["version"]) == digest
    # No login config in this subprocess: confirm the PUBLIC image is retrievable.
    with tempfile.TemporaryDirectory() as empty_config:
        run("docker", "--config", empty_config, "pull", data["image"] + "@" + digest)
    if run("docker", "inspect", data["image"] + "@" + digest, "--format", "{{.Id}}") != image_id:
        raise ValueError("public pull differs from tested image")
    data["digest"] = digest
    final_metadata = out / "release.json"
    final_metadata.write_text(json.dumps(data, indent=2) + "\n", encoding="utf-8")
    upload_once(final_metadata)
    checksum = out / "release.json.sha256"
    checksum.write_text(hashlib.sha256(final_metadata.read_bytes()).hexdigest() + "  release.json\n", encoding="utf-8")
    upload_once(checksum)
    body = (out / "release-notes.md").read_text(encoding="utf-8")
    body += f"\n\n## 构建记录 / Build record\n\n- Commit: `{data['revision']}`\n- Image: `{data['image']}@{digest}`\n- Platform: `{data['platform']}`\n- Build: {data['build_url']}\n- Verification: management-only; no RF/handset acceptance.\n"
    (out / "final-notes.md").write_text(body, encoding="utf-8")
    # Re-read the draft immediately before completion. Create a missing tag at
    # the exact verified commit, never let a changed draft target select it.
    current = api(f"repos/{repo}/releases/{release['id']}")
    if not current["draft"] or current.get("target_commitish") != data["revision"]:
        raise ValueError("release draft changed during publication")
    final_ref = api(f"repos/{repo}/git/ref/tags/{urllib.parse.quote(tag, safe='')}", missing=True)
    if final_ref is None:
        run("gh", "api", f"repos/{repo}/git/refs", "-f", "ref=refs/tags/" + tag, "-f", "sha=" + data["revision"])
        final_ref = api(f"repos/{repo}/git/ref/tags/{urllib.parse.quote(tag, safe='')}")
    final_obj = final_ref["object"]
    for _ in range(8):
        if final_obj["type"] != "tag":
            break
        final_obj = api(f"repos/{repo}/git/tags/{final_obj['sha']}")["object"]
    if final_obj["type"] != "commit" or final_obj["sha"] != data["revision"]:
        raise ValueError("final Git tag identity mismatch")
    api_write(f"repos/{repo}/releases/{release['id']}", {"body": body, "draft": False, "make_latest": "false"}, method="PATCH")
    with open(os.environ["GITHUB_STEP_SUMMARY"], "a", encoding="utf-8") as summary:
        summary.write(f"## 发布完成 / Published\n\n`{data['image']}@{digest}`\n\nhttps://github.com/{repo}/releases/tag/{tag}\n\nLTE/GSM not started or deployed.\n")
    print("Published release and verified public image:", tag, digest)


if __name__ == "__main__":
    try:
        main()
    except (ValueError, KeyError, AssertionError) as error:
        raise SystemExit(str(error) or "release verification failed") from None
