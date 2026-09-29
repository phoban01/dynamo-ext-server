# Solas specification

## 1. Introduction

### 1.1. Scope

This document specifies solas. Solas is a Kubernetes extension API
server that stores global resources in DynamoDB. Several member clusters
share one table and so share one view of the global resources. The
document also specifies the controller that binds devices to claims.

### 1.2. Terms

- **Member cluster**: a Kubernetes cluster that runs the solas API server
  and the solas controller.
- **Table**: the one DynamoDB table that all member clusters share.
- **Resource**: a group and resource name, for example
  `solas.dev/devices`.
- **Write**: a create, update, or delete of one object.
- **Resource version**: the `metadata.resourceVersion` of an object, or
  the version of a list or a watch.
- **Holder**: the claim that a `Device.status.claimRef` names.
- **Observer**: a member cluster that measures the lease of another
  member.

### 1.3. Conventions

The requirement key words of RFC 2119 and RFC 8174 have the meaning given
there when they appear in all capitals.

Each normative sentence holds one requirement. Code cites the sentence
with a Duvet annotation.

## 2. Storage

### 2.1. Table

All member clusters MUST use the same table.
The table MUST be in one AWS region.
The table MUST have a string partition key named `pk` and a string sort
key named `sk`.
The table MUST enable TTL on the attribute `expires`.
The table name MUST be a setting of the API server.
The default table name is `solas`.

### 2.2. Items

The table holds three kinds of item: object items, counter items, and
event items.

An object item MUST have `pk` equal to `obj#` followed by the resource.
An object item MUST have `sk` equal to the part of its storage key after
the resource name.
The sort key starts with `/`, so it is never empty.
For a namespaced object, `sk` is `/`, the namespace, `/`, and the name.
For a cluster-scoped object, `sk` is `/` and the name.
An object item MUST hold the resource version of its last write in the
number attribute `rv`.
An object item MUST hold the encoded object in the binary attribute
`value`.

A counter item MUST have `pk` equal to `rv` and `sk` equal to the
resource.
A counter item MUST hold the last issued resource version in the number
attribute `n`.

An event item MUST have `pk` equal to `ev#` followed by the resource.
An event item MUST have `sk` equal to its resource version as a decimal
string, padded with zeros to 20 digits.
The padding makes the string order of `sk` equal to the number order of
the versions.
An event item MUST hold the event type in the attribute `type`.
The event type MUST be one of `INIT`, `ADDED`, `MODIFIED`, or `DELETED`.
An `ADDED`, `MODIFIED`, or `DELETED` event item MUST hold the encoded
object in `value`.
An event item for a `MODIFIED` or `DELETED` event MUST hold the encoded
previous object in `prev`.
An event item MUST hold its expiry time, in Unix seconds, in `expires`.

### 2.3. Writes

Every write MUST be one `TransactWriteItems` call.
The call MUST hold three actions: a counter update, an object action, and
an event put.

The counter update MUST set `n` to `n + 1` on condition that `n` still
has the value that the server read.

For a create, the object action MUST be a put on condition that the
object item does not exist.
For an update, the object action MUST be a put on condition that `rv`
equals the version that the server read.
For a delete, the object action MUST be a delete on condition that `rv`
equals the version that the server read.

The event put MUST be on condition that the event item does not exist.

Each call MUST set a `ClientRequestToken`.
The token makes a retry after a lost reply safe, because DynamoDB does not
apply the same token twice.

If the transaction fails only on the counter condition, the server MUST
read the counter again and retry the same write.
The server SHOULD wait a short random time before each such retry.
If the transaction fails on the object condition of a create, the server
MUST return `409 AlreadyExists`.
If the transaction fails on the object condition of an update or a
delete, the server MUST return `409 Conflict`.
The Kubernetes generic registry then retries an update on the new state
where the request allows it.

The server MUST reject a write whose event item would exceed the
DynamoDB item size limit.
The server MUST return `413 RequestEntityTooLarge` for such a write.

### 2.4. Reads

Every read of an object item MUST be a strongly consistent read.
Every read of a counter item MUST be a strongly consistent read.
Every query of an event log MUST be a strongly consistent query.

A get MUST return the object from its object item.

A list MUST read the counter before it queries the object items.
A list MUST read the counter again after the query.
If the two counter values differ, the list MUST retry.
If the two counter values are equal, the list MUST return that value as
its resource version.
A counter that does not change during the query shows that no write
committed during the query.
So the list is a consistent snapshot at that version.

The store keeps only the latest state of each object.
A request for a list at an older, exact resource version MUST be served
by the watch cache, or it MUST fail with `410 Gone`.

## 3. Resource versions

### 3.1. Issue

Each resource MUST have its own counter item.
Kubernetes does not accept 0 as the resource version of a list.
So the counter of a resource never shows 0.
When the server first uses a resource, it MUST create the counter item
with `n` equal to 1 and an event item of type `INIT` at version 1, in one
transaction.
The first write to a resource MUST get resource version 2.
Each write MUST get a resource version exactly one more than the
previous write to the same resource.
The write transaction in section 2.3 gives this, because the counter
update and the object action commit together or not at all.

### 3.2. Order

A transaction for version `n + 1` cannot commit before the transaction
for version `n`, because its counter condition needs the value `n`.
So when the counter shows the value `c`, every write with a version up to
`c` has committed.

The resource version of a resource MUST strictly increase in commit
order.
The server MUST NOT compare resource versions of two different resources.

### 3.3. Objects

The `metadata.resourceVersion` of an object MUST equal the version of the
write that last changed it.
The `rv` attribute of the object item MUST equal the same version.
The object in a `DELETED` event MUST carry the version of the delete.
The server MUST encode each resource version as a decimal string.
Kubernetes clients treat a resource version as an opaque string.

## 4. Watch

### 4.1. Event log

Each write MUST add one event item to the event log of its resource.
The event item MUST have the same resource version as the write.
An event item MUST expire a set retention time after its write.
The retention time MUST be a setting of the API server.
The default retention time is one hour.
The server MUST NOT depend on the time at which DynamoDB deletes an
expired item.
DynamoDB can delete expired items late and in any order.

### 4.2. Poll

A watch from version `r` MUST deliver every event with a version greater
than `r`.
A watch MUST deliver events in version order.
A watch MUST NOT deliver the same event twice.
A watch MUST NOT deliver an `INIT` event.

Let `last` be the version of the last event that the watch delivered, or
`r` before the first event.
Each poll MUST read the counter first.
Let `c` be the counter value.
If `c` equals `last`, the poll MUST return no events.
If `c` is greater than `last`, the poll MUST query the event log for the
versions from `last + 1` to `c`.
Every write up to `c` has committed before the query starts, so a
complete log holds each of these versions.

The server SHOULD poll each resource at least once each second.
The server SHOULD NOT poll a resource more often than every 100
milliseconds.
The server SHOULD run one poller for each resource and share it between
watchers.
The API server watch cache gives this sharing.

### 4.3. Gaps

If the query result does not hold each version from `last + 1` to `c`
exactly once, the watch MUST end with `410 Gone`.
A gap means that DynamoDB has deleted expired events.
A client that gets `410 Gone` lists again and starts a new watch, as
Kubernetes requires.

### 4.4. Start

A watch with no resource version MUST first list the current state.
It MUST then deliver an `ADDED` event for each object in the list.
It MUST then continue from the version of the list.

## 5. Device

### 5.1. Resource

`Device` MUST be a cluster-scoped resource in the group `solas.dev`,
version `v1alpha1`.
The solas API server MUST serve `Device` from the table.
`Device` MUST have a `status` subresource.
Claims select devices by their labels.
`Device.spec.description` holds free text that describes the device.

### 5.2. Claim reference

`Device.status.claimRef` names the holder of the device.
A `claimRef` MUST hold the member name, the member UID, the claim
namespace, the claim name, and the claim UID.
A device is free when it has no `claimRef`.
A device is bound when it has a `claimRef`.

### 5.3. Status updates

The server MUST NOT accept an unconditional update of `Device` status.
Each status update MUST carry the resource version that the client read.
The server MUST reject a status update that changes `claimRef` from one
holder to a different holder.
A holder is different when any field of `claimRef` differs.
To move a device, a client MUST first clear `claimRef` and then set it
in a second update.
The server MUST reject a status update that sets a `claimRef` with an
empty field.

A spec update MUST NOT change `status`.
A status update MUST NOT change `spec`.

### 5.4. Delete

The server MUST reject a delete of a bound device.
The server MUST check this on the state that the delete removes, using
the delete condition on `rv` from section 2.3.
So a bind that commits before the delete makes the delete fail.

## 6. Claim

### 6.1. Resource

`DeviceClaim` MUST be a namespaced CRD in the group `solas.dev`, version
`v1alpha1`.
Each member cluster MUST store its claims in its own etcd.
`DeviceClaim.spec.selector` MUST be a label selector over `Device`
labels.
`DeviceClaim.status.phase` MUST be one of `Pending`, `Bound`,
`Suspended`, or `Lost`.
`DeviceClaim.status.deviceName` names the bound device.
`DeviceClaim.status.memberUID` records the member UID that bound the
claim.

### 6.2. Controller

Each member cluster MUST run at most one active claim controller.
The controller MUST use a leader election lease in its own cluster.
The controller MUST NOT bind while its member is not live by its own
clock, as section 7.4 defines.
The controller MUST NOT bind while its member is draining.

### 6.3. Bind

The controller MUST NOT bind a claim that has a deletion timestamp.
Before it binds a claim, the controller MUST add the finalizer
`solas.dev/release` to the claim.
Before it picks a device, the controller MUST do a consistent list of
devices.
If a device in that list has a `claimRef` with the claim UID and the UID
of the member's current `Member`, the controller MUST adopt that device
and MUST NOT bind another.
This covers a crash after the device write and before the claim write.
The controller MUST NOT adopt a device whose `claimRef` has an older
member UID.
A bind from an old member identity can land after a rejoin.
The sweeper clears that `claimRef`, as section 8.3 describes.

The controller MUST pick a device from the free devices that match the
selector.
The controller SHOULD pick at random among the matching devices.
Random choice lowers the chance that two clusters pick the same device.

The bind MUST be a status update of the device that sets `claimRef`.
The bind MUST carry the resource version from the consistent list.
The `claimRef` MUST hold the UID of the member's current `Member`.
If the bind fails with `409 Conflict`, the controller MUST list again
and MUST NOT retry with the old version.

After a successful bind, the controller MUST set the claim phase to
`Bound`.
It MUST also set `status.deviceName` and `status.memberUID`.
The controller MUST NOT set a claim to `Bound` from a bind that carried
an older member UID.

If more than one device has a `claimRef` with the same claim UID, the
controller MUST keep the device in `status.deviceName`.
It MUST release the other devices as section 6.4 describes.
The controller MUST NOT move a claim to another device while the claim
is `Bound`, because a workload can be using the device.
Two devices can name one claim when an old leader's write lands after a
new leader's list.

### 6.4. Release

When a claim has a deletion timestamp, the controller MUST release its
device.
To release, the controller MUST do a consistent list of devices.
For each device whose `claimRef` has the claim UID, the controller MUST
clear `claimRef` with a status update.
That update MUST carry the resource version from the list.
If the update fails with `409 Conflict`, the controller MUST list again.
The controller MUST NOT change a device whose `claimRef` does not have
the claim UID.
After the release, the controller MUST remove the finalizer.

A bind can land after the release has removed the finalizer.
The controller MUST clear a `claimRef` that names its own member and a
claim UID that does not exist in its cluster.
The clear MUST be a status update with the resource version that the
controller read.

### 6.5. Phases

A new claim MUST start in phase `Pending`.
When the controller's member is not live by its own clock, the
controller MUST set each `Bound` claim to `Suspended`.
When the member renews its lease, the controller MUST set each
`Suspended` claim back to `Bound` if its device still names it.
When the controller finds that its member UID changed, it MUST set each
claim with the old `status.memberUID` to `Lost`.
A `Lost` claim MUST NOT bind again.
A user deletes a `Lost` claim to free its finalizer.

A claim is in effect while it is `Bound`, it has no deletion timestamp,
and its member is live by the member's own clock.
Workloads MUST use a device only while its claim is in effect.
The safety properties in section 8.4 are about claims in effect.

The controller can set a claim to `Suspended` only some time after its
lease ends.
So the phase alone does not tell a workload that the claim is in effect.
How a workload learns the end of the lease is an open question.

## 7. Membership

### 7.1. Resource

`Member` MUST be a cluster-scoped resource in the group `solas.dev`,
version `v1alpha1`.
The solas API server MUST serve `Member` from the table.
The name of a `Member` MUST be the cluster ID of its member cluster.
`Member.spec.leaseDurationSeconds` MUST hold the lease duration `D`.
`Member.status.renewTime` holds the time of the last renew, as the
holder's clock gives it.
`Member.status.phase` MUST be `Active` or `Draining`.
A member is live while its `Member` exists.

### 7.2. Join

A member cluster MUST create its `Member` before it binds any device.
The server gives each new `Member` a new UID.
The member MUST use the UID of its current `Member` in each `claimRef`
that it writes.

When the controller starts, it MUST try to renew its existing `Member`.
If the renew succeeds, the controller MUST keep the UID of that `Member`.
If the `Member` does not exist, the controller MUST create a new one.
If the new UID differs from the UID in a claim's `status.memberUID`, the
claim is `Lost`, as section 6.5 describes.

### 7.3. Renew

A renew MUST be an update of `Member.status.renewTime`.
The renew MUST carry the resource version that the member last read or
wrote.
A renew that fails with `404 NotFound` means that a sweeper deleted the
`Member`.
After a `404 NotFound`, the member MUST join again as section 7.2
describes.
A member SHOULD renew every `D / 3`.
The default `D` is 30 seconds.

### 7.4. Lease on the holder

Let `S` be the local time at which the member sent its last successful
renew.
Let `M` be the safety margin.
The member MUST treat itself as not live from local time `S + D - M`.
The member MUST treat itself as live again only after a later renew
succeeds.

### 7.5. Clock drift

Let `rho` be the bound on the rate drift between any two clocks.
The design assumes that `rho` is at most 0.01.
The margin `M` MUST be at least `2 * rho * D`.
The default `M` is `D / 10`.
The default leaves room for scheduling delays on top of drift.
Solas does not depend on synchronized clocks.
Solas depends only on the drift bound.

### 7.6. Leave

To leave, the member MUST first set its phase to `Draining`.
A draining member MUST NOT bind.
The member MUST then set each `Bound` claim to `Suspended`.
The member MUST then release each device that it holds, as section 6.4
describes.
The member MUST NOT release the device of a claim that is still `Bound`.
The member MUST then delete its `Member`.

## 8. Reclaim

### 8.1. Expiry on the observer

Each member cluster MUST run a sweeper.
An observer MUST measure the lease of another member with its own clock.
The observer MUST record the local time at which it first saw the
current resource version of each `Member`.
Let `T` be that local time.
The observer MUST treat the `Member` as expired from local time `T + D`.
The observer MUST NOT compare `renewTime` with its own clock.
`renewTime` comes from another clock, and the two clocks can disagree by
any amount.

### 8.2. Delete an expired member

The sweeper MUST delete an expired `Member` with the resource version
that it observed.
If the member renewed after the observation, the delete fails with
`409 Conflict`.
So a renew and a delete of the same `Member` cannot both succeed.
The sweeper MUST NOT delete its own `Member`.

### 8.3. Clear stale claim references

The sweeper MUST do a consistent list of devices first.
The sweeper MUST then do a consistent list of members.
The order matters.
A member creates its `Member` before it writes any `claimRef`.
So a `claimRef` in the device list names a member that existed before
the member list.
If that member is not in the member list, it is gone.

For each device whose `claimRef` names a member UID that is not in the
member list, the sweeper MUST clear `claimRef`.
The clear MUST be a status update with the resource version from the
device list.
If the clear fails with `409 Conflict`, the sweeper MUST skip the device
until its next run.
The sweeper MUST run at least once every `D`.

### 8.4. Safety

The rules in sections 7.4, 7.5, 8.1, and 8.2 give this result.
A sweeper deletes a `Member` only after the holder treats itself as not
live.
The observer sees a renew at or after the time the holder sent it.
The observer waits `D` on its own clock from then.
The holder waits at most `D - M` on its own clock.
With `M` at least `2 * rho * D`, the holder stops first.

The Quint model checks these properties:

- No two claims are bound to the same device.
- If a claim is `Bound` to a device, the device's `claimRef` names the
  claim, or the claim's member is not live.
- A sweeper does not clear a `claimRef` while the holder is live by its
  own clock.
- The resource version of each resource strictly increases.
- A watch from `r` sees every event after `r` in order, or it gets
  `410 Gone`.
- A device whose member leaves or crashes becomes free.
- A claim for a free, matching device becomes `Bound` while its member
  is live.
