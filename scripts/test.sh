#!/usr/bin/env bash
# Runs the unit tests with the race detector. When Docker is present, the
# script starts dynamodb-local for the storage tests and removes it on exit.
set -euo pipefail

image=amazon/dynamodb-local:3.3.1

if [ -z "$(go list ./... 2>/dev/null)" ]; then
  echo "test: no Go packages yet"
  exit 0
fi

if [ -z "${SOLAS_DYNAMODB_ENDPOINT:-}" ] && command -v docker >/dev/null \
  && docker info >/dev/null 2>&1; then
  name="solas-ddb-test-$$"
  docker run -d --rm --name "$name" -p 127.0.0.1::8000 "$image" \
    -jar DynamoDBLocal.jar -inMemory >/dev/null
  trap 'docker rm -f "$name" >/dev/null 2>&1 || true' EXIT
  port=$(docker port "$name" 8000/tcp | head -1 | sed 's/.*://')
  for _ in $(seq 50); do
    if (echo >"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
      break
    fi
    sleep 0.2
  done
  export SOLAS_DYNAMODB_ENDPOINT="http://127.0.0.1:$port"
  echo "test: dynamodb-local at $SOLAS_DYNAMODB_ENDPOINT"
else
  echo "test: no Docker; storage tests that need DynamoDB are skipped"
fi

go test -race "$@" ./...
