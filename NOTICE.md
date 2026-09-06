# 第三方声明 Third-party Notices

本仓库自有代码（[`cmd/`](cmd)、[`internal/`](internal)、`configs/*.example`、[`deploy/`](deploy)、[`scripts/`](scripts)、[`docs/`](docs)）为 MIT 许可（见 `LICENSE`）。
Our own code in this repository ([`cmd/`](cmd), [`internal/`](internal), `configs/*.example`, [`deploy/`](deploy), [`scripts/`](scripts), [`docs/`](docs)) is MIT licensed (see `LICENSE`).

由 [`deploy/docker/Dockerfile`](deploy/docker/Dockerfile) 构建的 Docker 镜像还捆绑以下第三方软件，各自遵循其许可。如分发构建产物，必须遵守这些许可（特别是 srsRAN 的 AGPL-3.0 源码可得性要求——确切锁定版本记录在 `Dockerfile` 的 `ARG SRSRAN_VERSION`）。
The Docker image built from [`deploy/docker/Dockerfile`](deploy/docker/Dockerfile) additionally bundles the following third-party software, each under its own license. If you distribute the built image, you must comply with those licenses (notably the AGPL-3.0 source-availability requirement for srsRAN — the exact pinned version is recorded in `Dockerfile` as `ARG SRSRAN_VERSION`).

| 组件 Component | 许可 License | 来源 Source |
|---|---|---|
| srsRAN_4G（`srsepc/srsenb`，从源码构建 / built from source） | AGPL-3.0 | https://github.com/srsran/srsRAN_4G |
| UHD + FPGA 固件firmware镜像（`usrp_b210_fpga` 原版 / stock；BlackSDR 兼容板compatible board 二进制位于 [`firmware/uhd/`](firmware/uhd) 下 / compat blob under [`firmware/uhd/`](firmware/uhd)) | GPL-3.0 (UHD); vendor blob, redistributable for use with the hardware (`uhd_images_downloader.py` can re-fetch stock) | https://github.com/EttusResearch/uhd |
| pysim（镜像构建时克隆 / cloned at image build time） | GPL-2.0 | https://github.com/osmocom/pysim |
| Wireshark/tshark 抓包packet capture工具 / Wireshark/tshark packet capture tools | GPL-2.0+ | https://www.wireshark.org |
| hashcat 字典wordlist破解 / hashcat wordlist cracking | MIT | https://github.com/hashcat/hashcat |
| Go 工具链 + 标准库 / Go toolchain + stdlib | BSD-3-Clause | https://go.dev |
| gopkg.in/yaml.v3 | MIT/Apache-2.0 | https://gopkg.in/yaml.v3 |
| Ubuntu 22.04 基础 + apt 包 / Ubuntu 22.04 base + apt packages | various (mostly GPL/LGPL/Apache/MIT, see `/usr/share/doc/*/copyright` in the image) | https://ubuntu.com |
| Custom `testsim` card logic 定制 `testsim` 卡逻辑 | GPL-2.0 (see `third_party/pysim/`) | https://github.com/osmocom/pysim |
`third_party/pysim/cards.py` (GPL-2.0, rescued from the proven 2023 setup) overlays era-pinned upstream pysim in the image (see [`Dockerfile`](deploy/docker/Dockerfile)).
定制 `testsim` 卡逻辑（GPL-2.0，源自 2023 年实测环境）覆盖 era 版上游 pysim，见 [`Dockerfile`](deploy/docker/Dockerfile)。

[`third_party/srsran/`](third_party/srsran) contains AGPL-3.0-or-later patches and regression tests for the pinned srsRAN source. These modifications are part of the corresponding source for the bundled EPC; they are not covered by the repository's MIT license.
[`third_party/srsran/`](third_party/srsran) 中的补丁和回归测试采用 AGPL-3.0-or-later，属于镜像中 EPC 的对应修改源码，不属于本仓库自有代码的 MIT 许可范围。
