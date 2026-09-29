#!/usr/bin/env bash
# Generates deepcopy, conversion, defaulting, OpenAPI, and client code for
# the solas.dev API group with k8s.io/code-generator.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"

codegen_version=v0.37.1
codegen_dir=$(go mod download -json "k8s.io/code-generator@${codegen_version}" |
  sed -n 's/^[[:space:]]*"Dir": "\(.*\)",$/\1/p')

# Keep the generator binaries in the project, not in the global GOBIN.
export GOBIN="$root/.devbox/codegen-bin"
mkdir -p "$GOBIN"

# shellcheck source=/dev/null
source "$codegen_dir/kube_codegen.sh"

solas_pkg=github.com/phoban01/solas
boilerplate="$root/hack/boilerplate.go.txt"

kube::codegen::gen_helpers \
  --boilerplate "$boilerplate" \
  "$root/pkg/apis"

kube::codegen::gen_openapi \
  --output-dir "$root/pkg/generated/openapi" \
  --output-pkg "$solas_pkg/pkg/generated/openapi" \
  --report-filename "$root/hack/api-rules/violation_exceptions.list" \
  ${UPDATE_API_RULES:+--update-report} \
  --output-model-name-file zz_generated.model_name.go \
  --boilerplate "$boilerplate" \
  "$root/pkg/apis"

kube::codegen::gen_client \
  --with-watch \
  --output-dir "$root/pkg/generated" \
  --output-pkg "$solas_pkg/pkg/generated" \
  --boilerplate "$boilerplate" \
  "$root/pkg/apis"
