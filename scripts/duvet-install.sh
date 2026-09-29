#!/usr/bin/env bash
# Installs Duvet through cargo into .devbox/cargo, unless it is on the path.
set -euo pipefail

if command -v duvet >/dev/null && [ "${DUVET_FORCE_INSTALL:-}" != 1 ]; then
  echo "duvet-install: $(command -v duvet) is already on the path"
  exit 0
fi

rustup toolchain install stable --profile minimal
cargo +stable install duvet --locked --root "$DEVBOX_PROJECT_ROOT/.devbox/cargo"
