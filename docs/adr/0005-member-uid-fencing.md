# ADR 0005: The member UID is the fencing token

Status: accepted

## Context

A member cluster can crash or lose its network. Its devices must then
become free. But a member that only paused can wake up and continue to
act on old state. We must stop it from writing a binding that looks
valid.

## Decision

Each member cluster owns a `Member` object with a lease. The object gets
a new UID each time the cluster joins. Every `claimRef` records the UID
of the member that wrote it.

A sweeper in any cluster deletes a `Member` whose lease has expired. It
then clears each `claimRef` whose member UID has no `Member`. A member
whose `Member` is gone must join again with a new UID. It marks its old
claims `Lost`.

## Consequences

- A binding from an old member identity never looks current, because
  its UID matches no `Member`.
- Deletes and clears use resource version preconditions. A renew and a
  delete of the same `Member` cannot both succeed.
- Lease safety depends on a bound on clock rate drift. The spec states
  the bound and the safety margin.

## Rejected alternatives

- The member name as the token. A cluster that joins again has the same
  name, so old and new bindings look the same.
- Fencing by a counter in each write. The UID gives the same effect with
  no extra item.
