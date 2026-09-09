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

    def test_redirect_and_visibility_checks(self):
        with self.assertRaises(ValueError):
            registry.NoRedirect().redirect_request(None, None, None, None, None, None)
        instance = registry.Registry.__new__(registry.Registry)
        instance.image = "owner/lte-system"
        for private in (True, None):
            with patch.object(registry, "request", return_value=(json.dumps({"is_private": private}).encode(), {})):
                with self.assertRaises(ValueError):
                    instance.public()

    def test_digest_and_identity_resume_guards(self):
        a, b = "sha256:" + "a" * 64, "sha256:" + "b" * 64
        self.assertEqual(publish.matching_digest(None, a), a)
        self.assertEqual(publish.matching_digest(a, a), a)
        with self.assertRaises(ValueError):
            publish.matching_digest(a, b)
        original = self.valid()
        self.assertTrue(publish.same_identity(original, dict(original)))
        self.assertFalse(publish.same_identity(original, dict(original, revision="b" * 40)))
        self.assertFalse(publish.labels_match({}, dict(original, source="https://github.com/owner/repo")))

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
