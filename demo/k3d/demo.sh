#!/usr/bin/env bash
# The demo. It brings up the mesh and two member clusters, a and b,
# then shows device sharing, a race, fencing, and a rejoin. On failure it
# leaves the clusters up; demo/k3d/down.sh removes them.
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)
cd "$root"
# shellcheck source=scripts/k3d-lib.sh
source scripts/k3d-lib.sh
e2e_lock

k() { KUBECONFIG="$root/demo/k3d/.kube/$1" kubectl "${@:2}"; }
scene() { printf '\n=== %s\n' "$*"; }
say() { echo "  $*"; }
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
device_log() { curl -s "$device_url/log"; }
claim_phase() { k "$1" -n work get deviceclaim job -o jsonpath='{.status.phase}' 2>/dev/null; }

scene "Build the images and start the mesh"
scripts/images.sh
say "table at $(demo/k3d/mesh.sh up)"

scene "Two clusters come up on their own and join through the table"
for name in a b; do
  demo/k3d/join.sh "$name"
done
k a get members

scene "1. A device created in cluster a appears in b"
k a apply -f - >/dev/null <<YAML
apiVersion: solas.dev/v1alpha1
kind: Device
metadata:
  name: gpu-1
  labels: {kind: gpu}
spec:
  description: the only GPU in the fleet
YAML
for name in b; do
  wait_for 30 "gpu-1 visible in $name" k "$name" get device gpu-1
  say "cluster $name sees: $(k "$name" get device gpu-1 -o name)"
done

scene "2. Claims in a and b race for gpu-1; one wins"
docker rm -f solas-device-gpu-1 >/dev/null 2>&1 || true
docker run -d --name solas-device-gpu-1 --network solas-mesh solas-demo:dev device >/dev/null
device_url="http://$(docker inspect -f '{{(index .NetworkSettings.Networks "solas-mesh").IPAddress}}' solas-device-gpu-1):9000"
curl_ready() { curl -sf "$device_url/log"; }
# The device gatekeeper is on the mesh network; the host reaches it too.
wait_for 30 "the device gatekeeper starts" curl_ready
for name in a b; do
  sed -e "s|CLUSTER_ID|$name|" -e "s|DEVICE_URL|$device_url|" demo/k3d/manifests/claim.yaml | k "$name" apply -f - >/dev/null &
done
wait
one_bound() {
  local n=0
  for name in a b; do
    [ "$(claim_phase "$name")" = Bound ] && n=$((n + 1))
  done
  [ "$n" = 1 ]
}
wait_for 60 "exactly one claim Bound" one_bound
if [ "$(claim_phase a)" = Bound ]; then winner=a loser=b; else winner=b loser=a; fi
holder=$(k "$loser" get device gpu-1 -o jsonpath='{.status.claimRef.member}/{.status.fencingToken}')
[ "$holder" = "$winner/1" ] || fail "gpu-1 holder is $holder, want $winner/1"
sleep 3
[ "$(claim_phase "$loser")" = Pending ] || fail "the losing claim in $loser is $(claim_phase "$loser"), want Pending"
say "winner: $winner (token 1). $loser stays Pending."
uses_by() { device_log | grep -q "\"claim\":\"$1/work/job\",\"token\":$2,\"accepted\":$3"; }
wait_for 30 "the workload in $winner uses gpu-1" uses_by "$winner" 1 true
say "the device accepted uses from $winner with token 1"

scene "3. $winner freezes past its lease; $loser takes over; fencing stops $winner"
old_uid=$(k "$winner" get member "$winner" -o jsonpath='{.metadata.uid}')
docker pause "k3d-e2e-$winner-server-0" >/dev/null
say "paused cluster $winner; its lease is 10s"
loser_bound() { [ "$(claim_phase "$loser")" = Bound ]; }
wait_for 90 "the claim in $loser becomes Bound" loser_bound
holder=$(k "$loser" get device gpu-1 -o jsonpath='{.status.claimRef.member}/{.status.fencingToken}')
[ "$holder" = "$loser/2" ] || fail "gpu-1 holder is $holder, want $loser/2"
say "$loser's sweeper expired $winner; $loser holds gpu-1 with token 2"
wait_for 30 "the workload in $loser uses gpu-1" uses_by "$loser" 2 true
docker unpause "k3d-e2e-$winner-server-0" >/dev/null
say "unpaused $winner; its workload still thinks it holds token 1"
wait_for 60 "the device rejects $winner's stale token" uses_by "$winner" 1 false
say "the device REJECTED $winner's use with token 1 after $loser used token 2"

scene "4. $winner joins again with a new member UID; its old claim is Lost"
new_uid() {
  local uid
  uid=$(k "$winner" get member "$winner" -o jsonpath='{.metadata.uid}')
  [ -n "$uid" ] && [ "$uid" != "$old_uid" ]
}
wait_for 90 "$winner joins again" new_uid
lost() { [ "$(claim_phase "$winner")" = Lost ]; }
wait_for 60 "the claim in $winner becomes Lost" lost
say "$winner member uid: $old_uid -> $(k "$winner" get member "$winner" -o jsonpath='{.metadata.uid}')"
say "claim in $winner: Lost; claim in $loser: $(claim_phase "$loser")"

scene "Device log"
device_log | tr '}' '\n' | grep -o '"claim":"[^"]*","token":[0-9]*,"accepted":[a-z]*' | uniq -c

scene "PASS"
echo "Remove the demo with: demo/k3d/down.sh"
