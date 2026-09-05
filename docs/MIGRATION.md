# 迁移说明：Python/Flask → Go + srsRAN_4G

旧实现见 `legacy-python-workspace/`（`run.py` 548 行 Flask + `run.sh`/`stop.sh` + `supervisord.conf`）：

| 旧 | 新 | 说明 |
|---|---|---|
| `run.py` Flask `:8081` + gunicorn | `cmd/server` + `internal/api`（stdlib） | 9 接口 + `message_id` 原样兼容 |
| `run.sh` echo 生成 conf | `internal/lte` text/template | 修复 `/home/skygo/...` 硬编码；band→EARFCN 表不变 |
| `ps \| grep srs` 状态机 | `internal/sysop` pgrep + 进程组 | 不再误杀同名进程 |
| `os.popen` shell 拼接 | `exec.Command` 数组 + 校验 | 防注入（APN/IMSI/band） |
| `tcpdump &` | Manager 子进程 | stop 联动 kill |
| `tshark … grep -A 7` 下标解析 | `internal/crack` 正则解析 | 容忍 tshark 版本漂移 |
| `hashcat --force &` | `StartAsync` + `--show` | 相同 `-m 4800` |
| 写卡全硬编码 | `internal/sim` + `configs/app.yaml` 默认 | IMSI 必填，其余可选 |
| `user_db.csv[1738:]` 魔数切片 | CSV 解析 + `ueN` 自增 | 并发追加安全 |
| `/home/workspace/log` 硬路径 | `LTE_DATA_DIR`(`/data`) | compose 卷持久化 |
| srsLTE（停更） | srsRAN_4G `release_23_11` | 二进制名不变（`srsenb`/`srsepc`），日志关键字不变 |
| srsLTE `drb.conf`/`drb_config` | srsRAN_4G `rb.conf`/`rb_config` | 上游改名，旧 key 直接导致 eNB 退出 |
| srsLTE `rr.conf`（`meas_report_desc` 旧格式，无 `ul_earfcn`） | 上游 `rr.conf.example` 格式 + 每次按频段渲染 | 旧测量报告格式新版不认；TDD 下 UL 推导失败，必须显式 `ul_earfcn`（FDD=DL+18000，TDD=DL） |
| `ps \| grep` counted 僵尸进程 | `ps -eo pid,stat,comm` 排除 `Z` 状态 + kill 后 `Wait()` 回收 | 崩溃残留的 `<defunct>` 不再误判为运行中 |
| 固定运营商显示名 `srsRAN` | `/start` 新增 `full_net_name`/`short_net_name`（NITZ 下发） | 默认仍是 `srsRAN`，传参即改，`/status` 回显 |
| DNS 写死 `8.8.8.8` | `/start` 新增 `dns` + `default_dns` 配置 | 上行过滤公网 DNS 时可改网关/内网 DNS |
| 只加 MASQUERADE 就上网 | 每次 `/start` 额外确保 `DOCKER-USER` 放行 + TCP MSS 钳制，`/stop` 清理 | Docker 新版默认 FORWARD DROP 会静默丢掉 UE 流量；GTP 路径需要 MSS clamp |

`getfile` 缺文件时旧代码误回 `message_id 2`，新代码按文档返回 `3`（唯一有意的行为修正）。

旧资产对应位置：`legacy-python-workspace/`（旧实现全文 + `conf/user_db.csv` 原始多种子，线上种子已精简为 `configs/user_db.csv.example` 的 ue3 主卡）、`docs/legacy/`（2023 年中文 PDF/MD 存档）、`docs/samples/`（ue3 成功入网日志 + 白卡 ATR）。
