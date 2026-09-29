# solas

A Kubernetes extension API server that stores custom resources in
DynamoDB.

Several management clusters share global state through one DynamoDB
table. There is no central control cluster. Clusters can join and leave.
The demo uses a pool of devices: each cluster can lease a device, and a
device is bound to at most one claim at a time. Fencing tokens stop a
cluster that paused from using a device that another cluster now holds.

- [The demo](demo/k3d/README.md): two clusters, a race for a device, a
  reclaim, and fencing. Run it with `devbox run demo`.
- [The specification](spec/solas.md) and the [ADRs](docs/adr/).
- [The plan](docs/plan.md).

`devbox run verify` runs the unit tests, the Quint model checks, and the
Duvet traceability gate.
