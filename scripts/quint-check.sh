#!/usr/bin/env bash
# Checks the Quint models. Positive models must pass their invariants.
# Negative models in quint/negative/ must fail them.
set -euo pipefail

if [ ! -d quint ]; then
  echo "quint-check: no models yet (milestone M2)"
  exit 0
fi

echo "quint-check: models exist but checks are not written yet"
exit 1
