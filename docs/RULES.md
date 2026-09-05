# 使用规则（必读）

## 1. 无状态的含义

本工具**无数据库、无账号、无后台任务**：所有状态只 living 在三个地方：

| 状态 | 位置 | 生命周期 |
|---|---|---|
| 本次启动参数（band/apn/mcc/mnc/network/dns/名字/增益） | `/data/last_start.json`（容器卷 `lte-data`） | 随卷持久化，重建容器不丢 |
| 用户卡库 `user_db.csv`、字典 `wordlist.list` | `/data/conf/`、`/data/` | 同上，`/userupload` 可替换 |
| 运行态（EPC/eNB 进程、已附着 UE、抓包） | 内存 + `/data/log/` | `/stop` 或容器重建即清零 |

换言之：**配一次，之后可复用；停就全停，不留幽灵进程。**

## 2. 标准操作流

```bash
# 开机（每天/每次上电后只需一次）
curl -X POST http://127.0.0.1:8081/start -H 'Content-Type: application/json' -d '{}'
#   ^ 空 body = 复用上次配置（last_start.json）。首次无存档会返回参数不全。
#   改个别参数也行：只传想改的，其余自动继承上次，例如只换频段：
#   curl ... -d '{"band":"3"}'

# 看状态 / 存档
curl http://127.0.0.1:8081/status
curl http://127.0.0.1:8081/profile   # 存档配置 + 卡库清单（不含密钥）

# 关机
curl -X POST http://127.0.0.1:8081/stop -H 'Content-Type: application/json' -d '{}'
```

- 同一时间只允许一组 EPC/eNB 运行；`/start` 在运行中会返回 `is running`，先 `/stop`。
- `/stop` 会联动杀掉抓包并清理本工具加的 iptables 规则，不碰你宿主其余规则。
- 容器重建（`up -d --force-recreate`）后**不会自动发射**（合规要求），手动 `/start`（空 body 一键恢复）。

## 3. 配置从哪来、优先级

`/start` 参数（最高）→ 存档 `last_start.json`（空字段继承）→ 服务端默认（`configs/app.yaml.example`）。

`sib/rb.conf` 跟随镜像，`rr.conf` 每次按频段渲染，`user_db.csv`/`wordlist` 只有缺失才 seeding——**你的卡库永远不会被升级覆盖**。

## 4. 多终端说明

- 第一台附着的终端独占上网（srsRAN SPGW 限制），其余可附着但无数据——这是上游特性，不是 bug。
- 抓包（`getfile`）与 `basicinfo` 只反映第一台终端。
- 换卡/加卡：有写卡器调 `/writesim`（自动追加卡库）；无写卡器手写一行调 `/userupload`，**重启生效**（EPC 只在启动时读库）。

## 5. 升级与回滚

```bash
cd ~/lte-system && git pull
sudo docker compose -f deploy/docker/docker-compose.yml up -d --build
# 回滚：旧镜像 ltesystem-dep:1.0 与容器 ltesystem-v1-backup 一直保留在宿主机
```

升级不丢 `/data` 卷（配置/卡库/抓包都在）。大版本镜像另有 tarball 备份（见 `docs/DEPLOY.md`）。

## 6. 射频纪律

- 无线发射前确认频段、天线（TX/RX 口必接），人体远离天线。
- 虚拟机 USB 下默认 `n_prb 25`（5MHz）；切 `50/100` 前先看 CPU 与 `timed out` 计数（见 `docs/SDR.md`）。
- 长期无人值守不要开着发射：测完 `/stop`。
