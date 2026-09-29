# ADR 0003: Resource versions come from a counter item

Status: accepted

## Context

The Kubernetes API needs a `resourceVersion` on each object. Watch and
list use it to resume. Within one resource, the versions must strictly
increase in commit order. etcd gives this through its global revision.
DynamoDB has no such counter.

## Decision

Each resource has one counter item in the table. Every write is one
`TransactWriteItems` call. The call increments the counter from `n` to
`n+1` on condition that it is still `n`, writes the object, and appends
an event with version `n+1`.

## Consequences

- Versions within a resource strictly increase and have no gaps.
- A transaction for `n+1` cannot commit before the transaction for `n`.
  So the event log is complete up to the counter value.
- All writes to one resource contend on one item. This limits a resource
  to roughly 1000 writes per second. That is enough for this project.
- Versions from two resources do not compare. Kubernetes does not need
  them to.

## Rejected alternatives

- A timestamp as the version. Clocks on different servers disagree, so
  the order is not safe.
- One counter for the whole table. This makes every resource contend on
  one item, and gains nothing.
- The DynamoDB Streams sequence number. Streams are per shard and are
  not known at write time.
