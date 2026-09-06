# firmware/uhd — USRP B210 FPGA 固件 / FPGA firmware

> 两个子目录各放**一块板子**的镜像，文件名都是上游原始名
> `usrp_b210_fpga.bin`，用**目录名 + SHA256** 区分，改名也认得出来。
> Each subdirectory holds the image for **one board variant** under the
> upstream original filename `usrp_b210_fpga.bin`; identity is established
> by **directory name + SHA256**, robust against renames.

| 子目录 Directory | 对应硬件 Hardware | 大小 Size | SHA256（前 16 位 / first 16） |
|---|---|---|---|
| `ettus-B210-stock/` | Ettus 原版正版 B210 / genuine Ettus B210 | 4,224,632 B | `4AB68AF9D93B2E51…` |
| `blacksdr-B210mini-clone/` | BlackSDR 兼容板 B210mini（FPGA 型号不同）/ clone board (different FPGA) | 3,825,788 B | `8E2ACCE1F987D845…` |

完整校验值见各目录内 `SHA256SUMS`（`sha256sum -c` 可验）。
Full checksums live in each directory's `SHA256SUMS` (verify with `sha256sum -c`).

```bash
cd firmware/uhd/ettus-B210-stock && sha256sum -c SHA256SUMS
cd ../blacksdr-B210mini-clone && sha256sum -c SHA256SUMS
```

运行时选择（二选一，见 [`docs/SDR.md`](../../docs/SDR.md)）/ Runtime selection:
```bash
UHD_FPGA=compat docker compose up -d   # 兼容板 clone board
UHD_FPGA=stock  docker compose up -d   # 正版 genuine board
```

1GB 厂商包 `B210mini.zip` 不入库，留本地存档。
The 1GB vendor bundle `B210mini.zip` is NOT tracked; keep your local copy.

---
**导航 Navigation:** [固件 Firmware](../README.md) · [SDR](../../docs/SDR.md)
