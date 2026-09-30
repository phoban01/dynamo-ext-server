# The solas demo

Two Kubernetes clusters share one pool of devices. They have no link to
each other. Each cluster runs solas and reads and writes one DynamoDB
table. The table is the only thing they share.

## Run it

```sh
devbox run demo        # build, bring up, and run every scene
devbox run demo-down   # remove the clusters, the table, and the network
```

`devbox run e2e` runs the same scenes as a Go test. `demo/k3d/demo.sh`
and `demo/k3d/down.sh` also work on their own: outside devbox, they run
themselves again through devbox, which has the tools they need.

The demo needs Docker and about 2 GB of free memory. It uses k3d
(ADR 0009) and `amazon/dynamodb-local` (ADR 0007). The setup runs
quietly and writes its log to `demo/k3d/setup.log`. It shortens the
lease to 10 seconds, with a margin of 1 second and a sweep every 3
seconds, so each scene takes seconds, not minutes.

## What it shows

| Scene | What happens | Spec |
|-------|--------------|------|
| 1 | Eight devices are created in cluster `a` and appear in cluster `b`. Each has labels and attributes. The demo sets a `Ready` condition on each; `gpu-t4-2` reports `Ready=False`. | 2, 4, 10.1 |
| 2 | Cluster `a` claims an A100, a small GPU, and a 400G NIC. Cluster `b` claims an A100 and the FPGA. CEL picks by attribute and by condition, so `infer` skips `gpu-t4-2`. Three devices stay free. | 6.3, 10.3 |
| 3 | `urgent` in `b` has priority 5 and wants an A100. Both are taken, so it asks for the one that `a/train` holds at priority 1. `train` becomes `Preempting`, keeps the device for its 10 second grace period, then lets it go and goes back to `Pending`. `urgent` binds with token 2. | 10.5 to 10.7 |
| 4 | Claims `job` in `a` and in `b` race for `gpu-h100-1`. One wins with token 1. The other stays `Pending`. | 5.3, 6.3 |
| 5 | The winner's node is paused for longer than its lease. The other cluster's sweeper deletes the winner's `Member` and frees its devices. The other `job` binds the H100 with token 2. When the winner wakes up, its workload still believes it holds the device and uses token 1. The device rejects it. | 6.6, 8 |
| 6 | The winner's controller finds its `Member` gone. It joins again with a new UID, and its old claims become `Lost`. | 6.5, 7.2 |

The device table shows the holder, its priority, the fencing token, and
any preemption request. The claim table shows the phase, the device, the
priority, the token, and when the member's lease ends.

Scene 5 is the reason for fencing tokens (ADR 0008). The paused workload
acts on an old view, and no clock check can stop it. The device sees the
higher token first, so it rejects the lower one.

## The parts

- `manifests/devices.yaml`: the device pool.
- `manifests/claims-a.yaml`, `manifests/claims-b.yaml`: the claims of
  each cluster, with label and CEL selectors and priorities.
- `manifests/urgent.yaml`: the claim that preempts.
- `manifests/claim.yaml`: the `job` claim for the H100, and a workload
  that uses the device with the claim's token.
- `cmd/solas-demo`: the device gatekeeper (`device`) and the workload
  (`workload`). They are not part of solas.
- `mesh.sh up|down|endpoint`: the Docker network `solas-mesh` and the
  table container `solas-ddb`.
- `join.sh <name>`: the k3d cluster `e2e-<name>`, with solas deployed.
  Its kubeconfig goes to `demo/k3d/.kube/<name>`.
- `demo.sh`: the scenes above, with a check after each one.
- `down.sh`: removes everything.

## Try it by hand

```sh
demo/k3d/mesh.sh up
demo/k3d/join.sh a
export KUBECONFIG=demo/k3d/.kube/a
kubectl apply -f demo/k3d/manifests/devices.yaml
kubectl apply -f demo/k3d/manifests/claims-a.yaml
kubectl get devices
kubectl get deviceclaims -n work
```

Without a `Ready` condition, the CEL selectors that check it match no
device. Set one with a status patch:

```sh
kubectl patch device gpu-a100-1 --subresource=status --type=merge -p \
  '{"status":{"conditions":[{"type":"Ready","status":"True","reason":"Healthy",
  "message":"by hand","lastTransitionTime":"2026-01-01T00:00:00Z"}]}}'
```

`demo/k3d/down.sh` removes everything when you are done.
