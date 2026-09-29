# ADR 0008: Fencing tokens guard device use

Status: accepted

## Context

A claim is safe to use only while its member is live by the member's own
clock. The controller sets a claim to `Suspended` only after the lease
ends, and a paused controller can be late. A workload that reads only
the claim phase can use a device after a sweeper has reclaimed it and a
new claim holds it (issue #42).

Clocks cannot close this gap. A process can pause for any time between
its last check and its next use of the device.

## Decision

The device itself rejects a stale holder, with a fencing token.

- `Device.status.fencingToken` is a number. The server sets it to the
  old value plus 1 each time `claimRef` goes from empty to set. Clients
  cannot set it.
- The claim holds a copy of the token of its bind.
- A workload presents its token each time it uses the device.
- The device, or a gatekeeper in front of it, remembers the highest
  token it has seen. It rejects a lower token and accepts an equal or
  higher one.

## Consequences

- Once the new holder has used the device, the old holder cannot use it
  again. This holds with no bound on pauses and no bound on clock drift.
- The device or its gatekeeper must check tokens. A device that cannot
  check tokens gets no protection from this design.
- The lease margin `M` still decides when a sweeper may reclaim. It
  affects how often a holder loses a device, not whether two holders use
  it at once.
- An old holder can still use the device after reclaim and before the
  new holder's first use. The device sees these uses in a single order,
  so they never overlap.

## Rejected alternatives

- Publish the local lease end in a `Lease` object. Workloads would
  compare timestamps across nodes, which needs synced clocks.
- Suspend claims a margin before the lease ends. A pause longer than
  the margin still breaks it.
- Give workloads a local token with an expiry. A pause between the
  expiry check and the use still breaks it.
