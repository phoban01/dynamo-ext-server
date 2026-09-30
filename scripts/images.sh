#!/usr/bin/env bash
# Builds the solas-apiserver and solas-controller images.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
docker build -q --target apiserver -t solas-apiserver:dev . >/dev/null
docker build -q --target controller -t solas-controller:dev . >/dev/null
docker build -q --target demo -t solas-demo:dev . >/dev/null
docker build -q --target pivot -t solas-pivot:dev . >/dev/null
echo "built solas-apiserver:dev, solas-controller:dev, solas-demo:dev, and solas-pivot:dev"
