#!/usr/bin/env bash
# Runs the end-to-end tests. A lock makes a second run wait for the first,
# because two runs at once can exhaust the memory of the development VM.
set -euo pipefail

lock="${TMPDIR:-/tmp}/solas-e2e.lock"
exec 9>"$lock"
if ! flock -n 9; then
  echo "e2e: another run holds $lock, waiting"
  flock 9
fi

if [ ! -d test/e2e ]; then
  echo "e2e: not built yet (milestone M7)"
  exit 0
fi

go test -count=1 -timeout 30m ./test/e2e/...
