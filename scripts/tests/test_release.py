import base64
import contextlib
import hashlib
import io
import json
import os
from pathlib import Path
import re
import sys
import tempfile
import unittest
import zipfile
from unittest.mock import patch

SCRIPTS = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(SCRIPTS))
import release_metadata as metadata
import registry_check as registry
import release_publish as publish


class ReleaseTests(unittest.TestCase):
    def valid(self, **kwargs):
        args = dict(version="2.1", image="owner/lte-system", ref="refs/heads/master", sha="a" * 40)
        args.update(kwargs)
        return metadata.validate(**args)

    def test_versions_and_legacy_exception(self):
        for version in ("2.1", "2.1.1", "3.0.0", "2.2.0-rc.1"):
            self.assertEqual(self.valid(version=version)["tag"], "v" + version)
        for version in ("2.2", "02.1.0", "2.1.01", "2.1.0-rc.0", "2.1\nEVIL", "v2.1"):
            with self.assertRaises(ValueError):
                self.valid(version=version)

    def test_ref_image_and_existing_tag_fail_closed(self):
        self.valid(ref="refs/tags/v2.1", tag_sha="a" * 40)
        for args in ({"ref": "refs/heads/evil"}, {"ref": "refs/tags/v2.2.0"},
                     {"image": "owner/repo:latest"}, {"image": "https://host/repo"},
                     {"image": "owner/repo\nKEY=value"}, {"image": "Owner/repo"},
                     {"tag_sha": "b" * 40}, {"sha": "1234"}):
            with self.assertRaises(ValueError):
                self.valid(**args)

    def test_scope_is_for_exact_repository(self):
        data = {"access": [{"type": "repository", "name": "owner/lte-system", "actions": ["pull"]},
                           {"type": "repository", "name": "other/repo", "actions": ["pull", "push"]}]}
        token = "header." + base64.urlsafe_b64encode(json.dumps(data).encode()).decode().rstrip("=") + ".signature"
        self.assertEqual(registry.granted_actions(token, "owner/lte-system"), {"pull"})
        with self.assertRaises(ValueError):
            registry.granted_actions("opaque", "owner/lte-system")

    def test_credentials_are_not_printed_and_insufficient_scope_fails(self):
        token = "h." + base64.urlsafe_b64encode(json.dumps({"access": []}).encode()).decode().rstrip("=") + ".s"
        stdout = io.StringIO()
        with patch.dict(os.environ, {"DOCKERHUB_USERNAME": "user", "DOCKERHUB_TOKEN": "fake-sensitive-token"}), \
                patch.object(registry, "request", return_value=(json.dumps({"token": token}).encode(), {})), \
                contextlib.redirect_stdout(stdout):
            with self.assertRaisesRegex(ValueError, "did not grant"):
                registry.Registry("owner/lte-system")
        self.assertNotIn("fake-sensitive-token", stdout.getvalue())
        self.assertNotIn(token, stdout.getvalue())

    def test_draft_creation_returns_authoritative_id(self):
        import subprocess
        with patch.object(publish.subprocess, "run", return_value=subprocess.CompletedProcess([], 0, '{"id":123,"draft":true}', '')) as command:
            result = publish.api_write("repos/owner/repo/releases", {"tag_name": "v2.1", "draft": True})
            self.assertEqual(result["id"], 123)
            self.assertEqual(json.loads(command.call_args.kwargs["input"])["tag_name"], "v2.1")
        source = (SCRIPTS / "release_publish.py").read_text(encoding="utf-8")
        self.assertIn('releases/{release[\'id\']}', source)
        self.assertNotIn('next(r for page in pages', source)

    def test_redirect_and_visibility_checks(self):
        with self.assertRaises(ValueError):
            registry.NoRedirect().redirect_request(None, None, None, None, None, None)
        instance = registry.Registry.__new__(registry.Registry)
        instance.image = "owner/lte-system"
        for private in (True, None):
            with patch.object(registry, "request", return_value=(json.dumps({"is_private": private}).encode(), {})):
                with self.assertRaises(ValueError):
                    instance.public()

    def test_identity_resume_guards_ignore_only_legacy_source_tag(self):
        original = self.valid()
        self.assertNotIn("source_tag", original)
        self.assertTrue(publish.same_identity(original, dict(original)))
        historical = dict(original, source_tag="sha-" + original["revision"])
        self.assertTrue(publish.same_identity(original, historical))
        self.assertTrue(publish.same_identity(historical, original))
        self.assertFalse(publish.same_identity(original, dict(original, revision="b" * 40)))
        self.assertFalse(publish.labels_match({}, dict(original, source="https://github.com/owner/repo")))

    def test_registry_preflight_checks_only_version_tag(self):
        calls = []

        class FakeRegistry:
            def __init__(self, image):
                self.image = image

            def public(self):
                pass

            def digest(self, tag):
                calls.append(tag)
                return None

        with tempfile.TemporaryDirectory() as folder:
            out = Path(folder)
            data = dict(self.valid(version="2.1.1"), source_tag="sha-" + "a" * 40)
            (out / "release.json").write_text(json.dumps(data), encoding="utf-8")
            with patch.dict(os.environ, {"RELEASE_OUT": str(out), "RELEASE_MODE": "publish"}), \
                    patch.object(registry, "Registry", FakeRegistry), contextlib.redirect_stdout(io.StringIO()):
                registry.main()
        self.assertEqual(calls, ["2.1.1"])

    def _release_data(self):
        return dict(self.valid(version="2.1.1"),
                    source="https://github.com/owner/repo",
                    build_url="https://github.com/owner/repo/actions/runs/123")

    def _write_release_evidence(self, out, data):
        (out / "release.json").write_text(json.dumps(data), encoding="utf-8")
        (out / "release-notes.md").write_text("## 中文\n完成\n\n## English\nDone\n", encoding="utf-8")
        (out / "tested-image-id.txt").write_text("sha256:tested-image\n", encoding="utf-8")
        for name in publish.EVIDENCE_FILES - {"SHA256SUMS", "release.json", "release-notes.md", "tested-image-id.txt"}:
            (out / name).write_text("evidence-" + name, encoding="utf-8")

    def test_publish_uses_only_version_tag(self):
        data = self._release_data()
        digest = "sha256:" + "d" * 64
        registry_tags, commands = [], []
        state = {"digest": None, "tag_created": False}

        class FakeRegistry:
            def __init__(self, image):
                self.image = image

            def public(self):
                pass

            def digest(self, tag):
                registry_tags.append(tag)
                if tag != data["version"]:
                    raise AssertionError("publisher inspected a non-version tag")
                return state["digest"]

        labels = {"org.opencontainers.image." + key: data[key]
                  for key in ("version", "revision", "source")}

        def fake_run(*args):
            commands.append(args)
            if args[:4] == ("gh", "api", "--paginate", "--slurp"):
                return json.dumps([[]])
            if args[:2] == ("docker", "inspect") and args[-1] == "{{json .Config.Labels}}":
                return json.dumps(labels)
            if args[:2] == ("docker", "inspect") and args[-1] == "{{.Id}}":
                return "sha256:tested-image"
            if args[:2] == ("docker", "tag"):
                self.assertEqual(args[-1], data["image"] + ":" + data["version"])
            if args[:2] == ("docker", "push"):
                self.assertEqual(args[-1], data["image"] + ":" + data["version"])
                state["digest"] = digest
            if args[:3] == ("gh", "api", "repos/owner/repo/git/refs"):
                state["tag_created"] = True
            return ""

        def fake_api(path, missing=False):
            if "/git/ref/tags/" in path:
                if not state["tag_created"]:
                    return None
                return {"object": {"type": "commit", "sha": data["revision"]}}
            if path == "repos/owner/repo/releases/1":
                return {"id": 1, "draft": True, "target_commitish": data["revision"], "assets": []}
            raise AssertionError("unexpected API read: " + path)

        def fake_api_write(path, payload, method="POST"):
            if path == "repos/owner/repo/releases" and method == "POST":
                return {"id": 1, "draft": True, "target_commitish": data["revision"], "assets": []}
            if path == "repos/owner/repo/releases/1" and method == "PATCH":
                return {"id": 1, "draft": False}
            raise AssertionError("unexpected API write: " + path)

        with tempfile.TemporaryDirectory() as folder:
            out = Path(folder) / "out"
            out.mkdir()
            self._write_release_evidence(out, data)
            env = {"GITHUB_ACTIONS": "true", "RELEASE_MODE": "publish", "RELEASE_OUT": str(out),
                   "GITHUB_REPOSITORY": "owner/repo", "TEST_IMAGE": "lte-release-test:123",
                   "GITHUB_STEP_SUMMARY": str(Path(folder) / "summary.md")}
            with patch.dict(os.environ, env), patch.object(publish, "Registry", FakeRegistry), \
                    patch.object(publish, "run", side_effect=fake_run), \
                    patch.object(publish, "api", side_effect=fake_api), \
                    patch.object(publish, "api_write", side_effect=fake_api_write), \
                    contextlib.redirect_stdout(io.StringIO()):
                publish.main()
            final = json.loads((out / "release.json").read_text())

        self.assertEqual(set(registry_tags), {data["version"]})
        self.assertEqual(final["digest"], digest)
        self.assertNotIn("source_tag", final)
        self.assertEqual(len([args for args in commands if args[:2] == ("docker", "push")]), 1)

    def _completed_resume(self, published_digest):
        data = self._release_data()
        digest = "sha256:" + "d" * 64
        historical = dict(data, source_tag="sha-" + data["revision"])
        registry_tags, commands = [], []

        class FakeRegistry:
            def __init__(self, image):
                self.image = image

            def public(self):
                pass

            def digest(self, tag):
                registry_tags.append(tag)
                if tag != data["version"]:
                    raise AssertionError("resume inspected a non-version tag")
                return digest

        labels = {"org.opencontainers.image." + key: data[key]
                  for key in ("version", "revision", "source")}
        bundle_name = "build-evidence-" + data["revision"] + ".zip"
        release = {"id": 7, "tag_name": data["tag"], "target_commitish": data["revision"],
                   "draft": False, "assets": [{"name": bundle_name}, {"name": "release.json"}]}

        def fake_run(*args):
            commands.append(args)
            if args[:4] == ("gh", "api", "--paginate", "--slurp"):
                return json.dumps([[release]])
            if args[:3] == ("gh", "release", "download"):
                pattern = args[args.index("--pattern") + 1]
                if pattern == "release.json":
                    target = Path(args[args.index("--dir") + 1]) / pattern
                    target.write_text(json.dumps(dict(historical, digest=published_digest)), encoding="utf-8")
                return ""
            if args[:2] == ("docker", "inspect") and args[-1] == "{{json .Config.Labels}}":
                return json.dumps(labels)
            if args[:2] == ("docker", "inspect") and args[-1] == "{{.Id}}":
                return "sha256:tested-image"
            return ""

        def fake_extract(bundle, out):
            (out / "release.json").write_text(json.dumps(historical), encoding="utf-8")

        def fake_api(path, missing=False):
            if "/git/ref/tags/" in path:
                return {"object": {"type": "commit", "sha": data["revision"]}}
            raise AssertionError("unexpected API read: " + path)

        error = None
        with tempfile.TemporaryDirectory() as folder:
            out = Path(folder) / "out"
            out.mkdir()
            self._write_release_evidence(out, data)
            env = {"GITHUB_ACTIONS": "true", "RELEASE_MODE": "resume", "RELEASE_OUT": str(out),
                   "GITHUB_REPOSITORY": "owner/repo", "TEST_IMAGE": "lte-release-test:456",
                   "GITHUB_STEP_SUMMARY": str(Path(folder) / "summary.md")}
            with patch.dict(os.environ, env), patch.object(publish, "Registry", FakeRegistry), \
                    patch.object(publish, "run", side_effect=fake_run), \
                    patch.object(publish, "api", side_effect=fake_api), \
                    patch.object(publish, "extract_evidence", side_effect=fake_extract), \
                    contextlib.redirect_stdout(io.StringIO()):
                try:
                    publish.main()
                except ValueError as caught:
                    error = caught
        return error, registry_tags, commands, data

    def test_completed_release_single_tag_resumes_without_sha_operations(self):
        digest = "sha256:" + "d" * 64
        error, registry_tags, commands, data = self._completed_resume(digest)
        self.assertIsNone(error)
        self.assertEqual(set(registry_tags), {data["version"]})
        self.assertFalse(any(args[:2] == ("docker", "push") for args in commands))
        self.assertNotIn("sha-" + data["revision"], "\n".join(" ".join(args) for args in commands))

    def test_completed_release_metadata_digest_mismatch_refuses(self):
        error, registry_tags, commands, data = self._completed_resume("sha256:" + "e" * 64)
        self.assertIsNotNone(error)
        self.assertIn("completed release metadata differs", str(error))
        self.assertEqual(set(registry_tags), {data["version"]})
        self.assertFalse(any(args[:2] == ("docker", "push") for args in commands))

    def test_resume_before_first_image_push_uses_original_build(self):
        data = dict(self.valid(), source="https://github.com/owner/repo", build_url="https://github.com/owner/repo/actions/runs/123")
        self.assertEqual(publish.resume_source(data, None), ("artifact", "123"))
        self.assertEqual(publish.resume_source(data, "sha256:" + "a" * 64),
                         ("registry", "owner/lte-system@sha256:" + "a" * 64))
        for url in ("https://github.com/other/repo/actions/runs/123", "https://evil/actions/runs/123", "123\nEVIL"):
            with self.assertRaises(ValueError):
                publish.resume_source(dict(data, build_url=url), None)

    def test_checksum_artifact(self):
        with tempfile.TemporaryDirectory() as folder:
            out = Path(folder)
            (out / "release.json").write_bytes(b"{}")
            publish.checksums(out)
            self.assertEqual((out / "SHA256SUMS").read_text(), hashlib.sha256(b"{}").hexdigest() + "  release.json\n")
            publish.checksums(out)
            self.assertNotIn("  SHA256SUMS", (out / "SHA256SUMS").read_text())

    def test_immutable_evidence_is_validated_before_extract(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            src, dst = root / "src", root / "dst"
            src.mkdir(); dst.mkdir()
            for name in publish.EVIDENCE_FILES - {"SHA256SUMS"}:
                (src / name).write_text("original-" + name)
            publish.checksums(src)
            bundle = root / "evidence.zip"
            with zipfile.ZipFile(bundle, "w") as archive:
                for name in sorted(publish.EVIDENCE_FILES):
                    archive.write(src / name, name)
            publish.extract_evidence(bundle, dst)
            self.assertEqual((dst / "release.json").read_text(), "original-release.json")
            with zipfile.ZipFile(bundle, "w") as archive:
                for name in sorted(publish.EVIDENCE_FILES):
                    archive.writestr(name, b"changed" if name == "release.json" else (src / name).read_bytes())
            with self.assertRaisesRegex(ValueError, "checksum mismatch"):
                publish.extract_evidence(bundle, dst)
            self.assertEqual((dst / "release.json").read_text(), "original-release.json")
            with zipfile.ZipFile(bundle, "w") as archive:
                archive.writestr("../escape", "bad")
            with self.assertRaisesRegex(ValueError, "unexpected"):
                publish.extract_evidence(bundle, dst)

    def test_workflow_has_explicit_publication_and_pinned_actions(self):
        workflow = (SCRIPTS.parent / ".github/workflows/release.yml").read_text()
        self.assertIn("default: verify", workflow)
        self.assertIn("tags: ['v*']", workflow)
        self.assertIn("cancel-in-progress: false", workflow)
        self.assertIn("persist-credentials: false", workflow)
        self.assertIn("actions: read", workflow)
        self.assertNotIn("pull_request_target", workflow)
        self.assertNotIn("ssh ", workflow)
        actions = re.findall(r"uses:\s*(\S+)", workflow)
        self.assertTrue(actions)
        self.assertTrue(all(re.fullmatch(r"[\w-]+/[\w-]+@[0-9a-f]{40}", a) for a in actions))
        build = workflow.split("  build:", 1)[1].split("  publish:", 1)[0]
        self.assertNotIn("secrets.", build)

    def test_image_smoke_never_starts_rf_or_probes_hardware(self):
        smoke = (SCRIPTS / "release_smoke.py").read_text()
        for forbidden in ("--privileged", "/dev/bus/usb", "/api/v1/health", "uhd_find_devices", '"POST"'):
            self.assertNotIn(forbidden, smoke)
        self.assertIn('"--network", "none"', smoke)
        self.assertIn('"--entrypoint", "/usr/local/bin/lte-system"', smoke)
        self.assertIn('"top", name, "-eo", "pid,comm"', smoke)
        self.assertNotIn('"top", name, "-eo", "comm"', smoke)


if __name__ == "__main__":
    unittest.main()
