#!/usr/bin/env bash
# Smoke test: one k3d cluster serves kubectl get devices from dynamodb-local.
# It takes the e2e lock, because heavy runs must not overlap.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$root"
cluster=e2e-smoke
# shellcheck source=scripts/k3d-lib.sh
source "$root/scripts/k3d-lib.sh"

e2e_lock

work=$(mktemp -d)
export KUBECONFIG="$work/kubeconfig"
cleanup() {
  status=$?
  if [ "$status" -ne 0 ]; then
    echo "smoke: FAILED; pods and events:"
    kubectl -n solas-system get pods -o wide 2>/dev/null || true
    kubectl -n solas-system get events --sort-by=.lastTimestamp 2>/dev/null | tail -15 || true
    echo "smoke: logs of the API server:"
    kubectl -n solas-system logs deploy/solas-apiserver --tail=50 2>/dev/null || true
  fi
  cluster_delete "$cluster"
  rm -rf "$work"
  exit "$status"
}
trap cleanup EXIT

step() { echo "smoke: $*"; }
# expect_fail <what> <command...> passes when the command fails.
expect_fail() {
  local what=$1
  shift
  if "$@" >"$work/out" 2>&1; then
    echo "smoke: expected failure: $what"
    exit 1
  fi
  step "rejected as expected: $what ($(tail -1 "$work/out"))"
}

step "create k3d cluster $cluster"
cluster_create "$cluster" "$KUBECONFIG"

step "build and load the image"
docker build -q -t solas-apiserver:dev . >/dev/null
image_import solas-apiserver:dev "$cluster"

step "deploy dynamodb-local and the API server"
kubectl apply -f deploy/apiserver/namespace.yaml >/dev/null
kubectl apply -f deploy/dynamodb-local/ >/dev/null
kubectl apply -f deploy/apiserver/ >/dev/null
hack/gen-certs.sh >/dev/null
kubectl -n solas-system rollout status deploy/dynamodb-local --timeout=180s >/dev/null
kubectl -n solas-system rollout status deploy/solas-apiserver --timeout=180s >/dev/null
kubectl wait --for=condition=Available apiservice/v1alpha1.solas.dev --timeout=120s >/dev/null

step "create and list a device"
kubectl apply -f - >/dev/null <<YAML
apiVersion: solas.dev/v1alpha1
kind: Device
metadata:
  name: d1
spec:
  description: smoke test device
YAML
kubectl get devices
kubectl get device d1 -o jsonpath='{.metadata.resourceVersion}{"\n"}' | grep -qx '[0-9]\+'

ref() {
  printf '{"status":{"claimRef":{"member":"a","memberUID":"m1","namespace":"ns","name":"%s","uid":"u-%s"}}}' "$1" "$1"
}

step "bind d1 to claim c1"
kubectl patch device d1 --subresource=status --type=merge -p "$(ref c1)" >/dev/null
test "$(kubectl get device d1 -o jsonpath='{.status.claimRef.name}')" = c1

expect_fail "move d1 to claim c2" \
  kubectl patch device d1 --subresource=status --type=merge -p "$(ref c2)"
expect_fail "delete bound device d1" kubectl delete device d1

test "$(kubectl get device d1 -o jsonpath='{.status.fencingToken}')" = 1

step "release d1, bind it again, and check the token goes up"
kubectl patch device d1 --subresource=status --type=merge -p '{"status":{"claimRef":null}}' >/dev/null
kubectl patch device d1 --subresource=status --type=merge -p "$(ref c3)" >/dev/null
test "$(kubectl get device d1 -o jsonpath='{.status.fencingToken}')" = 2

step "release d1, then delete it"
kubectl patch device d1 --subresource=status --type=merge -p '{"status":{"claimRef":null}}' >/dev/null
kubectl delete device d1 >/dev/null

step "create a member"
kubectl apply -f - >/dev/null <<YAML
apiVersion: solas.dev/v1alpha1
kind: Member
metadata:
  name: cluster-a
YAML
test "$(kubectl get member cluster-a -o jsonpath='{.spec.leaseDurationSeconds}/{.status.phase}')" = "30/Active"

step "PASS"
