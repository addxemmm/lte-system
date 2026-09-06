# syntax=docker/dockerfile:1
# lte-system (srsRAN_4G + Go tool API)
# Base: Ubuntu 22.04 (gcc-11 works with srsRAN_4G; 24.04 needs gcc-11 downgrade).
# Pin the srsRAN version here; image version lives in VERSION at repo root.
ARG SRSRAN_VERSION=release_23_11

# ---------- stage 1: build srsRAN_4G ----------
FROM ubuntu:22.04 AS srs-builder
ARG SRSRAN_VERSION
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
WORKDIR /src/srsRAN_4G/build
RUN cmake ../ -DCMAKE_BUILD_TYPE=Release \
 && make -j$(nproc) \
 && make install \
 && ldconfig
# Verify RF drivers got compiled in (fail early if UHD missing)
RUN grep -E "UHD|BLADERF|SOAPY|ZEROMQ" CMakeCache.txt || true

# ---------- stage 2: build Go server ----------
FROM golang:1.22-bookworm AS go-builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o /out/lte-system ./cmd/server

# ---------- stage 3: runtime ----------
FROM ubuntu:22.04
ENV DEBIAN_FRONTEND=noninteractive
RUN apt-get update && apt-get install -y --no-install-recommends \
    libboost-program-options1.74.0 libconfig++9v5 libfftw3-single3 \
    libmbedtls14 libsctp1 libuhd4.1.0 uhd-host \
    libzmq5 iproute2 iptables iputils-ping dnsutils curl \
    tcpdump tshark hashcat pcscd pcsc-tools libccid \
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

EXPOSE 8081
ENTRYPOINT ["/entrypoint.sh"]
