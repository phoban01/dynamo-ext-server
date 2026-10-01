# ADR 0014: Choose the store with a storage URL

Status: accepted

## Context

Solas keeps `Device` and `Member` in one DynamoDB table (ADR 0001).
Some users run their management clusters on premises, with no AWS
account. They already run etcd. The protocol of sections 5 to 10 of the
spec does not need DynamoDB. It needs a store with conditional writes,
consistent lists, and a watch. etcd has all three, and
`k8s.io/apiserver` already has an etcd3 storage that the kube-apiserver
uses.

We also want to show that the store is a setting: the demo runs on
DynamoDB, and after one change, on etcd.

## Decision

One flag of the API server, `--storage-url`, picks the shared store.

- `dynamodb://<table>?region=<region>&endpoint=<url>&create-table=true`
  picks a DynamoDB table. An empty table name picks `solas`. The query
  keys are optional. AWS credentials come from the environment, as today.
- `etcd://<host:port>[,<host:port>...]` picks an etcd cluster. The store
  is the etcd3 storage of `k8s.io/apiserver`, with the watch cache, under
  the storage prefix of the API server.

All member clusters use the same store. The spec states the rules that
the protocol needs from any store, the store contract. Each store must
keep them.

The flags `--dynamodb-table`, `--dynamodb-region`, `--dynamodb-endpoint`,
and `--dynamodb-create-table` go away. The URL carries their values.

This amends ADR 0001: the shared store is one DynamoDB table in one
region, or one etcd cluster.

## Consequences

- An operator changes the store by changing one value. The Deployment
  reads it from the Secret `solas-storage`.
- A change of store does not move data. ADR 0015 covers migration.
- The etcd store relies on upstream code for the store contract. Our
  Quint store model covers only the DynamoDB store.
- Tests and model-based tests run on both stores, so a difference in
  behaviour shows up.

## Out of scope

- TLS and authentication to etcd. The demo uses plain HTTP. A later
  change can add certificate files as query keys.
- Running two stores at once.

## Rejected alternatives

- A flag per store, for example `--etcd-servers` next to the DynamoDB
  flags. Two flags can both be set, and the operator must change more
  than one value.
- Running solas against the kube-apiserver's own etcd of one cluster.
  Each cluster has its own etcd, so the clusters would not share state.
