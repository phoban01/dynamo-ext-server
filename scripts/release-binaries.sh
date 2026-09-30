#!/usr/bin/env bash
# Builds solas and solas-pivot for linux/amd64 and linux/arm64 into <dir>,
# with a SHA256SUMS file.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
out=${1:?usage: release-binaries.sh <dir>}
mkdir -p "$out"
for arch in amd64 arm64; do
  for c in solas solas-pivot; do
    CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath -ldflags='-s -w' -o "$out/$c-linux-$arch" "./cmd/$c"
  done
done
(cd "$out" && sha256sum solas* >SHA256SUMS)
