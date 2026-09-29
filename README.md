# dynamo-ext-server

A Kubernetes extension API server that stores custom resources in
DynamoDB.

Several management clusters share global state through one DynamoDB
table. There is no central control cluster. Clusters can join and leave.
The demo uses a fleet of devices: each cluster can lease a device, and a
device is bound to at most one claim at a time.

Status: planning. See [docs/plan.md](docs/plan.md).
