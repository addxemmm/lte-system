# firmware/bladerf — bladeRF FPGA images (optional)

This project supports bladeRF x40 / xA4 / 2.0 micro via `device_name=bladerf`.
Place the matching `hostedx40.rbf` / `hostedxA4.rbf` / `hosted2.0micro.rbf` in this
directory; the Dockerfile copies them to `/etc/Nuand/bladeRF/` (TBD when a
bladeRF is available for validation).

Until then this directory intentionally only holds this README so the layout
is stable. See `docs/SDR.md` for `bladeRF-cli -e info` verification steps.

---
**导航 Navigation:** [固件 Firmware](../README.md) · [SDR](../../docs/SDR.md)
