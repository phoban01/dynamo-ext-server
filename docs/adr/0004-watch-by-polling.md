# ADR 0004: Watch polls a TTL event log

Status: accepted

## Context

Kubernetes clients and informers depend on watch. The store must deliver
every change after a given resource version, in order. When it cannot,
it must return `410 Gone`, so that the client lists again.

## Decision

Each write appends an event item to the resource's event log in the same
transaction. Event items expire through DynamoDB TTL. A watcher reads the
counter, then queries the log for the events after its last version, up
to the counter value. A gap in the result means expired events, and the
watch ends with `410 Gone`.

## Consequences

- Watch reuses the same strongly consistent reads as the rest of the
  store. It needs no other AWS service.
- Polling adds latency, bounded by the poll period.
- Each poll costs a read. The API server watch cache runs one poller per
  resource, not one per client.
- TTL deletes items late and in no set order. The gap check handles this.

## Rejected alternatives

- DynamoDB Streams. Streams give push delivery, but they add shard
  handling, a 24-hour retention, and extra LocalStack behavior to trust.
  We can add them later behind the same interface.
- No event log, with a full list on each poll. This loses the order and
  the delete events that watch must deliver.
