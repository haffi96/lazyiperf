#!/bin/sh
# Usage: curl -fsSL <release-host>/install.sh | LAZYIPERF_RELEASE_URL=<release-directory> sh
set -eu
case "$(uname -s)" in Darwin) os=darwin ;; Linux) os=linux ;; *) echo 'Supported systems: macOS and Linux' >&2; exit 1 ;; esac
case "$(uname -m)" in x86_64|amd64) arch=amd64 ;; arm64|aarch64) arch=arm64 ;; *) echo 'Supported architectures: amd64 and arm64' >&2; exit 1 ;; esac
repo=${LAZYIPERF_REPO:-haffi96/lazyiperf}
base=${LAZYIPERF_RELEASE_URL:-}
if [ -z "$base" ] && [ -n "$repo" ]; then
  base="https://github.com/$repo/releases/latest/download"
fi
if [ -z "$base" ]; then
  echo 'Set LAZYIPERF_REPO=owner/repo or LAZYIPERF_RELEASE_URL=https://host/release-directory' >&2
  exit 1
fi
case "$base" in https://*) ;; *) echo 'Release URL must use HTTPS' >&2; exit 1 ;; esac
bin_dir=${LAZYIPERF_BIN_DIR:-"$HOME/.local/bin"}
asset="lazyiperf_${os}_${arch}"
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM
curl --proto '=https' --tlsv1.2 -fsSL "$base/$asset" -o "$tmp/$asset"
curl --proto '=https' --tlsv1.2 -fsSL "$base/checksums.txt" -o "$tmp/checksums.txt"
expected=$(awk -v name="$asset" '$2 == name {print $1}' "$tmp/checksums.txt")
if [ "${#expected}" -ne 64 ]; then echo 'Missing or invalid checksum' >&2; exit 1; fi
case "$expected" in *[!a-fA-F0-9]*) echo 'Invalid checksum' >&2; exit 1 ;; esac
if command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "$tmp/$asset" | awk '{print $1}')
else
  actual=$(shasum -a 256 "$tmp/$asset" | awk '{print $1}')
fi
if [ "$actual" != "$expected" ]; then echo 'Checksum mismatch; installation aborted' >&2; exit 1; fi
mkdir -p "$bin_dir"
# Stage within the destination filesystem, then atomically replace the executable.
staged=$(mktemp "$bin_dir/.lazyiperf.XXXXXX")
trap 'rm -rf "$tmp"; rm -f "$staged"' EXIT HUP INT TERM
cp "$tmp/$asset" "$staged"
chmod 755 "$staged"
mv -f "$staged" "$bin_dir/lazyiperf"
printf 'Installed lazyiperf to %s/lazyiperf\n' "$bin_dir"
case ":$PATH:" in *":$bin_dir:"*) ;; *) printf 'Add %s to your PATH to run lazyiperf.\n' "$bin_dir" ;; esac
