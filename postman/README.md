# postman — Postman 集合 Postman Collection

## 导入 Import

Postman → Import → 选择 `postman/lte-system.postman_collection.json`。
Postman → Import → select the file above.

## 使用 Use

1. 右键集合 → Edit → Variables：`base_url` 改成你的服务器地址
   （服务器本机 `http://127.0.0.1:8081`，局域网如 `http://192.0.2.10:8081`）。
   Set `base_url` to your server in collection Variables.
2. `token` 留空即可（开放模式）；服务端设了 `LTE_API_TOKEN` 才填，
   集合会自动带 `Authorization: Bearer {{token}}`。
   Leave `token` empty for open mode.
3. 先跑 `Legacy` 文件夹冒烟，再用 `v1 标准` 做新集成；上传类请求先在本地备好文件。
   Smoke-test with `Legacy` first, then build on `v1`.
4. 文件夹自带断言：Legacy 断 HTTP 200 + JSON，v1 断 2xx + 包络字段。
   Folder-level tests assert status codes and the envelope.

---
**导航 Navigation:** [仓库根 Repo Root](../README.md) · [API v1](../docs/API.md) · [旧版API Legacy](../docs/API_LEGACY.md)
