# 使用规则（必读） Usage Rules (Must Read)

## 1. 无状态的含义 What "Stateless" Means

本工具**无数据库、无账号、无后台任务**：所有状态只存在于三个地方：
All state lives in only three places (no database, no accounts, no background jobs):
This tool has **no database, no accounts, no background jobs**: all state lives in only three places:

| 状态 State | 位置 Location | 生命周期 Lifecycle |
|---|---|---|
| 本次启动参数（band/apn/mcc/mnc/network/dns/名字/增益） / Current boot params (band/apn/mcc/mnc/network/dns/names/gains) | `/data/last_start.json`（容器卷 `lte-data`） / `/data/last_start.json` (container volume `lte-data`) | 随卷持久化，重建容器不丢 / Persists with the volume, survives container rebuilds |
| 用户卡库 `user_db.csv`、字典 `wordlist.list` / SIM database `user_db.csv`, wordlist `wordlist.list` | `/data/conf/`、`/data/` / `/data/conf/`, `/data/` | 同上，`/userupload` 可替换 / Same as above, replaceable via `/userupload` |
| 运行态（EPC/eNB 进程、已附着 UE、抓包） / Runtime state (EPC/eNB processes, attached UEs, packet capture) | 内存 + `/data/log/` / Memory + `/data/log/` | `/stop` 或容器重建即清零 / Cleared on `/stop` or container rebuild |

换言之：**配一次，之后可复用；停就全停，不留幽灵进程。**
In other words: **configure once, then reuse; stop means everything stops, no ghost processes.**

## 2. 标准操作流 Standard Operation Flow

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

开机（每天/每次上电后只需一次）：空 body 即复用上次配置（`last_start.json`）。首次无存档会返回参数不全。
Boot (once per day / once per power-on): empty body reuses the last config (`last_start.json`). The first call with no saved profile returns missing-params.

改个别参数也行：只传想改的，其余自动继承上次，例如只换频段（`-d '{"band":"3"}'`）。
To change individual params, send only what you want to change; the rest inherit from last time, e.g. only switch band (`-d '{"band":"3"}'`).

看状态/存档：`curl http://127.0.0.1:8081/status` 看运行态；`curl http://127.0.0.1:8081/profile` 看存档配置 + 卡库清单（不含密钥）。
Check status / saved profile: `curl http://127.0.0.1:8081/status` for runtime; `curl http://127.0.0.1:8081/profile` for the saved profile + SIM inventory (no secrets).

- 同一时间只允许一组 EPC/eNB 运行；`/start` 在运行中会返回 `is running`，先 `/stop`。 / Only one EPC/eNB set may run at a time; `/start` while running returns `is running`, so `/stop` first.
- `/stop` 会联动杀掉抓包并清理本工具加的 iptables 规则，不碰你宿主其余规则。 / `/stop` also kills packet capture and cleans up iptables rules added by this tool, leaving your other host rules alone.
- 容器重建（`up -d --force-recreate`）后**不会自动发射**（合规要求），手动 `/start`（空 body 一键恢复）。 / After a container rebuild (`up -d --force-recreate`) it will **not transmit automatically** (compliance requirement); run `/start` manually (empty body restores in one shot).

## 3. 配置从哪来、优先级 Where Config Comes From, Priority

`/start` 参数（最高）→ 存档 `last_start.json`（空字段继承）→ 服务端默认（`configs/app.yaml.example`）。
`/start` params (highest) → saved profile `last_start.json` (empty fields inherit) → server defaults (`configs/app.yaml.example`).

`sib/rb.conf` 跟随镜像，`rr.conf` 每次按频段渲染，`user_db.csv`/`wordlist` 只有缺失才 seeding——**你的卡库永远不会被升级覆盖**。
`sib/rb.conf` follows the image, `rr.conf` is re-rendered per band each time, and `user_db.csv`/`wordlist` are seeded only when missing — **your SIM database is never overwritten by upgrades**.

## 4. 多终端说明 Multiple UEs

- 第一台附着的终端独占上网（srsRAN SPGW 限制），其余可附着但无数据——这是上游特性，不是 bug。 / The first attached UE gets exclusive internet access (srsRAN SPGW limit); others may attach but get no data — this is an upstream trait, not a bug.
- 抓包（`getfile`）与 `basicinfo` 只反映第一台终端。 / Packet capture (`getfile`) and `basicinfo` only reflect the first UE.
- 换卡/加卡：有写卡器调 `/writesim`（自动追加卡库）；无写卡器手写一行调 `/userupload`，**重启生效**（EPC 只在启动时读库）。 / Change/add cards: with a SIM writer call `/writesim` (auto-appends to the database); without one, hand-write a line and call `/userupload`; takes effect **after restart** (EPC reads the database only at boot).

## 5. 升级与回滚 Upgrade and Rollback

```bash
cd ~/lte-system && git pull
sudo docker compose -f deploy/docker/docker-compose.yml up -d --build
# 回滚：旧镜像 ltesystem-dep:1.0 与容器 ltesystem-v1-backup 一直保留在宿主机
```

升级不丢 `/data` 卷（配置/卡库/抓包都在）。大版本镜像另有 tarball 备份（见 `docs/DEPLOY.md`）。
Upgrades keep the `/data` volume (config/SIM database/packet capture all stay). Major images also have tarball backups (see `docs/DEPLOY.md`).

回滚：旧镜像 `ltesystem-dep:1.0` 与容器 `ltesystem-v1-backup` 一直保留在宿主机。
Rollback: the old image `ltesystem-dep:1.0` and container `ltesystem-v1-backup` stay on the host.

## 6. 射频纪律 Radio Discipline

- 无线发射前确认频段、天线（TX/RX 口必接），人体远离天线。 / Before transmitting, confirm the band and antennas (TX/RX port required); keep bodies away from antennas.
- 虚拟机 USB 下默认 `n_prb 25`（5MHz）；切 `50/100` 前先看 CPU 与 `timed out` 计数（见 `docs/SDR.md`）。 / Under VM USB, default `n_prb 25` (5MHz); before switching to `50/100`, check CPU and the `timed out` count (see `docs/SDR.md`).
- 长期无人值守不要开着发射：测完 `/stop`。 / Do not leave the transmitter on unattended for long: `/stop` when testing is done.

---
**导航 Navigation:** [文档索引 Docs](README.md) · [QUICKSTART](QUICKSTART.md) · [RULES](RULES.md) · [API v1](API.md) · [旧版API Legacy](API_LEGACY.md) · [DEPLOY](DEPLOY.md) · [SIM](SIM.md) · [SDR](SDR.md) · [MIGRATION](MIGRATION.md)
