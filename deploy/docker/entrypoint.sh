#!/bin/bash
# Entrypoint: pick FPGA (stock vs BlackSDR-compat), seed /data, start Go API.
set -e

# 1. FPGA selection for clone B210 boards.
#    UHD_FPGA=stock|compat|auto  (default auto = keep current images untouched)
if command -v select-uhd-fpga >/dev/null 2>&1; then
  select-uhd-fpga "${UHD_FPGA:-auto}" || true
fi

# 2. Seed /data on first run (idempotent for user data).
# Static radio configs always follow the image so redeploys propagate fixes;
# user_db.csv / wordlist.list are only copied when missing (never overwritten).
mkdir -p /data/conf /data/log
for f in sib.conf rr.conf rb.conf; do
  cp -f "/app/configs/$f" "/data/conf/$f"
done
if [ ! -f /data/wordlist.list ] && [ -f /app/configs/wordlist.list.example ]; then
  cp /app/configs/wordlist.list.example /data/wordlist.list
fi

# 3. pcscd for ACR1281U (best-effort; needs --privileged + /dev/bus/usb).
service pcscd start || true

# 4. Show radio state (never fails the boot).
uhd_find_devices 2>&1 | head -n 20 || true
bladeRF-cli -e info 2>&1 | head -n 20 || true

exec /usr/local/bin/lte-system
