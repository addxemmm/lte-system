# firmware 固件 Firmware Images

SDR 射频固件（二进制受控跟踪）。
SDR radio firmware (tracked binaries).

## uhd/ — USRP B210 FPGA（二板两目录，勿混）

- `uhd/ettus-B210-stock/` — **正版 Ettus B210 / genuine Ettus B210**
- `uhd/blacksdr-B210mini-clone/` — **BlackSDR 兼容板 B210mini / clone board**（FPGA 型号不同，不可混用 / different FPGA, not interchangeable）
- 两目录内文件名都是上游原始名 `usrp_b210_fpga.bin`，靠**目录名 + `SHA256SUMS`** 区分（`sha256sum -c` 可验），改名也不怕认错。
  Both directories use the upstream original filename `usrp_b210_fpga.bin`; identity comes from the **directory name + `SHA256SUMS`** (verify with `sha256sum -c`), robust against renames.
- `uhd/README.md` 有对照表（大小/SHA256头/来源/选用）。
  `uhd/README.md` has the comparison table (size/SHA256 prefix/source/selection).
- `bladerf/` — bladeRF FPGA（`hosted*.rbf`）预留位，有硬件后再放入 / bladeRF FPGA (`hosted*.rbf`) placeholder, add files once hardware is available.

容器内通过 `UHD_FPGA=stock|compat|auto`（compose 默认 `compat`）选择，详见 [`docs/SDR.md`](../docs/SDR.md)。
Select inside the container via `UHD_FPGA=stock|compat|auto` (compose defaults to `compat`); see [`docs/SDR.md`](../docs/SDR.md) for details.

大体积厂商包（`B210mini.zip` 约 1GB）**不入库**，留本地存档。
Large vendor bundles (`B210mini.zip`, about 1GB) are **not committed**; keep a local copy.

---
**导航 Navigation:** [仓库根 Repo Root](../README.md) · [UHD 固件详情 UHD Details](uhd/README.md) · [SDR](../docs/SDR.md)
