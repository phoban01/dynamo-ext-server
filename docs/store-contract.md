# The store contract

Spec 2.5 lists the rules that the protocol needs from a store. The Quint
model of the protocol, `quint/solas.qnt`, assumes only these rules. This
page shows how each store keeps each rule.

The DynamoDB store is our code, so we check it: with the Quint model
`quint/store.qnt`, and with the upstream storage tests of
`k8s.io/apiserver` in `pkg/storage/dynamo/conformance_test.go`. The etcd
store is the etcd3 storage of `k8s.io/apiserver`, which the
kube-apiserver uses. We rely on etcd and on that code, and the REST
store tests and model-based tests run on both stores.

| Rule of spec 2.5 | DynamoDB store | etcd store |
|------------------|----------------|------------|
| A conditional write applies only when the object still has the resource version that the writer read. | The object action of each transaction has the condition `rv = :rv`, spec 2.3. Conformance: `RunTestGuaranteedUpdateWithConflict`, `RunTestDeleteWithConflict`. | The etcd3 store commits in a transaction that compares the mod revision of the key. |
| A write applies in full or not at all. | Each write is one `TransactWriteItems` call, spec 2.3, which DynamoDB applies in full or not at all. `quint/store.qnt` models the write as one step. | An etcd transaction is atomic. |
| A create fails when an object with the same key exists. | The put has the condition `attribute_not_exists(pk)`, spec 2.3. Conformance: `RunTestCreateWithKeyExist`. | The etcd3 store creates in a transaction that requires create revision 0. |
| A read sees every write that completed before the read started. | Every read is strongly consistent, spec 2.4. | etcd reads are linearizable by default, spec 2.7. |
| A list is a consistent snapshot at the resource version that it returns. | The list reads the counter before and after the query, spec 2.4. `quint/store.qnt`: invariant `listSnapshot`; the negative model `quint/negative/list-no-recheck.qnt` shows that the second read is needed. | An etcd range read returns the state at one revision. |
| The resource version of each write to a resource is greater than that of every earlier write. | The counter grows by 1 in the same transaction as the write, spec 3.1. `quint/store.qnt`: invariant `monotonicRV`. | etcd gives each write a revision, and revisions grow. |
| A watch from a resource version delivers every later change, in order, or fails with `410 Gone`. | Polling of the event log, spec 4.2 and 4.3. `quint/store.qnt`: invariants `monotonicRV` and `watchComplete`; the negative model `quint/negative/watch-no-gap-check.qnt`. Conformance: `RunTestWatch`, `RunWatchSemantics`. | etcd watches deliver every event from a revision in order. A watch from a compacted revision fails with `410 Gone`. |
