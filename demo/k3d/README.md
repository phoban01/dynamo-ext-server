# The solas demo

Two Kubernetes clusters share one pool of devices. They have no link to
each other. Each cluster runs solas and reads and writes one DynamoDB
table. The table is the only thing they share.

## Run it

```sh
devbox run demo        # build, bring up, and run every step
devbox run demo-down   # remove the clusters, the table, and the network
```

`devbox run e2e` runs the same steps as a Go test.

The demo needs Docker and about 2 GB of free memory. It uses k3d
(ADR 0009) and `amazon/dynamodb-local` (ADR 0007). It shortens the lease
to 10 seconds, with a margin of 1 second and a sweep every 3 seconds, so
each step takes seconds, not minutes.

## What it shows

| Step | What happens | Spec |
|------|--------------|------|
| Join | `join.sh a` and `join.sh b` each bring up a cluster, deploy solas, and create a `Member`. | 7.2 |
| 1 | A `Device` created in cluster `a` appears in cluster `b`. | 2, 4 |
| 2 | Claims in `a` and `b` race for one device. One wins with fencing token 1. The other stays `Pending`. | 5.3, 6.3 |
| 3 | The winner's node is paused for longer than its lease. The other cluster's sweeper deletes the winner's `Member` and frees the device. The other claim binds it with token 2. When the winner wakes up, its workload still believes it holds the device and uses token 1. The device rejects it. | 6.6, 8 |
| 4 | The winner's controller finds its `Member` gone. It joins again with a new UID, and its old claim becomes `Lost`. | 6.5, 7.3 |

Step 3 is the reason for fencing tokens (ADR 0008). The paused workload
acts on an old view, and no clock check can stop it. The device sees the
higher token first, so it rejects the lower one.

## The parts

- `mesh.sh up|down|endpoint`: the Docker network `solas-mesh` and the
  table container `solas-ddb`.
- `join.sh <name>`: the k3d cluster `e2e-<name>`, with solas-apiserver and
  solas-controller. Its kubeconfig goes to `demo/k3d/.kube/<name>`.
- `manifests/claim.yaml`: a `DeviceClaim` for any GPU, and a workload that
  uses the device with the claim's token.
- `cmd/solas-demo`: the device gatekeeper (`device`) and the workload
  (`workload`). They are not part of solas.
- `demo.sh`: the steps above, with a check after each one.
- `down.sh`: removes everything.

## Run one step by hand

```sh
demo/k3d/mesh.sh up
demo/k3d/join.sh a
KUBECONFIG=demo/k3d/.kube/a kubectl get members,devices
KUBECONFIG=demo/k3d/.kube/a kubectl -n solas-system logs deploy/solas-controller
```

`demo/k3d/down.sh` removes everything when you are done.
