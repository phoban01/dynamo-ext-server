# ADR 0006: The API groups are solas.dev and claims.solas.dev

Status: accepted, amended in M5

## Context

The extension API server and the `DeviceClaim` CRD need API groups and a
version.

## Decision

`Device` and `Member` use the group `solas.dev`, version `v1alpha1`.
They are cluster-scoped. The extension API server serves them.

`DeviceClaim` uses the group `claims.solas.dev`, version `v1alpha1`. It
is namespaced. The kube-apiserver of each cluster serves it as a CRD.

## Consequences

- One `APIService` object, `v1alpha1.solas.dev`, registers the extension
  server.
- `v1alpha1` gives no compatibility promise. We can change the types
  until we move to `v1beta1`.

## Amendment

The first version of this ADR put `DeviceClaim` in `solas.dev` too. It
said that a CRD version and an `APIService` version of one group do not
conflict when no resource name is in both. That is wrong. The aggregator
routes a whole group and version to one server, so every request for
`solas.dev/v1alpha1` goes to the extension server. The CRD machinery
would also register an `APIService` with the same name,
`v1alpha1.solas.dev`. So the CRD needs its own group.
