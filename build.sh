#!/usr/bin/env bash
# Builds ShopKeeper for Windows (a window, no console) and Linux (opens the browser) into dist/
set -euo pipefail
cd "$(dirname "$0")"

mkdir -p dist

echo "Building dist/ShopKeeper.exe (windows/amd64)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-s -w -H=windowsgui" -o dist/ShopKeeper.exe ./cmd/server

echo "Building dist/shopkeeper-linux (linux/amd64)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o dist/shopkeeper-linux ./cmd/server

echo "Done. Binaries in dist/"
