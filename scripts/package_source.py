#!/usr/bin/env python3
"""Export tracked working-tree source, never sibling projects or private state.

New files must be git-added first. No git index, checkout or remote is modified.
"""
import argparse
import hashlib
import io
import os
from pathlib import Path, PurePosixPath
import subprocess
import tarfile

CONFIG_FILES = {
    "app.yaml.example", "user_db.csv.example", "wordlist.list.example",
    "sib.conf", "rb.conf", "rr.conf", "sim_profiles.yaml", "README.md",
}


def included(name):
    path = PurePosixPath(name)
    if path.is_absolute() or ".." in path.parts:
        return False
    if any(p in {".git", ".agents", ".ssh", "data", "var", "bin", "__pycache__"} or p.startswith(".codex") for p in path.parts):
        return False
    if path.name in {"user_db.csv", "wordlist.list", "last_start.json", "ue-sessions.json"}:
        return False
    if path.name == ".env" or path.name.startswith(".env."):
        return False
    if name.startswith("configs/") and (len(path.parts) != 2 or path.name not in CONFIG_FILES):
        return False
    if path.name.endswith(("_run.conf", ".pcap", ".pyc", ".crash", ".tgz", ".tar", ".tar.gz", ".zip")):
        return False
    if path.suffix == ".log" and not name.startswith("docs/samples/"):
        return False
    return True


def package(root, output):
    root = Path(root).resolve()
    output = Path(output).resolve()
    actual = Path(subprocess.check_output(
        ["git", "-C", str(root), "rev-parse", "--show-toplevel"], text=True).strip()).resolve()
    if actual != root or not (root / "go.mod").is_file() or not (root / "deploy/docker/Dockerfile").is_file():
        raise ValueError("root must be the lte-system repository")
    if output.is_relative_to(root):
        raise ValueError("archive output must be outside the repository")
    names = subprocess.check_output(["git", "-C", str(root), "ls-files", "--cached", "-z"])
    with tarfile.open(output, "w:gz") as archive:
        for raw in sorted(set(names.split(b"\0")) - {b""}):
            name = os.fsdecode(raw)
            if not included(name):
                continue
            source = root / name
            if source.is_symlink() or not source.resolve().is_relative_to(root):
                raise ValueError("source symlinks are not allowed: " + name)
            if not source.is_file():
                raise ValueError("tracked source missing; stage its deletion first: " + name)
            if source.resolve() == output:
                raise ValueError("archive output must not be a tracked source file")
            info = archive.gettarinfo(str(source), arcname=name)
            info.uid = info.gid = 0
            info.uname = info.gname = ""
            info.mode = 0o755 if name.endswith(".sh") else 0o644
            content = source.read_bytes()
            # Respect the repository's LF text policy even on a Windows checkout.
            text_suffixes = {".go", ".mod", ".sum", ".sh", ".ps1", ".py", ".md", ".yaml", ".yml",
                             ".conf", ".example", ".csv", ".txt", ".toml", ".json",
                             ".patch", ".cpp", ".cc", ".h", ".hpp", ".cmake"}
            if source.suffix in text_suffixes or source.name in {"Dockerfile", "Makefile", "VERSION", ".dockerignore", ".gitignore", ".gitattributes"}:
                content = content.replace(b"\r\n", b"\n")
            info.size = len(content)
            archive.addfile(info, io.BytesIO(content))
    digest = hashlib.sha256(output.read_bytes()).hexdigest()
    print(digest + "  " + output.name)
    return digest


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--root", default=Path(__file__).resolve().parent.parent)
    parser.add_argument("--output", required=True)
    args = parser.parse_args()
    package(args.root, args.output)
