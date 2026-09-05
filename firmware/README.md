# firmware — SDR 射频固件（二进制受控跟踪）

- `uhd/usrp_b210_fpga.stock.bin`（~4.2MB）— 正版 Ettus B210
- `uhd/usrp_b210_fpga.blacksdr_compat.bin`（~3.8MB）— BlackSDR 兼容板（FPGA 型号不同，不可混用）
- `bladerf/` — bladeRF FPGA（`hosted*.rbf`）预留位，有硬件后再放入

容器内通过 `UHD_FPGA=stock|compat|auto`（compose 默认 `compat`）选择，详见 `docs/SDR.md`。
大体积厂商包（`B210mini.zip` 约 1GB）**不入库**，留本地存档。
