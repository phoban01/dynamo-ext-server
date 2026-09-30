#!/usr/bin/env bash
# Builds the solas, solas-demo, and solas-pivot images.
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
docker build -q --target solas -t solas:dev . >/dev/null
docker build -q --target demo -t solas-demo:dev . >/dev/null
docker build -q --target pivot -t solas-pivot:dev . >/dev/null
echo "built solas:dev, solas-demo:dev, and solas-pivot:dev"
