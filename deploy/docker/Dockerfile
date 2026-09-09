# syntax=docker/dockerfile:1
# lte-system (srsRAN_4G + Go tool API)
# Base: Ubuntu 22.04 (gcc-11 works with srsRAN_4G; 24.04 needs gcc-11 downgrade).
# Pin the srsRAN version here; image version lives in VERSION at repo root.
ARG SRSRAN_VERSION=release_23_11
ARG OCI_VERSION=2.1
ARG OCI_REVISION=unknown
ARG OCI_SOURCE=https://github.com/addxemmm/lte-system

# ---------- stage 1: build srsRAN_4G ----------
FROM ubuntu:22.04 AS srs-builder
ARG SRSRAN_VERSION
ARG SRSRAN_BUILD_JOBS=2
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential cmake git pkg-config \
    libfftw3-dev libmbedtls-dev libboost-program-options-dev \
    libboost-system-dev libboost-thread-dev libboost-test-dev \
    libconfig++-dev libsctp-dev \
    libuhd-dev uhd-host libzmq3-dev \
    libpcsclite-dev \
    ca-certificates \
 && rm -rf /var/lib/apt/lists/*
# Optional bladeRF (uncomment if you need bladeRF RF support baked in):
# RUN apt-get update && apt-get install -y software-properties-common \
#  && add-apt-repository -y ppa:nuandllc/bladerf \
#  && apt-get update && apt-get install -y bladerf libbladerf-dev \
#  && rm -rf /var/lib/apt/lists/*
WORKDIR /src
RUN git clone --depth 1 --branch ${SRSRAN_VERSION} https://github.com/srsran/srsRAN_4G.git srsRAN_4G
# Local NAS interoperability fixes and CPU-only regression tests.
# Fail closed if the selected upstream revision no longer matches the patch baseline.
COPY third_party/srsran/0001-protected-pdn-reject.patch \
     third_party/srsran/0002-ipv4v6-fallback.patch \
     third_party/srsran/0003-strict-apn-session-safety.patch \
     third_party/srsran/0004-restricted-apn-access.patch \
     third_party/srsran/nas_pdn_reject_test.cc \
     third_party/srsran/ipv4v6_fallback_test.cc \
     third_party/srsran/strict_apn_test.cc \
     third_party/srsran/spgw_session_safety_test.cc \
     third_party/srsran/ue_snapshot_test.cc \
     third_party/srsran/restricted_apn_access_test.cc \
     third_party/srsran/restricted_mme_lifecycle_test.cc \
     third_party/srsran/test-CMakeLists.txt /tmp/lte-srsran/
RUN test "$(git -C /src/srsRAN_4G rev-parse HEAD)" = "eea87b1d893ae58e0b08bc381730c502024ae71f" \
 && mkdir -p /src/srsRAN_4G/srsepc/test \
 && cp /tmp/lte-srsran/test-CMakeLists.txt /src/srsRAN_4G/srsepc/test/CMakeLists.txt \
 && cp /tmp/lte-srsran/*_test.cc /src/srsRAN_4G/srsepc/test/ \
 && git -C /src/srsRAN_4G apply --check /tmp/lte-srsran/0001-protected-pdn-reject.patch \
 && git -C /src/srsRAN_4G apply /tmp/lte-srsran/0001-protected-pdn-reject.patch \
 && git -C /src/srsRAN_4G apply --check /tmp/lte-srsran/0002-ipv4v6-fallback.patch \
 && git -C /src/srsRAN_4G apply /tmp/lte-srsran/0002-ipv4v6-fallback.patch \
 && git -C /src/srsRAN_4G apply --check /tmp/lte-srsran/0003-strict-apn-session-safety.patch \
 && git -C /src/srsRAN_4G apply /tmp/lte-srsran/0003-strict-apn-session-safety.patch \
 && git -C /src/srsRAN_4G apply --check /tmp/lte-srsran/0004-restricted-apn-access.patch \
 && git -C /src/srsRAN_4G apply /tmp/lte-srsran/0004-restricted-apn-access.patch
WORKDIR /src/srsRAN_4G/build
RUN cmake ../ -DCMAKE_BUILD_TYPE=Release \
 && make -j"${SRSRAN_BUILD_JOBS}" \
 && ctest --output-on-failure --no-tests=error -R '^(nas_pdn_reject_test|ipv4v6_fallback_test|strict_apn_test|spgw_session_safety_test|ue_snapshot_test|restricted_apn_access_test|restricted_mme_lifecycle_test)$' \
 && make install \
 && ldconfig
# Verify RF drivers got compiled in (fail early if UHD missing)
RUN grep -E "UHD|BLADERF|SOAPY|ZEROMQ" CMakeCache.txt || true
# Preserve the patched source, including local tests and the upstream license.
RUN mkdir -p /out \
 && tar --exclude='srsRAN_4G/.git' --exclude='srsRAN_4G/build' \
        -czf /out/srsran-source.tar.gz -C /src srsRAN_4G

# ---------- stage 2: build the Go Web UI + API server ----------
FROM golang:1.22-bookworm AS go-builder
ARG OCI_VERSION
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY VERSION version.go ./
COPY cmd/ ./cmd/
# internal/webui/static is compiled into the Go binary by go:embed. COPY
# internal/ already includes the complete frontend; no Node build is required.
COPY internal/ ./internal/
# Keep deployment contracts available to CPU-only go-builder regression tests.
COPY deploy/docker/*.yml deploy/docker/Dockerfile* ./deploy/docker/
COPY deploy/docker/*.sh ./deploy/docker/
COPY third_party/srsran/ ./third_party/srsran/
COPY firmware/uhd/ ./firmware/uhd/
COPY LICENSE THIRD_PARTY_NOTICES.md ./
RUN embedded_version="$(tr -d '[:space:]' < VERSION)" \
 && test -n "${OCI_VERSION}" \
 && test "${OCI_VERSION}" = "${embedded_version}" \
 && CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /out/lte-system ./cmd/server \
 && tar -czf /out/build-materials.tar.gz VERSION go.mod go.sum LICENSE THIRD_PARTY_NOTICES.md deploy/docker third_party/srsran firmware/uhd

# ---------- stage 3: runtime ----------
FROM ubuntu:22.04
ARG OCI_VERSION
ARG OCI_REVISION
ARG OCI_SOURCE
LABEL org.opencontainers.image.version="${OCI_VERSION}" \
      org.opencontainers.image.revision="${OCI_REVISION}" \
      org.opencontainers.image.source="${OCI_SOURCE}"
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    libboost-program-options1.74.0 libconfig++9v5 libfftw3-single3 \
    libmbedtls14 libsctp1 libuhd4.1.0 uhd-host \
    libzmq5 iproute2 iptables iputils-ping dnsutils curl \
    tcpdump tshark hashcat intel-opencl-icd pcscd pcsc-tools libccid \
    # intel-opencl-icd: CPU OpenCL backend — without it hashcat exits
    # ("No OpenCL installation found") and the whole crack flow is dead.
    python3 python3-pip python3-pyscard python3-serial python3-yaml \
    python3-cupshelpers \
    usbutils kmod net-tools \
    ca-certificates git \
 && rm -rf /var/lib/apt/lists/*
# era-pinned pysim python deps (pySim-prog/read import chain).
# No --break-system-packages: Ubuntu 22.04 pip predates PEP 668.
RUN pip install --no-cache-dir pytlv cmd2 jsonpath-ng construct bidict \
    gsm0338 termcolor colorlog pycryptodomex packaging

# srsRAN binaries + configs from builder
COPY --from=srs-builder /usr/local/bin/srsenb /usr/local/bin/srsepc /usr/local/bin/
COPY --from=srs-builder /usr/local/lib/ /usr/local/lib/
COPY --from=srs-builder /usr/local/share/srsran/ /usr/local/share/srsran/
COPY --from=srs-builder /out/srsran-source.tar.gz /usr/share/lte-system/sources/srsran-source.tar.gz
COPY --from=go-builder /out/build-materials.tar.gz /usr/share/lte-system/sources/build-materials.tar.gz
COPY LICENSE THIRD_PARTY_NOTICES.md /usr/share/lte-system/
RUN ldconfig

# UHD FPGA images (stock) + compat firmware overlay.
# Sources live in per-board dirs under firmware/uhd/ (see its README);
# dest names are kept stable for select-uhd-fpga.sh.
RUN /usr/lib/uhd/utils/uhd_images_downloader.py || true
COPY firmware/uhd/blacksdr-B210mini-clone/usrp_b210_fpga.bin /usr/share/uhd/images/usrp_b210_fpga.blacksdr_compat.bin
COPY firmware/uhd/ettus-B210-stock/usrp_b210_fpga.bin /usr/share/uhd/images/usrp_b210_fpga.stock.bin
COPY deploy/docker/select-uhd-fpga.sh /usr/local/bin/select-uhd-fpga
RUN chmod +x /usr/local/bin/select-uhd-fpga

# pysim pinned to the era our custom cards.py was proven against
# (2023-07-09, pre-legacy-split unified pySim/ layout; prog supports
# -t/-A/--acc/-o/-k and prints "Programming successful").
# Overlay: custom testsim card logic (see third_party/pysim/).
ARG PYSIM_COMMIT=263fb0871c0c8b6b3cb58eaa1ca1779ce1adf6c4
RUN git clone https://github.com/osmocom/pysim.git /opt/pysim \
 && git -C /opt/pysim checkout ${PYSIM_COMMIT}
COPY third_party/pysim/cards.py /opt/pysim/pySim/cards.py
# Fail the build early if the overlay target moved upstream.
RUN python3 -c "import ast; t=open('/opt/pysim/pySim/cards.py').read(); assert '_cards_classes' in t and 'testsim' in t" \
 && python3 /opt/pysim/pySim-prog.py --help >/dev/null

# Go server + bundled configs.
# app.yaml is materialized from the example so LTE_CONFIG always exists
# (server also runs on pure defaults if the file is absent).
COPY --from=go-builder /out/lte-system /usr/local/bin/lte-system
COPY configs/*.example configs/sib.conf configs/rb.conf configs/rr.conf configs/sim_profiles.yaml /app/configs/
RUN cp /app/configs/app.yaml.example /app/configs/app.yaml
COPY deploy/docker/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh \
 && mkdir -p /data/conf /data/log

ENV LTE_CONFIG=/app/configs/app.yaml \
    LTE_DATA_DIR=/data \
    LTE_PYSIM_DIR=/opt/pysim \
    UHD_IMAGES_DIR=/usr/share/uhd/images

# Default surface: Web UI plus its limited same-origin management gateway.
# Direct API port 8081 is optional for explicit legacy/test use. EXPOSE is
# image metadata, not a firewall and not a host publishing rule.
EXPOSE 8080
ENTRYPOINT ["/entrypoint.sh"]
