#!/usr/bin/env bash
# Installs ShopKeeper for the current user (no root needed):
#   - the program and its data live in  ~/.local/share/shopkeeper/
#     (shop.db, backups/ and shop.log are created there, next to the program)
#   - an application-menu entry "ShopKeeper" with its icon
#   - a "shopkeeper" command in ~/.local/bin
#
# Run it from the unpacked folder:   ./install.sh
# Install somewhere else:            SHOPKEEPER_DIR=/path ./install.sh
# Running it again updates the program and leaves your data alone.
set -euo pipefail

here="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
app_dir="${SHOPKEEPER_DIR:-$data_home/shopkeeper}"
bin_dir="${HOME}/.local/bin"

if [ ! -f "$here/shopkeeper" ]; then
  echo "shopkeeper binary not found next to install.sh" >&2
  exit 1
fi

# The path ends up inside the launcher's Exec= line, where these characters
# would need escaping rules not worth getting wrong.
case "$app_dir" in
  *[\"\\\`\$%]*) echo "The install folder may not contain \" \\ \` \$ or %: $app_dir" >&2; exit 1 ;;
esac

mkdir -p "$app_dir" "$bin_dir" \
  "$data_home/applications" \
  "$data_home/icons/hicolor/scalable/apps" \
  "$data_home/icons/hicolor/256x256/apps" \
  "$data_home/icons/hicolor/48x48/apps"

# Copy under a temporary name and rename, so a ShopKeeper that is running now
# keeps its old file until it restarts (replacing a running binary in place
# fails with "text file busy").
install -m 0755 "$here/shopkeeper" "$app_dir/shopkeeper.new"
mv -f "$app_dir/shopkeeper.new" "$app_dir/shopkeeper"

install -m 0644 "$here/icons/icon.svg"     "$data_home/icons/hicolor/scalable/apps/shopkeeper.svg"
install -m 0644 "$here/icons/icon-256.png" "$data_home/icons/hicolor/256x256/apps/shopkeeper.png"
install -m 0644 "$here/icons/icon-48.png"  "$data_home/icons/hicolor/48x48/apps/shopkeeper.png"

desktop="$data_home/applications/shopkeeper.desktop"
sed "s|@EXEC@|\"$app_dir/shopkeeper\"|" "$here/shopkeeper.desktop.in" > "$desktop"
chmod 0644 "$desktop"

ln -sf "$app_dir/shopkeeper" "$bin_dir/shopkeeper"

# Menus and icon themes pick changes up by themselves; these just make it immediate.
if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database "$data_home/applications" >/dev/null 2>&1 || true
fi
if command -v gtk-update-icon-cache >/dev/null 2>&1; then
  gtk-update-icon-cache -q -t -f "$data_home/icons/hicolor" >/dev/null 2>&1 || true
fi

cat <<MSG
ShopKeeper is installed.

  Start it:   open "ShopKeeper" in your application menu, or run: shopkeeper
  Program:    $app_dir/shopkeeper
  Your data:  $app_dir/shop.db  (backups in $app_dir/backups)
  Remove it:  ./uninstall.sh   (your data is kept unless you add --purge)

It opens in your web browser. Closing the browser tab does not stop it: use the
"Quit" button at the top of the page. Starting it again just opens the page.
MSG
case ":$PATH:" in
  *":$bin_dir:"*) ;;
  *) echo; echo "Note: $bin_dir is not in your PATH, so the 'shopkeeper' command needs the full path. The menu entry works regardless." ;;
esac
