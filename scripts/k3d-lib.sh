#!/usr/bin/env bash
# Shared functions for k3d clusters, ADR 0009. Source this file.

K3S_IMAGE=${K3S_IMAGE:-rancher/k3s:v1.37.0-k3s1}

# e2e_lock waits for the lock that heavy runs share, see CLAUDE.md.
e2e_lock() {
  local lock="${TMPDIR:-/tmp}/solas-e2e.lock"
  exec 9>"$lock"
  if ! flock -n 9; then
    echo "another run holds $lock, waiting"
    flock 9
  fi
}

# cluster_create <name> <kubeconfig> [docker-network] creates a k3d cluster
# with only what solas needs and writes its kubeconfig. Disk eviction
# starts only below 1% free, as kind does, so a full host disk does not
# evict the pods.
cluster_create() {
  local name=$1 kubeconfig=$2 network=${3:-}
  case $name in
  e2e-*) ;;
  *)
    echo "cluster name $name must start with e2e-" >&2
    return 1
    ;;
  esac
  k3d cluster create "$name" --image "$K3S_IMAGE" --wait --timeout 180s \
    --kubeconfig-update-default=false \
    ${network:+--network "$network"} \
    --k3s-arg "--disable=traefik@server:0" \
    --k3s-arg "--disable=servicelb@server:0" \
    --k3s-arg "--disable=metrics-server@server:0" \
    --k3s-arg "--kubelet-arg=eviction-hard=imagefs.available<1%,nodefs.available<1%@server:0" \
    --k3s-arg "--kubelet-arg=eviction-minimum-reclaim=imagefs.available=1%,nodefs.available=1%@server:0" \
    >/dev/null 2>&1
  k3d kubeconfig get "$name" >"$kubeconfig"
}

# cluster_delete <name> deletes a cluster whose name starts with e2e-.
cluster_delete() {
  case $1 in
  e2e-*) k3d cluster delete "$1" >/dev/null 2>&1 || true ;;
  *) echo "refusing to delete $1: name does not start with e2e-" >&2 ;;
  esac
}

# image_import <image> <cluster> loads a local image into the cluster.
image_import() {
  k3d image import "$1" --cluster "$2" >/dev/null 2>&1
}
