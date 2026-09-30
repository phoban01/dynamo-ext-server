# ADR 0011: Device metadata, CEL selection, priority, and preemption

Status: accepted

## Context

The demo should be about devices and claims. Users want to see:

- more about a device: its attributes and its health;
- who holds a device, at what priority, and when the holder's lease ends;
- claims that select devices by any field, not only by label;
- claims with a priority, so an urgent claim can take a device from a
  less urgent one, when the device allows it.

A claim does not ask for a lease length. A claim holds its device until
it is deleted, preempted, or lost.

## Decision

- `Device.spec.attributes` holds free-form metadata.
  `Device.status.conditions` holds health, set by whoever runs the device.
- `Device.spec.preemptible` allows preemption.
  `Device.spec.preemptionGracePeriodSeconds` sets the grace period.
- The `claimRef` of a device also holds the priority of the claim and the
  time of the bind.
- `DeviceClaim.spec.priority` is a number. A higher number wins.
- `DeviceClaim.spec.selector.cel` is a CEL expression over the device. It
  must match, together with the label selector.
- `DeviceClaim.status.leaseExpiresAt` shows when the holder's lease ends
  by the holder's clock. It is for display only.
- Preemption is a request, then a grace period:
  1. A pending claim that finds no free device may write a preemption
     request on a preemptible, matching device whose holder has a lower
     priority.
  2. The holder sets its claim to `Preempting`. Its workload may go on.
  3. When the grace period ends on the holder's clock, the holder sets
     its claim to `Preempted`, clears its `claimRef`, and its claim goes
     back to `Pending`.
  4. While a request stands, only the preemptor may bind the device. The
     bind clears the request and raises the fencing token.
  5. Before each renew, the holder handles each request on its devices.
     If it cannot, it does not renew. A holder that stops responding
     loses its lease, and the sweeper frees its device.

## Consequences

- No claim in effect loses its device. A stuck holder delays a
  preemption by at most one lease duration D.
- A claim in `Preempting` is still in effect. A claim in `Preempted` is
  not.
- The server checks the priority rules, so a client cannot preempt with
  an equal or lower priority.
- The Quint model gains priorities and the preemption steps.


## Rejected alternatives

- A lease length on each claim. Not wanted now.
- Immediate preemption. It cuts off running work with no warning.
- A server-side clock for the grace period. solas does not trust clocks
  across clusters, so each side measures on its own clock.

## Amendment

The first version let a preemptor clear the holder's `claimRef` after the
grace period plus D. The Quint model found that this takes a device from
a live holder whose claim is still in effect, when the holder's
controller has not handled the request. Fencing kept device use safe,
but the claim lost its device with no warning. The negative model
`quint/negative/force-clear.qnt` shows it. The holder now handles
requests before each renew instead, and a stuck holder loses its lease.
