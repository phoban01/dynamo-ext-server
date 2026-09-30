#!/usr/bin/env bash
# Runs the end-to-end tests against two k3d clusters. A lock makes a second
# run wait for the first, because the clusters use a lot of memory.
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
# shellcheck source=scripts/k3d-lib.sh
source scripts/k3d-lib.sh
e2e_lock
go test -tags e2e -count=1 -timeout 30m -v ./test/e2e/...
