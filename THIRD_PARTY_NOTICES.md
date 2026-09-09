# 镜像组件与对应源码 / Image components and corresponding source

The repository's MIT license applies to its original code, not to every program or firmware file in the combined image. Component licenses remain in force. This inventory is not a claim of complete license clearance or a software bill of materials.

本仓库原创代码的 MIT 许可不覆盖组合镜像的所有程序和固件。各组件仍适用自身许可；本文是材料索引，不代表全部许可审查已完成，也不是 SBOM。

## srsRAN_4G

- Upstream: https://github.com/srsran/srsRAN_4G
- Baseline: `eea87b1d893ae58e0b08bc381730c502024ae71f` (`release_23_11`).
- License: GNU AGPL v3; see the upstream license inside the source archive.
- Exact patched source, including local modifications/tests and the upstream license, is bundled at `/usr/share/lte-system/sources/srsran-source.tar.gz` in full release images. The release build recipe and local patches are bundled alongside it as `build-materials.tar.gz`.
- 完整发布镜像内包含与编译版本匹配的修改后源码、测试、原许可和构建材料；公开镜像用户不需要访问私有 GitHub 仓库即可提取。

## pySIM

- Upstream: https://github.com/osmocom/pysim
- Baseline: `263fb0871c0c8b6b3cb58eaa1ca1779ce1adf6c4`.
- Complete checked-out source, license notices and the modified `pySim/cards.py` are present at `/opt/pysim`. Refer to its COPYING/source headers for GPL and file-specific notices.
- 完整检出源码及本地 cards.py 修改随镜像保留，参阅对应许可文件和源码头。

## Ubuntu, Python and other runtime components

Ubuntu packages retain notices under `/usr/share/doc/<package>/copyright`. Python distribution metadata and licenses remain under their installation directories. Each GitHub Release records the installed Debian package inventory; this is not a replacement for dependency/license review or complete corresponding-source obligations. Assess matching source availability before redistributing modified or no-longer-available dependencies.

Ubuntu 与 Python 组件保留自己的许可材料；Release 的 Debian 包清单不替代依赖审查或完整对应源码要求。重新分发修改或已下架的依赖前，应核对匹配源码的可获得性。

## FPGA firmware

Stock and BlackSDR compatibility images have separate provenance and SHA-256 records under the repository's `firmware/uhd/` tree, also included in build materials. The BlackSDR file is vendor supplied. A file's presence or checksum does not itself grant redistribution rights; the publisher must retain applicable vendor permission/license evidence. Do not describe the combined firmware distribution as MIT-licensed.

原版和兼容 FPGA 有各自来源及校验和；兼容镜像来自板卡厂商。文件和哈希本身不授予再分发权，发布者应保留相应厂商授权/许可证据，不应将组合固件标为 MIT。

## Extract without starting services / 不启动服务地提取

On a machine with Docker, create (do not start) a container from the chosen immutable image digest, copy `/usr/share/lte-system/` and `/opt/pysim`, then remove that temporary container. Creation/copying requires no USB, privileged mode or radio startup. This does not change the existing LTE/GSM containers.

在有 Docker 的机器上从指定 digest 创建但不启动临时容器，复制上述目录后删除该临时容器即可；无须 USB、特权或射频启动，不修改既有 LTE/GSM 容器。
