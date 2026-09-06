# BlackSDR 兼容板 B210mini FPGA / BlackSDR clone B210mini FPGA

- 文件 File: `usrp_b210_fpga.bin`（3,825,788 字节 bytes）
- 来源 Source: 板卡厂商提供的 `B210mini/FPGA配置文件/`（原始大包 `B210mini.zip` 未入库，留本地）
  vendor-supplied `B210mini/FPGA配置文件/` (original 1GB `B210mini.zip` not tracked, keep locally)
- 适用 Hardware: BlackSDR 兼容板 B210mini（FPGA 型号与原版不同，不可混用）
  BlackSDR clone B210mini (different FPGA model; NOT interchangeable with stock)
- 选用 Select: `UHD_FPGA=compat`（本机默认，见 `docker-compose.yml`）
  `UHD_FPGA=compat` (default on our host, see `docker-compose.yml`)

```bash
sha256sum -c SHA256SUMS   # 必须 OK / must print OK
```

---
**导航 Navigation:** [UHD 固件 UHD Firmware](../README.md) · [SDR](../../../docs/SDR.md)
