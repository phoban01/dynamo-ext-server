# ADR 0002: DeviceClaim is a CRD in local etcd

Status: accepted

## Context

A `Device` is global. Every member cluster must see it and its binding.
A `DeviceClaim` belongs to one cluster. Workloads in that cluster create
and read it. Other clusters need to know only which claim holds a device.

## Decision

`DeviceClaim` is a plain CRD. Each cluster stores its claims in its own
etcd. `Device` and `Member` are served by the extension API server and
stored in DynamoDB.

`Device.status.claimRef` is the single source of truth for a binding.
`DeviceClaim.status` holds a copy of it.

## Consequences

- DynamoDB holds only global state. Its write load does not grow with
  local claim traffic.
- A claim and its device cannot change in one transaction. The claim
  controller must handle a crash between the device write and the claim
  write. The spec gives the rules for this.
- A cluster that leaves takes its claims with it. The reclaim rules free
  the devices that those claims held.

## Rejected alternatives

- Store `DeviceClaim` in DynamoDB too. This would allow one transaction
  for both records. But every cluster would then store every other
  cluster's claims, and the claim would no longer be cluster-local.
