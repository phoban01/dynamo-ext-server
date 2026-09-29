#!/usr/bin/env bash
# Runs the unit tests with the race detector.
set -euo pipefail

if [ -z "$(go list ./... 2>/dev/null)" ]; then
  echo "test: no Go packages yet"
  exit 0
fi

go test -race ./...
