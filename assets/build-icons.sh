#!/usr/bin/env bash
# Regenerates the icon files from assets/icon.svg. Needs rsvg-convert and python3 with Pillow.
#
#   assets/icon.ico                      installer, shortcuts, and the window/taskbar icon
#   cmd/server/icon_windows.ico          copy embedded in the exe (go:embed)
#   web/static/icon.svg, favicon.ico     browser tab
#   assets/icon-48.png, icon-256.png     the Linux package's menu icon (packaging/linux)
#
#   cmd/server/rsrc_windows_amd64.syso   the exe's own icon in Explorer (go build links it)
#
# The .syso is made by tools/mksyso (standard library only) from assets/icon.ico.
set -euo pipefail
cd "$(dirname "$0")/.."

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT

for s in 16 24 32 48 64 128 256; do
	rsvg-convert -w "$s" -h "$s" assets/icon.svg -o "$tmp/$s.png"
done

python3 - "$tmp" <<'PY'
import sys
from PIL import Image
tmp = sys.argv[1]
sizes = [16, 24, 32, 48, 64, 128, 256]
big = Image.open(f"{tmp}/256.png").convert("RGBA")
big.save("assets/icon.ico", format="ICO", sizes=[(s, s) for s in sizes])
big.save("web/static/favicon.ico", format="ICO", sizes=[(16, 16), (32, 32), (48, 48)])
PY

cp "$tmp/48.png" assets/icon-48.png
cp "$tmp/256.png" assets/icon-256.png
cp assets/icon.svg web/static/icon.svg
cp assets/icon.ico cmd/server/icon_windows.ico

go run ./tools/mksyso -ico assets/icon.ico -o cmd/server/rsrc_windows_amd64.syso
echo "icons written"
