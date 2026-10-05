#!/usr/bin/env bash
# Builds double-clickable ShopKeeper binaries for Windows and Linux into dist/.
set -euo pipefail
cd "$(dirname "$0")"

mkdir -p dist

echo "Building dist/ShopKeeper.exe (windows/amd64)..."
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -ldflags "-s -w" -o dist/ShopKeeper.exe ./cmd/server

echo "Building dist/shopkeeper-linux (linux/amd64)..."
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags "-s -w" -o dist/shopkeeper-linux ./cmd/server

echo "Done. Binaries in dist/"
