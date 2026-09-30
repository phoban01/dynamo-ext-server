#!/usr/bin/env bash
# The demo. Two clusters, a and b, share one pool of devices through one
# DynamoDB table. The setup runs quietly; its log goes to
# demo/k3d/setup.log. On failure the clusters stay up; demo/k3d/down.sh
# removes them.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root"
# shellcheck source=scripts/k3d-lib.sh
source scripts/k3d-lib.sh
e2e_lock

manifests=demo/k3d/manifests
setup_log=demo/k3d/setup.log

k() { KUBECONFIG="$root/demo/k3d/.kube/$1" kubectl "${@:2}"; }
scene() { printf '\n=== %s\n' "$*"; }
say() { echo "  $*"; }
# show <cluster> <kubectl args...> prints a command and its output.
show() {
  printf '\n  [%s] $ kubectl %s\n' "$1" "${*:2}"
  k "$@" | sed 's/^/  /'
}
fail() {
  echo "FAILED: $*" >&2
  echo "The clusters are still up. Remove them with demo/k3d/down.sh." >&2
  exit 1
}
# wait_for <seconds> <description> <command...> retries until the command
# succeeds.
wait_for() {
  local secs=$1 what=$2
  shift 2
  for _ in $(seq "$secs"); do
    if "$@" >/dev/null 2>&1; then
      return 0
    fi
    sleep 1
  done
  fail "$what, after ${secs}s"
}
# phase <cluster> <claim> prints the phase of a claim in namespace work.
phase() { k "$1" -n work get deviceclaim "$2" -o jsonpath='{.status.phase}' 2>/dev/null; }
is_phase() { [ "$(phase "$1" "$2")" = "$3" ]; }
# device_of <cluster> <claim> prints the device of a claim.
device_of() { k "$1" -n work get deviceclaim "$2" -o jsonpath='{.status.deviceName}'; }
# holder <device> prints member/claim/token of the holder, as cluster a sees it.
holder() { k a get device "$1" -o jsonpath='{.status.claimRef.member}/{.status.claimRef.name}/{.status.fencingToken}'; }
# set_ready <device> <True|False> <reason> sets the Ready condition, as
# the party that runs the device would.
set_ready() {
  local now
  now=$(date -u +%Y-%m-%dT%H:%M:%SZ)
  k a patch device "$1" --subresource=status --type=merge -p "{\"status\":{\"conditions\":[
    {\"type\":\"Ready\",\"status\":\"$2\",\"reason\":\"$3\",\"message\":\"set by the demo\",\"lastTransitionTime\":\"$now\"}]}}" >/dev/null
}
device_log() { curl -s "$device_url/log"; }
uses_by() { device_log | grep -q "\"claim\":\"$1/work/job\",\"token\":$2,\"accepted\":$3"; }

scene "Setup: two k3d clusters and one table (log: $setup_log)"
: >"$setup_log"
quiet() { "$@" >>"$setup_log" 2>&1 || fail "$*; see $setup_log"; }
quiet scripts/images.sh
quiet demo/k3d/mesh.sh up
for name in a b; do
  quiet demo/k3d/join.sh "$name"
done
show a get members

scene "1. The device pool: created in a, seen in b"
k a apply -f "$manifests/devices.yaml" >/dev/null
for d in gpu-a100-1 gpu-a100-2 gpu-h100-1 gpu-t4-1 fpga-1 nic-1 nic-2; do
  set_ready "$d" True Healthy
done
set_ready gpu-t4-2 False Overheating
say "the gpu-t4-2 agent reports Ready=False"
wait_for 30 "gpu-t4-2 visible in b" k b get device gpu-t4-2
show b get devices -L kind,zone
show b get devices -o custom-columns=NAME:.metadata.name,VENDOR:.spec.attributes.vendor,MODEL:.spec.attributes.model,MEMORY_GIB:.spec.attributes.memoryGiB,SPEED:.spec.attributes.speed

scene "2. Claims select devices with labels and CEL"
k a apply -f "$manifests/claims-a.yaml" >/dev/null
for c in train infer net; do
  wait_for 60 "a/$c Bound" is_phase a "$c" Bound
done
k b apply -f "$manifests/claims-b.yaml" >/dev/null
for c in render encode; do
  wait_for 60 "b/$c Bound" is_phase b "$c" Bound
done
[ "$(device_of a infer)" = gpu-t4-1 ] || fail "a/infer holds $(device_of a infer), want gpu-t4-1: gpu-t4-2 is not Ready"
[ "$(device_of a net)" = nic-1 ] || fail "a/net holds $(device_of a net), want nic-1"
show a get deviceclaims -n work
show b get deviceclaims -n work
show a get devices
say "infer wants a Ready GPU with at most 16 GiB: gpu-t4-2 is not Ready, so it got gpu-t4-1."
say "net wants a 400G NIC, so nic-2 stays free. Nothing asked for gpu-h100-1."

scene "3. A claim of priority 5 in b preempts a claim of priority 1 in a"
k b apply -f "$manifests/urgent.yaml" >/dev/null
victim=$(device_of a train)
wait_for 30 "a/train Preempting" is_phase a train Preempting
show a get devices "$victim" -o wide
show a get deviceclaims -n work train
say "a/train keeps $victim for its grace period of 10s, then lets it go."
wait_for 60 "b/urgent Bound" is_phase b urgent Bound
[ "$(device_of b urgent)" = "$victim" ] || fail "b/urgent holds $(device_of b urgent), want $victim"
wait_for 30 "a/train Pending" is_phase a train Pending
show b get deviceclaims -n work urgent
show a get deviceclaims -n work train
say "a/train waits. It may not preempt b/render, which has a higher priority."

scene "4. Claims in a and b race for gpu-h100-1; one wins"
docker rm -f solas-device-gpu-h100-1 >/dev/null 2>&1 || true
docker run -d --name solas-device-gpu-h100-1 --network solas-mesh solas-demo:dev device >/dev/null
device_url="http://$(docker inspect -f '{{(index .NetworkSettings.Networks "solas-mesh").IPAddress}}' solas-device-gpu-h100-1):9000"
curl_ready() { curl -sf "$device_url/log"; }
# The gatekeeper stands in for the device. It checks fencing tokens.
wait_for 30 "the gpu-h100-1 gatekeeper starts" curl_ready
for name in a b; do
  sed -e "s|CLUSTER_ID|$name|" -e "s|DEVICE_URL|$device_url|" "$manifests/claim.yaml" | k "$name" apply -f - >/dev/null &
done
wait
one_bound() {
  local n=0
  for name in a b; do
    is_phase "$name" job Bound && n=$((n + 1))
  done
  [ "$n" = 1 ]
}
wait_for 60 "exactly one job claim Bound" one_bound
if is_phase a job Bound; then winner=a loser=b; else winner=b loser=a; fi
[ "$(holder gpu-h100-1)" = "$winner/job/1" ] || fail "gpu-h100-1 holder is $(holder gpu-h100-1), want $winner/job/1"
sleep 3
is_phase "$loser" job Pending || fail "$loser/job is $(phase "$loser" job), want Pending"
wait_for 30 "the workload in $winner uses gpu-h100-1" uses_by "$winner" 1 true
show a get devices gpu-h100-1
say "$winner won with token 1 and its workload uses the device. $loser/job waits."

scene "5. $winner freezes past its lease; $loser takes over; fencing stops $winner"
old_uid=$(k "$winner" get member "$winner" -o jsonpath='{.metadata.uid}')
docker pause "k3d-e2e-$winner-server-0" >/dev/null
say "paused cluster $winner; its lease is 10s"
wait_for 90 "$loser/job Bound" is_phase "$loser" job Bound
[ "$(k "$loser" get device gpu-h100-1 -o jsonpath='{.status.claimRef.member}/{.status.fencingToken}')" = "$loser/2" ] ||
  fail "gpu-h100-1 is not held by $loser with token 2"
wait_for 30 "the workload in $loser uses gpu-h100-1" uses_by "$loser" 2 true
say "$loser's sweeper expired $winner and freed its devices; $loser/job holds gpu-h100-1 with token 2"
docker unpause "k3d-e2e-$winner-server-0" >/dev/null
say "unpaused $winner; its workload still thinks it holds token 1"
wait_for 60 "the device rejects $winner's stale token" uses_by "$winner" 1 false
say "the device REJECTED $winner's use with token 1, because it saw token 2"

scene "6. $winner joins again with a new member UID; its old claims are Lost"
new_uid() {
  local uid
  uid=$(k "$winner" get member "$winner" -o jsonpath='{.metadata.uid}')
  [ -n "$uid" ] && [ "$uid" != "$old_uid" ]
}
wait_for 90 "$winner joins again" new_uid
wait_for 60 "$winner/job Lost" is_phase "$winner" job Lost
show "$winner" get deviceclaims -n work
show a get devices

scene "Device log of gpu-h100-1"
device_log | tr '}' '\n' | grep -o '"claim":"[^"]*","token":[0-9]*,"accepted":[a-z]*' | uniq -c | sed 's/^/  /'

scene "PASS"
echo "Remove the demo with: demo/k3d/down.sh"
