# scripts 辅助脚本 Helper Scripts

本地/服务器辅助脚本。源码打包由 `package_source.py` 执行；新文件须先 git add。smoke 仅读取状态。
Helper scripts for local machine and server. `package_source.py` packages tracked source; git-add new files first. Smoke checks are read-only.

- `smoke.sh` — 在**服务器上**对运行中的容器做接口冒烟 / Smoke-test the running container on the **server**: `BASE=http://127.0.0.1:8081 bash scripts/smoke.sh`
- `deploy_to_ubuntu.sh` — Linux/Mac 本地用：上传独立源码快照并在服务器构建（不替换容器） / For Linux/Mac local use: upload a fresh source snapshot and build remotely (no container replacement): `./scripts/deploy_to_ubuntu.sh addx@192.0.2.10`
- `deploy_from_windows.ps1` — Windows 开发机用：打包→scp→远端解压，可选 `-Build` 直接构建（部署deploy） / For Windows dev machine: pack → scp → remote extract, optional `-Build` to build directly (deploy): `.\scripts\deploy_from_windows.ps1 -Build`

---
**导航 Navigation:** [仓库根 Repo Root](../README.md) · [DEPLOY](../docs/DEPLOY.md)
