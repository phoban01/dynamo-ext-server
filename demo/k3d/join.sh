#!/usr/bin/env bash
# Brings up cluster e2e-<name> on the mesh and joins it to solas. The
# cluster shares nothing with the others except the table.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root"
# shellcheck source=scripts/k3d-lib.sh
source scripts/k3d-lib.sh

name=${1:?usage: join.sh <name>}
cluster=e2e-$name
kube=demo/k3d/.kube/$name
mkdir -p demo/k3d/.kube
# SOLAS_STORE picks the shared store: dynamodb, the default, or etcd.
store=${SOLAS_STORE:-dynamodb}
url=$(demo/k3d/mesh.sh url "$store")
say() { echo "[$name] $*"; }

if ! k3d cluster get "$cluster" >/dev/null 2>&1; then
  say "create cluster $cluster"
  cluster_create "$cluster" "$kube" solas-mesh
fi
k3d kubeconfig get "$cluster" >"$kube"
export KUBECONFIG="$root/$kube"

say "load images"
image_import "$cluster" solas:dev solas-demo:dev

say "install solas as member $name, store $url"
# A short lease, so the demo scenes take seconds.
solas_install "$name" "$url" 10s 1s 3s
for _ in $(seq 60); do
  kubectl get member "$name" >/dev/null 2>&1 && break
  sleep 1
done
say "joined as member $name, uid $(kubectl get member "$name" -o jsonpath='{.metadata.uid}')"
