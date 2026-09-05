# firmware/uhd — USRP B210 FPGA images

| file | size | source | use |
|---|---|---|---|
| `usrp_b210_fpga.stock.bin` | ~4.2 MB | Ettus/UHD stock (`原始fpga/`) | Genuine B210 |
| `usrp_b210_fpga.blacksdr_compat.bin` | ~3.8 MB | BlackSDR vendor `B210mini/FPGA配置文件/` | Clone/compat B210 (different FPGA) |

Both are tracked in git (small). The 1 GB `B210mini.zip` vendor bundle is NOT tracked;
keep it at `C:\Onedrive\StudyTool\SDR\usrp_b210\blacksdr_b210_mini\B210mini\B210mini.zip`.

Runtime selection (inside container):

```bash
UHD_FPGA=compat docker compose up   # clone board
UHD_FPGA=stock  docker compose up   # genuine board
```

or manually:

```bash
docker exec ltesystem select-uhd-fpga compat
docker restart ltesystem
```

`UHD_IMAGES_DIR=/usr/share/uhd/images` is the canonical UHD lookup path.
