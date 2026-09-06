import importlib.util
import os
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location("package_source", SCRIPTS / "package_source.py")
source = importlib.util.module_from_spec(spec)
spec.loader.exec_module(source)


class ComposeListenerTests(unittest.TestCase):
    def test_lan_listener_default_keeps_token_and_data_volume(self):
        compose = (SCRIPTS.parent / "deploy/docker/docker-compose.yml").read_text(encoding="utf-8")
        self.assertIn("LTE_LISTEN: ${LTE_LISTEN:-0.0.0.0:8081}", compose)
        self.assertIn("LTE_API_TOKEN: ${LTE_API_TOKEN:-}", compose)
        self.assertIn("network_mode: host", compose)
        self.assertIn("- lte-data:/data", compose)


class SourcePackageTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.parent = Path(self.temp.name)
        self.root = self.parent / "lte-system"
        self.root.mkdir()
        self.archive = self.parent / "source.tgz"
        subprocess.run(["git", "init", "-q", str(self.root)], check=True)
        for name, content in {
            "go.mod": "module test\ngo 1.22\n",
            "deploy/docker/Dockerfile": "FROM scratch\n",
            "cmd/server/main.go": "package main\n",
            "configs/app.yaml.example": "fixture: true\n",
            "configs/user_db.csv.example": "example\n",
            "configs/app.yaml": "PRIVATE\n",
            "configs/user_db.csv": "PRIVATE\n",
            ".env": "TOKEN=PRIVATE\n",
            "data/wordlist.list": "PRIVATE\n",
            "docs/samples/example.log": "example\n",
            "scripts/test.sh": "#!/bin/bash\nexit 0\n",
        }.items():
            path = self.root / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(content, encoding="utf-8")
        firmware = self.root / "firmware/uhd/test.bin"
        firmware.parent.mkdir(parents=True)
        firmware.write_bytes(b"\x00\xff\r\n")
        (self.root / "scripts").mkdir(exist_ok=True)
        for name in ("package_source.py", "deploy_from_windows.ps1", "deploy_to_ubuntu.sh"):
            shutil.copyfile(SCRIPTS / name, self.root / "scripts" / name)
        subprocess.run(["git", "-C", str(self.root), "add", "-f", "."], check=True,
                       stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        (self.root / "untracked.txt").write_text("PRIVATE")
        (self.parent / "sibling-secret.txt").write_text("PRIVATE")

    def test_tracked_working_tree_only_and_private_data_excluded(self):
        (self.root / "cmd/server/main.go").write_text("updated working tree\n")
        source.package(self.root, self.archive)
        with tarfile.open(self.archive) as archive:
            names = archive.getnames()
            for private in ("configs/app.yaml", "configs/user_db.csv", ".env",
                            "data/wordlist.list", "untracked.txt", "sibling-secret.txt"):
                self.assertNotIn(private, names)
            self.assertIn("configs/user_db.csv.example", names)
            self.assertIn("docs/samples/example.log", names)
            self.assertEqual(archive.extractfile("cmd/server/main.go").read(), b"updated working tree\n")
            self.assertEqual(archive.extractfile("firmware/uhd/test.bin").read(), b"\x00\xff\r\n")
            self.assertEqual(archive.getmember("scripts/test.sh").mode, 0o755)

    def test_reject_parent_directory(self):
        with self.assertRaises((ValueError, subprocess.CalledProcessError)):
            source.package(self.parent, self.archive)

    def test_output_cannot_truncate_source(self):
        path = self.root / "go.mod"
        before = path.read_bytes()
        with self.assertRaisesRegex(ValueError, "outside the repository"):
            source.package(self.root, path)
        self.assertEqual(path.read_bytes(), before)

    def test_missing_tracked_source_is_an_error(self):
        (self.root / "cmd/server/main.go").unlink()
        with self.assertRaisesRegex(ValueError, "tracked source missing"):
            source.package(self.root, self.archive)

    @unittest.skipUnless(shutil.which("pwsh"), "PowerShell 7 not installed")
    def test_windows_script_propagates_copy_and_build_failures(self):
        script = self.root / "scripts/deploy_from_windows.ps1"
        # Native command substitutes; no SSH or server is contacted.
        for failure in ("copy", "build", "none"):
            output = self.parent / (failure + ".tgz")
            code = r'''
function global:scp {
    if ($env:MOCK_FAILURE -eq 'copy') { $global:LASTEXITCODE=23; return }
    [IO.File]::Copy($args[-2], $env:MOCK_ARCHIVE, $true)
    $global:LASTEXITCODE=0
}
function global:ssh {
    if ($env:MOCK_FAILURE -eq 'build' -and $args[-1] -match 'compose.*build') {
        $global:LASTEXITCODE=29
    } else { $global:LASTEXITCODE=0 }
}
& $env:MOCK_SCRIPT -HostAlias TARGET -Build
'''
            env = dict(os.environ, MOCK_FAILURE=failure, MOCK_ARCHIVE=str(output), MOCK_SCRIPT=str(script))
            result = subprocess.run(["pwsh", "-NoProfile", "-NonInteractive", "-Command", code],
                                    env=env, capture_output=True, text=True, encoding="utf-8", errors="replace")
            if failure == "none":
                self.assertEqual(result.returncode, 0, result.stderr)
                with tarfile.open(output) as archive:
                    self.assertIn("go.mod", archive.getnames())
                    self.assertNotIn("lte-system/go.mod", archive.getnames())
            else:
                self.assertNotEqual(result.returncode, 0, result.stdout)

    @unittest.skipIf(os.name == "nt", "POSIX shell mock runs in Linux CI/server")
    def test_bash_script_propagates_remote_failure(self):
        mock = self.parent / "mock-bin"
        mock.mkdir()
        for name, body in {"scp": "exit 0", "ssh": "exit 27"}.items():
            path = mock / name
            path.write_text("#!/bin/sh\n" + body + "\n")
            path.chmod(0o755)
        env = dict(os.environ, PATH=str(mock) + os.pathsep + os.environ["PATH"])
        result = subprocess.run(["bash", str(self.root / "scripts/deploy_to_ubuntu.sh"), "TARGET"],
                                env=env, capture_output=True, text=True, encoding="utf-8", errors="replace")
        self.assertEqual(result.returncode, 27, result.stderr)


if __name__ == "__main__":
    unittest.main()
