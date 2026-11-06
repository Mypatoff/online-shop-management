#!/usr/bin/env bash
# Packs the Linux binary with its installer, launcher and icons:
#   packaging/linux/make-tarball.sh dist/shopkeeper-linux dist/ShopKeeper-linux.tar.gz
# Must run on Linux (the scripts' executable bits have to survive the archive).
set -euo pipefail

bin="${1:?usage: make-tarball.sh <binary> <output.tar.gz>}"
out="${2:?usage: make-tarball.sh <binary> <output.tar.gz>}"
root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"

stage="$(mktemp -d)"
trap 'rm -rf "$stage"' EXIT
pkg="$stage/ShopKeeper-linux"
mkdir -p "$pkg/icons"

install -m 0755 "$bin" "$pkg/shopkeeper"
install -m 0755 "$root/packaging/linux/install.sh" "$pkg/install.sh"
install -m 0755 "$root/packaging/linux/uninstall.sh" "$pkg/uninstall.sh"
install -m 0644 "$root/packaging/linux/shopkeeper.desktop.in" "$pkg/shopkeeper.desktop.in"
install -m 0644 "$root/packaging/linux/README.txt" "$pkg/README.txt"
install -m 0644 "$root/assets/icon.svg" "$pkg/icons/icon.svg"
install -m 0644 "$root/assets/icon-256.png" "$pkg/icons/icon-256.png"
install -m 0644 "$root/assets/icon-48.png" "$pkg/icons/icon-48.png"

mkdir -p "$(dirname "$out")"
tar -C "$stage" --owner=0 --group=0 --numeric-owner -czf "$out" ShopKeeper-linux
echo "wrote $out"
