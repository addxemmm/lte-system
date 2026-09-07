# Postman Collection — LTE-System API v3

导入 `postman/lte-system.postman_collection.json`。集合只含 `/api/v1/*` 标准接口；根路径旧接口与 `/api/v1/ue` 已删除。

Import `postman/lte-system.postman_collection.json`. The collection contains only the standard `/api/v1/*` API; root legacy routes and `/api/v1/ue` were removed.

## 使用 Use

1. 在 collection variables 设置 `base_url=http://HOST:8081`；服务端启用 `LTE_API_TOKEN` 时填写 `token`。
2. 日常启动用第一项 **default: inherit verified profile**，body 固定 `{}`。它继承上次验证可用的 `network`、DNS、UE subnet 与 access policy，不会把现场配置覆盖成 Postman 示例值。
3. 只有首次安装且没有存档时才使用 **first installation template**。先确认 `first_dns` 从 UE 路径可达；`network=auto` 默认解析容器/主机当前命名空间默认路由。
4. 在线状态先查 `GET /api/v1/ues`；`missing|stale|invalid|cell_stopped` 且 `sessions=[]` 表示缺少可靠当前快照，不表示没有 UE。配置授权查 `/subscribers`，两者不是同一概念。
5. “已入网但不能上网”先调用只读 `GET /api/v1/diagnostics/connectivity`。200 是证据结果；并发检查可能返回 429 `diagnostic_busy`。不要用会停止小区的 crack 请求替代状态诊断。
6. subscriber 新增、修改、删除、批量替换和 SIM 写卡都要求小区已停止，否则 409。响应不回显 Key、OP/OPc、AMF、SQN；批量替换保留现有 IMSI 的磁盘最新 SQN。
7. 集合中的 subscriber 凭据变量是不可用占位符；只在本地变量中填入测试卡资料，不要提交真实凭据。

完整语义与错误码见 [`docs/API.md`](../docs/API.md)，机器契约见 [`docs/api/openapi.yaml`](../docs/api/openapi.yaml)。

---
**导航 Navigation:** [仓库根](../README.md) · [API v3](../docs/API.md)
