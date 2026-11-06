#!/usr/bin/env bash
# Removes the ShopKeeper program, menu entry and icons installed by install.sh.
# Your shop data (shop.db, backups/, shop.log) is KEPT, so installing again picks
# up where you left off. Add --purge to delete the data too.
set -euo pipefail

data_home="${XDG_DATA_HOME:-$HOME/.local/share}"
app_dir="${SHOPKEEPER_DIR:-$data_home/shopkeeper}"
bin_dir="${HOME}/.local/bin"

purge=0
case "${1:-}" in
  "") ;;
  --purge) purge=1 ;;
  *) echo "usage: $0 [--purge]" >&2; exit 2 ;;
esac

if pgrep -x shopkeeper >/dev/null 2>&1; then
  echo "ShopKeeper is running. Quit it first (the Quit button at the top of its page)." >&2
  exit 1
fi

rm -f "$app_dir/shopkeeper" "$app_dir/shopkeeper.new" "$app_dir/shopkeeper.old"
# Only remove the command if it still points at our program.
if [ -L "$bin_dir/shopkeeper" ] && [ "$(readlink "$bin_dir/shopkeeper")" = "$app_dir/shopkeeper" ]; then
  rm -f "$bin_dir/shopkeeper"
fi
rm -f "$data_home/applications/shopkeeper.desktop" \
  "$data_home/icons/hicolor/scalable/apps/shopkeeper.svg" \
  "$data_home/icons/hicolor/256x256/apps/shopkeeper.png" \
  "$data_home/icons/hicolor/48x48/apps/shopkeeper.png"

if command -v update-desktop-database >/dev/null 2>&1; then
  update-desktop-database "$data_home/applications" >/dev/null 2>&1 || true
fi

if [ "$purge" = 1 ]; then
  # Refuse to delete a folder that does not look like ours.
  if [ -d "$app_dir" ] && { [ -f "$app_dir/shop.db" ] || [ -d "$app_dir/backups" ]; }; then
    rm -rf -- "$app_dir"
    echo "ShopKeeper and all its data were removed."
  else
    rmdir "$app_dir" 2>/dev/null || true
    echo "ShopKeeper was removed. No data folder found to delete."
  fi
else
  echo "ShopKeeper was removed. Your data is still in $app_dir (run with --purge to delete it)."
fi
