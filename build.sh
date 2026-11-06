#!/usr/bin/env bash
# Builds ShopKeeper for Windows (a window, no console) and Linux (opens the browser) into dist/
set -euo pipefail
cd "$(dirname "$0")"

mkdir -p dist

# The version shown in the app and compared by its "Update" button: VERSION=1.2.0 ./build.sh
VERSION="${VERSION:-dev}"
VERSION_FLAG="-X shop/internal/update.Version=${VERSION#v}"

echo "Building dist/ShopKeeper.exe (windows/amd64)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -H=windowsgui ${VERSION_FLAG}" -o dist/ShopKeeper.exe ./cmd/server

echo "Building dist/shopkeeper-linux (linux/amd64)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w ${VERSION_FLAG}" -o dist/shopkeeper-linux ./cmd/server

echo "Packing dist/ShopKeeper-linux.tar.gz..."
packaging/linux/make-tarball.sh dist/shopkeeper-linux dist/ShopKeeper-linux.tar.gz

echo "Done. Files in dist/"
