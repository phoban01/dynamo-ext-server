# ADR 0012: One solas binary and one Deployment

Status: proposed

## Context

Each member cluster runs two binaries today: solas-apiserver and
solas-controller, in two Deployments. Operators want one thing to
install and to upgrade.

## Decision

One binary, `solas`, runs both parts in one process. One Deployment runs
it.

- The API server part serves requests on every replica.
- The controller part runs only on the replica that holds the leader
  election lease in the cluster, spec 6.2.
- The controller part reaches the API through the cluster's
  kube-apiserver, as today, so the aggregation layer, authentication,
  and authorization stay the same.
- One ConfigMap sets the cluster ID and the lease settings. One Secret
  sets the table endpoint and credentials.

## Consequences

- One image, one Deployment, one set of RBAC rules to install.
- A crash of the process stops both parts. The API server comes back
  with the Deployment; the controller part joins again as spec 7.2
  describes.
- The controller part starts only after the API server part is ready,
  because it reads Devices and Members through it.

## Rejected alternatives

- Keep two Deployments. More to install and keep in step.
- A sidecar pattern with two containers in one Pod. Still two binaries
  and two images.
