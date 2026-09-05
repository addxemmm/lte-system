#!/bin/bash
# select-uhd-fpga: switch /usr/share/uhd/images/usrp_b210_fpga.bin
# between stock Ettus image and the BlackSDR/compatible-board image.
# Usage: select-uhd-fpga [stock|compat|auto]
set -e
IMG_DIR="${UHD_IMAGES_DIR:-/usr/share/uhd/images}"
MODE="${1:-auto}"
case "$MODE" in
  stock)
    if [ -f "$IMG_DIR/usrp_b210_fpga.stock.bin" ]; then
      cp -f "$IMG_DIR/usrp_b210_fpga.stock.bin" "$IMG_DIR/usrp_b210_fpga.bin"
      echo "UHD FPGA -> stock"
    else
      echo "stock image not found, keeping current" >&2
    fi
    ;;
  compat|blacksdr)
    if [ -f "$IMG_DIR/usrp_b210_fpga.blacksdr_compat.bin" ]; then
      cp -f "$IMG_DIR/usrp_b210_fpga.blacksdr_compat.bin" "$IMG_DIR/usrp_b210_fpga.bin"
      echo "UHD FPGA -> BlackSDR compat"
    else
      echo "compat image not found, keeping current" >&2
    fi
    ;;
  auto|*)
    echo "UHD FPGA -> auto (keeping $(ls -la "$IMG_DIR/usrp_b210_fpga.bin" 2>/dev/null || echo MISSING))"
    ;;
esac
# Print checksums for ops log.
md5sum "$IMG_DIR"/usrp_b210_fpga*.bin 2>/dev/null || true
