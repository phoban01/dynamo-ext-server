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
endpoint=$(demo/k3d/mesh.sh endpoint)
say() { echo "[$name] $*"; }

if ! k3d cluster get "$cluster" >/dev/null 2>&1; then
  say "create cluster $cluster"
  cluster_create "$cluster" "$kube" solas-mesh
fi
k3d kubeconfig get "$cluster" >"$kube"
export KUBECONFIG="$root/$kube"

say "load images"
for image in solas-apiserver:dev solas-controller:dev solas-demo:dev; do
  image_import "$image" "$cluster"
done

say "deploy solas-apiserver, table $endpoint"
kubectl apply -f deploy/apiserver/namespace.yaml >/dev/null
kubectl -n solas-system create secret generic solas-dynamodb --dry-run=client -o yaml \
  --from-literal=endpoint="$endpoint" \
  --from-literal=accessKeyID=local --from-literal=secretAccessKey=local | kubectl apply -f - >/dev/null
kubectl apply -f deploy/apiserver/ >/dev/null
hack/gen-certs.sh >/dev/null
kubectl -n solas-system rollout status deploy/solas-apiserver --timeout=180s >/dev/null
kubectl wait --for=condition=Available apiservice/v1alpha1.solas.dev --timeout=120s >/dev/null

say "deploy solas-controller as member $name"
kubectl -n solas-system create configmap solas-member --dry-run=client -o yaml \
  --from-literal=clusterID="$name" --from-literal=leaseDuration=10s \
  --from-literal=leaseMargin=1s --from-literal=sweepInterval=3s | kubectl apply -f - >/dev/null
kubectl apply -f deploy/controller/crd.yaml >/dev/null
kubectl wait --for=condition=Established crd/deviceclaims.claims.solas.dev --timeout=60s >/dev/null
kubectl apply -f deploy/controller/rbac.yaml -f deploy/controller/deployment.yaml >/dev/null
kubectl -n solas-system rollout status deploy/solas-controller --timeout=120s >/dev/null
for _ in $(seq 60); do
  kubectl get member "$name" >/dev/null 2>&1 && break
  sleep 1
done
say "joined as member $name, uid $(kubectl get member "$name" -o jsonpath='{.metadata.uid}')"
