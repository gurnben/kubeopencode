#!/bin/sh
# Crush Init Container Entrypoint
#
# Copies the crush binary to the shared tools volume at /tools/crush.
# The work container can then execute: /tools/crush run ...
#
# Environment Variables:
#   TOOLS_DIR   - Target directory (default: /tools)
#   CRUSH_BIN   - Binary name (default: crush)

set -e

TOOLS_DIR="${TOOLS_DIR:-/tools}"
CRUSH_BIN="${CRUSH_BIN:-crush}"
TARGET="${TOOLS_DIR}/${CRUSH_BIN}"

echo "[crush-init] Copying Crush binary to ${TARGET}..."
mkdir -p "${TOOLS_DIR}"
cp /crush "${TARGET}"
chmod +x "${TARGET}"

echo "[crush-init] Crush binary installed successfully."
echo "[crush-init] Work containers can use: ${TARGET}"

# Print version for verification
"${TARGET}" --version 2>/dev/null || echo "[crush-init] Version check skipped"
