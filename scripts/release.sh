#!/bin/sh
set -eu
version=${1:-dev}
cd "$(dirname "$0")/.."
mkdir -p dist
for os in darwin linux; do
  for arch in amd64 arm64; do
    CGO_ENABLED=0 GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=$version" -o "dist/lazyiperf_${os}_${arch}" ./cmd/lazyiperf
  done
done
cp scripts/install.sh dist/install.sh
cd dist
if command -v sha256sum >/dev/null 2>&1; then
  sha256sum lazyiperf_* install.sh > checksums.txt
else
  shasum -a 256 lazyiperf_* install.sh > checksums.txt
fi
