#!/usr/bin/env bash
# Model-based tests: generate traces of quint/solas.qnt with a fixed seed,
# then replay each one against the real REST stores on dynamodb-local.
#   MBT_COUNT  traces to generate (default 40)
#   MBT_STEPS  steps per trace (default 30)
#   MBT_SEED   seed of the trace generator (default 0x5eed)
set -euo pipefail
root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

dir=$(mktemp -d)
trap 'rm -rf "$dir"' EXIT
count=${MBT_COUNT:-40}

quint run quint/solas.qnt --main solas2 --mbt \
  --max-steps "${MBT_STEPS:-30}" --max-samples "$count" --n-traces "$count" \
  --seed "${MBT_SEED:-0x5eed}" --out-itf "$dir/trace{seq}.itf.json" >/dev/null
echo "mbt: generated $(find "$dir" -name '*.itf.json' | wc -l) traces"

MBT_TRACES=$dir TEST_PKGS=./mbt/... scripts/test.sh -count=1 -run 'TestMBT' 
