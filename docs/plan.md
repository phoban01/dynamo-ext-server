# Plan

This document is the project plan. It proposes an architecture, lists the
decisions to confirm, and splits the work into milestones. Each milestone
ends with a command that proves it is done.

## Goal

Run several Kubernetes management clusters with no central control
cluster. Global state lives in custom resources that an extension API
server stores in DynamoDB. Every member cluster sees the same global state.

The demo proves the pattern with two resources:

- `Device` is global. Its status shows which claim holds it.
- `DeviceClaim` is local to one cluster. It asks for one device.

The core safety rule: a device is bound to at most one claim at a time.

## Architecture

### Components

Each member cluster runs two things:

1. `solas-apiserver`, an aggregated API server. It serves the global
   resources (`Device`, `Member`) and stores them in DynamoDB. An
   `APIService` object registers it with the cluster's kube-apiserver.
2. `solas-controller`, a controller manager. It runs the claim controller,
   the membership heartbeat, and the reclaim sweeper.

All clusters point at the same DynamoDB table. The demo uses one dynamodb-local
container for this table.

```
 cluster A                     cluster B                     cluster C
 kube-apiserver                kube-apiserver                kube-apiserver
   | APIService                  | APIService                  | APIService
 solas-apiserver --+           solas-apiserver --+           solas-apiserver --+
 solas-controller  |           solas-controller  |           solas-controller  |
                   |                             |                             |
                   +-----------------------------+-----------------------------+
                                                 |
                                      DynamoDB table "solas"
```

### Resources

API group `solas.dev`, version `v1alpha1`.

| Kind          | Scope      | Store      | Purpose                              |
|---------------|------------|------------|--------------------------------------|
| `Device`      | cluster    | DynamoDB   | A leasable device. `status.claimRef` names the holder. |
| `Member`      | cluster    | DynamoDB   | One per member cluster. Holds the heartbeat lease. |
| `DeviceClaim` | namespaced | local etcd | A CRD. A request for one device.     |

`Device.status.claimRef` holds the member name, the member UID, the claim
namespace, the claim name, and the claim UID.

`DeviceClaim` is a plain CRD in each cluster's own etcd. This keeps
DynamoDB for global state only. `Device.status.claimRef` is the single
source of truth for a binding. `DeviceClaim.status` is a copy of it.

### DynamoDB storage layer

`pkg/storage/dynamo` implements `k8s.io/apiserver/pkg/storage.Interface`.
The generic registry and the watch cache sit on top of it, as they do
for etcd.

One table holds three kinds of item:

| Item    | Partition key       | Sort key          | Content                    |
|---------|---------------------|-------------------|----------------------------|
| object  | `obj#<resource>`    | `<ns>/<name>`     | object bytes, `rv`         |
| counter | `rv`                | `<resource>`      | last issued `rv`           |
| event   | `ev#<resource>`     | `rv` (number)     | type, object, prev object, TTL |

Every write is one `TransactWriteItems` call with three parts:

1. Increment the resource counter from `n` to `n+1`, on condition that it
   is still `n`.
2. Put or delete the object, on condition that its `rv` equals the
   expected value, or that it does not exist for a create.
3. Put an event item with sort key `n+1`.

This gives a strictly increasing resource version per resource. A
transaction for `n+1` cannot commit before the transaction for `n`, so the
event log has no gaps. Reads use `ConsistentRead`.

Watch polls the event partition for items with `rv` greater than the last
one seen. Events expire through DynamoDB TTL. A watch from an expired
`rv` returns `410 Gone`, as etcd does after compaction. DynamoDB Streams
can replace polling later.

The counter item serializes all writes to one resource. A single item
takes about 1000 writes per second. That is enough for this demo.

We test the layer with the storage conformance suite in
`k8s.io/apiserver/pkg/storage/testing`, the same suite that tests the etcd3
store.

### Claim flow

1. A user creates a `DeviceClaim` in cluster A.
2. The claim controller in A picks a free `Device` that matches the claim.
3. It updates `Device.status.claimRef` with the device's current `rv` as a
   precondition. DynamoDB applies this as a conditional write.
4. If the write fails on a conflict, the controller picks again.
5. On success, the controller sets `DeviceClaim.status.deviceName` and the
   phase `Bound`.
6. When the claim is deleted, a finalizer clears `claimRef`. The clear is
   conditional on `claimRef` still naming this claim.

The `Device` status strategy in `solas-apiserver` also rejects any update
that changes `claimRef` from one holder to another holder. A holder must
release before a new holder binds. The conditional write and this check
together enforce the safety rule.

### Membership, join, and leave

- Join: a cluster installs `solas-apiserver` and `solas-controller` with a
  cluster ID and table credentials. The controller creates its `Member`
  object. The object gets a new UID on each join.
- Heartbeat: the controller renews `Member.status.renewTime` on a fixed
  period. The lease duration is in `Member.spec`.
- Graceful leave: the controller marks the member as draining, releases
  all its claims, and deletes its `Member`.
- Crash: the lease expires. The reclaim sweeper in any other cluster
  deletes the stale `Member`. It then clears each `claimRef` whose member
  UID has no live `Member`.

Observers measure lease expiry with their own clock, from the moment they
last saw `renewTime` change. The holder treats its own lease as expired
earlier than observers do. The spec states the clock drift bound that this
needs.

A cluster that comes back after a crash finds that its member UID is gone.
It joins again with a new UID and marks its old claims `Lost`. The member
UID acts as a fencing token.

## Decisions to confirm

These become ADRs in `docs/adr/`. After approval, they go into the
"Architecture decisions already made" section of `CLAUDE.md`.

1. One DynamoDB table in one region is the shared store. Multi-region
   global tables in eventual mode do not support safe conditional writes
   across regions, so they are out of scope. Multi-region strong
   consistency mode is a later option.
2. `DeviceClaim` is a CRD in local etcd. `Device` and `Member` live in
   DynamoDB.
3. Resource versions come from one counter item per resource.
4. Watch reads a TTL event log by polling.
5. The member UID is the fencing token for reclaim.
6. The API group is `solas.dev/v1alpha1`.

## Safety and liveness properties

The spec states these. The Quint model checks them. The code cites them
through Duvet.

- `NoDoubleBind`: no two claims are bound to the same device.
- `ClaimMatchesDevice`: if a claim is `Bound` to device D, then D's
  `claimRef` names that claim, or the claim's member is no longer live.
- `NoEarlyReclaim`: a sweeper does not clear a `claimRef` while the
  holder's lease is still valid by the holder's own clock.
- `MonotonicRV`: each resource's `rv` strictly increases.
- `WatchComplete`: a watcher from `rv` n sees every event after n, in
  order, or gets `410 Gone`.
- `EventualRelease` (liveness): if a member leaves or crashes, its devices
  become free.
- `EventualBind` (liveness): a claim for a free, matching device becomes
  `Bound` while its member is live.

## Milestones

Each milestone lists its exit command. The issues for a milestone are
small. An agent can finish each one in under five minutes. Each issue
names its spec sentence, the task, the files, and the command that proves
it is done.

### M0: Repository and tooling

- `devbox.json` with Go, Node and Quint, a JDK for Apalache and TLC, k3d,
  kubectl, the AWS CLI, Rust for Duvet, and golangci-lint.
- The resource caps from `CLAUDE.md`: `JVM_ARGS`, `JAVA_TOOL_OPTIONS`,
  `GOFLAGS=-p=4`, and four simulator shards.
- devbox scripts: `test`, `sim-gate`, `e2e`, `verify`, `duvet-install`.
  Each script is a stub that exits 0 until its milestone lands.
- `.github/workflows/verify.yml` runs `devbox run verify`.
- An empty `.duvet/config.toml` and `.duvet/snapshot.txt`.

Exit: `devbox run verify` passes locally and in CI.

### M1: Specification

- `spec/solas.md` with numbered sections: storage, resource versions, watch,
  device, claim, membership, and reclaim. Requirements use MUST and
  SHOULD, one per sentence.
- The ADRs from "Decisions to confirm".
- The Duvet config points at `spec/solas.md`. The snapshot lists every
  requirement as not yet cited.

Exit: `devbox run duvet-report` lists every requirement.

### M2: Quint model

- `quint/solas.qnt` models members, devices, claims, the store as a
  linearizable map with conditional writes, lease clocks, crash, join,
  and leave.
- Invariants for each safety property.
- Negative models in `quint/negative/`. Each one breaks one rule, for
  example an unconditional write or a reclaim without the lease check.
  Each one must fail its invariant.
- `scripts/quint-check.sh` runs `quint run` on every model, then a bounded
  `quint verify` on the main model. Positive models must pass. Negative
  models must fail.

Exit: `scripts/quint-check.sh` passes, and `devbox run verify` calls it.

### M3: DynamoDB storage layer

- `pkg/storage/dynamo` with `Create`, `Get`, `GetList`, `GuaranteedUpdate`,
  `Delete`, `Watch`, and the other `storage.Interface` methods.
- Table bootstrap code and a dynamodb-local test fixture.
- The `k8s.io/apiserver` storage conformance tests run against
  dynamodb-local.
- Duvet citations on each MUST sentence in the storage, resource version,
  and watch sections.

Exit: `devbox run test` passes with the race detector.

### M4: Extension API server

- API types for `Device` and `Member`, with generated deepcopy, clients,
  informers, and OpenAPI.
- `cmd/solas-apiserver` built on `k8s.io/apiserver`, with the DynamoDB
  storage from M3.
- A `Device` status strategy that rejects a direct holder-to-holder
  change of `claimRef`.
- Deployment manifests: `APIService`, RBAC, serving certificates, and the
  table endpoint and credentials.

Exit: unit tests for the strategies pass. A single k3d cluster serves
`kubectl get devices`.

### M5: Controllers

- The `DeviceClaim` CRD.
- `cmd/solas-controller` with the claim controller, the member heartbeat,
  the graceful leave path, and the reclaim sweeper.
- Unit tests against a fake store.

Exit: `devbox run test` passes.

### M6: Simulator gate

- A deterministic simulator runs the real controller code against an
  in-memory store. It injects crashes, partitions, delays, and clock
  drift from a seed.
- The simulator checks the same invariants as the Quint model after every
  step.
- `docs/sim-gate.md` describes the simulator and how to replay a failing
  seed.
- `.github/workflows/sim-gate.yml` runs four shards.

Exit: `devbox run sim-gate` passes.

### M7: Two-cluster demo and end-to-end tests

- `demo/k3d/` scripts create two k3d clusters, `a` and `b`, and one
  dynamodb-local container on a shared Docker network. Each cluster comes
  up on its own and joins through the table. Three clusters ran the VM
  out of memory, so the demo uses two.
- The demo shows four things. A device created in `a` appears in `b`.
  Claims in `a` and `b` race for one device, and only one wins. The
  winner's node is paused past its lease; the other cluster reclaims the
  device, and the device gatekeeper rejects the winner's stale fencing
  token. The winner joins again with a new member UID, and its claim
  becomes `Lost`.
- `test/e2e/` uses `sigs.k8s.io/e2e-framework` with plain `testing`. It
  takes the e2e lock and deletes only clusters named `e2e-*`.

Exit: `devbox run demo` and `devbox run e2e` pass.

### M8: Pivot from etcd

- `solas-pivot` moves Devices from an existing CRD in a cluster's etcd
  into solas: `copy`, `verify`, and a mutating admission webhook for the
  cutover. ADR 0010, spec section 9.

Exit: `devbox run pivot-smoke` passes.

### M9: Model-based tests

- A Quint run exports ITF traces.
- A Go driver replays each trace against a real `solas-apiserver` and the
  controllers. It checks that the observed state matches the trace state
  after each step.

The driver replays the traces against the real REST stores of
solas-apiserver on dynamodb-local. It needs no cluster, so `verify` runs
it. The simulator gate covers the controllers. See `docs/mbt.md`.

Exit: `devbox run mbt` passes, as part of `devbox run verify`.

### M10: Confidence ladder

- `docs/confidence.md` lists each level of evidence: spec, Duvet
  coverage, Quint checks, negative models, unit tests, the simulator,
  model-based tests, and end-to-end tests. It says what each level
  proves and what it does not.
- The Duvet gate requires every MUST sentence to have a citation and a
  test.

Exit: `devbox run verify` passes with the full Duvet gate.

## Order and parallel work

M0 comes first. M1 blocks M2 to M5. M2 and M3 can run in parallel. M4
needs M3. M5 needs M4. M6 needs M5. M7 needs M4 and M5. M8 needs M4.
M9 needs M2 and M7. M10 closes the project.

Only one heavy job runs at a time on the development VM, as `CLAUDE.md`
says. Two agents can work in parallel on code, but not on e2e or
`quint verify` runs.

## Risks

- `storage.Interface` changes between Kubernetes minor versions. Pin one
  `k8s.io/apiserver` version and upgrade on purpose.
- Watch polling adds latency and read cost. The watch cache hides most of
  it, and DynamoDB Streams is the fallback.
- dynamodb-local may differ from DynamoDB in transaction or TTL behavior. The
  conformance tests should run against real DynamoDB at least once before
  any claim about production use.
- Two k3d clusters and dynamodb-local use about 1.5 GB of memory. The
  e2e lock and the one-heavy-job rule manage this.
- Lease safety depends on a clock drift bound. The spec must state it,
  and the model must include drift.
