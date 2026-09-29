# ADR 0006: The API group is solas.dev/v1alpha1

Status: accepted

## Context

The extension API server and the `DeviceClaim` CRD need an API group
and a version.

## Decision

All resources use the group `solas.dev` and the version `v1alpha1`.
`Device` and `Member` are cluster-scoped. `DeviceClaim` is namespaced.

## Consequences

- One `APIService` object, `v1alpha1.solas.dev`, registers the extension
  server.
- `DeviceClaim` is a CRD in the same group. The kube-apiserver serves a
  CRD version and an `APIService` version of one group without conflict,
  as long as no resource name is in both.
- `v1alpha1` gives no compatibility promise. We can change the types
  until we move to `v1beta1`.
