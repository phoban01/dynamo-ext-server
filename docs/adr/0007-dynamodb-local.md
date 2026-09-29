# ADR 0007: Use dynamodb-local, not LocalStack

Status: accepted

## Context

The tests and the demo need a local DynamoDB. `CLAUDE.md` allows
LocalStack. The current LocalStack image (2026.8.4) exits at start with
"License activation failed" unless `LOCALSTACK_AUTH_TOKEN` is set. A
token needs a LocalStack account.

`amazon/dynamodb-local` is the local DynamoDB from AWS. It needs no
account. We checked that it creates tables, accepts the TTL setting,
runs `TransactWriteItems` with conditions, and does not apply a second
call with the same `ClientRequestToken`. It uses about 230 MB of memory.

## Decision

The unit tests, the conformance tests, and the demo use
`amazon/dynamodb-local` in memory mode.

## Consequences

- Contributors and CI need no account or token.
- dynamodb-local does not delete items when their TTL passes. Spec 4.1
  already says the server must not depend on when DynamoDB deletes.
  Tests that need a missing event delete the item directly.
- dynamodb-local can differ from DynamoDB in some behavior. The plan
  already lists a run against real DynamoDB as a risk to close.

## Rejected alternatives

- LocalStack with an auth token. Every contributor and the CI would need
  an account.
- An old LocalStack image that runs without a token. It gets no fixes.
