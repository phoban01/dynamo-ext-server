#!/usr/bin/env bash
# Fails when the generated code is out of date.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

"$root/hack/update-codegen.sh" >/dev/null
if ! git diff --quiet -- pkg/ || [ -n "$(git ls-files --others --exclude-standard pkg/)" ]; then
  echo "verify-codegen: generated code is out of date; run devbox run codegen"
  git status --short -- pkg/
  exit 1
fi
echo "verify-codegen: generated code is up to date"
