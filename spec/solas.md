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
An object item MUST have `sk` equal to the namespace, a `/`, and the
object name.
For a cluster-scoped object, the namespace part of `sk` is empty.
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
The event type MUST be one of `ADDED`, `MODIFIED`, or `DELETED`.
An event item MUST hold the encoded new object in `value`.
An event item for a `MODIFIED` or `DELETED` event MUST hold the encoded
previous object in `prev`.
An event item MUST hold its expiry time, in Unix seconds, in `expires`.

### 2.3. Writes

Every write MUST be one `TransactWriteItems` call.
The call MUST hold three actions: a counter update, an object action, and
an event put.

The counter update MUST set `n` to `n + 1` on condition that `n` still
has the value that the server read.
If the counter item does not exist, the counter update MUST create it
with `n` equal to 1, on condition that it still does not exist.

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

## 4. Watch

## 5. Device

## 6. Claim

## 7. Membership

## 8. Reclaim
