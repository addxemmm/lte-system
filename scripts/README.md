# scripts — 本地/服务器辅助脚本

- `smoke.sh` — 在**服务器上**对运行中的容器做接口冒烟：`BASE=http://127.0.0.1:8081 bash scripts/smoke.sh`
- `deploy_to_ubuntu.sh` — 在**本地**把代码同步到服务器并重建容器：`./scripts/deploy_to_ubuntu.sh addx@192.168.100.199`
