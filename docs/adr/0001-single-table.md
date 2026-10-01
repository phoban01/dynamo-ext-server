# ADR 0001: One DynamoDB table in one region

Status: accepted, amended by ADR 0014

## Context

Member clusters share global state. The binding of a device to a claim
must be safe: a device is bound to at most one claim at a time. Safety
needs a compare-and-set on the device record that every cluster sees at
once.

DynamoDB offers conditional writes and transactions. These are strongly
consistent inside one region. Global tables in the default eventual mode
replicate across regions with "last writer wins". Two regions can then
accept two conflicting conditional writes, and one write is lost later.

## Decision

All member clusters use one DynamoDB table in one region. Every read that
safety depends on is a strongly consistent read.

## Consequences

- Conditional writes and transactions give a single order of writes to
  each item.
- Clusters in other regions pay cross-region latency on each write.
- The region is a single point of failure. When it is down, clusters
  cannot bind or release devices. Local work in each cluster continues.

## Rejected alternatives

- Global tables in eventual mode. Conflicting writes in two regions can
  both succeed, which breaks the safety rule.
- Global tables in multi-region strong consistency mode. This mode can
  keep the safety rule, but it limits regions and features. We can
  revisit it after the demo works in one region.
- A table per cluster with our own replication. This needs a consensus
  protocol, which is the work DynamoDB already does for us.

## Amendment

ADR 0014 adds a second store: one etcd cluster. The flag `--storage-url`
picks the store. The rules of this ADR still hold for the DynamoDB store.
