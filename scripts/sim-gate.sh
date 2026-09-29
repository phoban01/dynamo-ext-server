#!/usr/bin/env bash
# Runs the simulator gate in SIM_SHARDS shards.
set -euo pipefail

if [ ! -d sim ]; then
  echo "sim-gate: not built yet (milestone M6)"
  exit 0
fi

echo "sim-gate: simulator exists but the gate is not written yet"
exit 1
