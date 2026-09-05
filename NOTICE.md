# Third-party notices

Our own code in this repository (`cmd/`, `internal/`, `configs/*.example`,
`deploy/`, `scripts/`, `docs/`) is MIT licensed (see `LICENSE`).

The Docker image built from `deploy/docker/Dockerfile` additionally bundles
the following third-party software, each under its own license. If you
distribute the built image, you must comply with those licenses (notably the
AGPL-3.0 source-availability requirement for srsRAN — the exact pinned
version is recorded in `Dockerfile` as `ARG SRSRAN_VERSION`).

| Component | License | Source |
|---|---|---|
| srsRAN_4G (srsepc/srsenb, built from source) | AGPL-3.0 | https://github.com/srsran/srsRAN_4G |
| UHD + FPGA images (usrp_b210_fpga stock; BlackSDR compat blob under `firmware/uhd/`) | GPL-3.0 (UHD); vendor blob, redistributable for use with the hardware (`uhd_images_downloader.py` can re-fetch stock) | https://github.com/EttusResearch/uhd |
| pysim (cloned at image build time) | GPL-2.0 | https://github.com/osmocom/pysim |
| Wireshark/tshark | GPL-2.0+ | https://www.wireshark.org |
| hashcat | MIT | https://github.com/hashcat/hashcat |
| Go toolchain + stdlib | BSD-3-Clause | https://go.dev |
| gopkg.in/yaml.v3 | MIT/Apache-2.0 | https://gopkg.in/yaml.v3 |
| Ubuntu 22.04 base + apt packages | various (mostly GPL/LGPL/Apache/MIT, see `/usr/share/doc/*/copyright` in the image) | https://ubuntu.com |

`legacy-python-workspace/pysim/cards.py` is a trimmed copy from the pysim
project (GPL-2.0, © Sylvain Munaut / Harald Welte and contributors).
