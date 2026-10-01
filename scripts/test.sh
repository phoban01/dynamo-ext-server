#!/usr/bin/env bash
# Runs the unit tests with the race detector. The storage tests need a
# DynamoDB and an etcd. When Docker is present, the script starts
# dynamodb-local; when etcd is present, it starts an etcd. It removes both
# on exit. SOLAS_DYNAMODB_ENDPOINT and SOLAS_ETCD_ENDPOINT point the tests
# at stores that already run.
set -euo pipefail

image=amazon/dynamodb-local:3.3.1

if [ -z "$(go list ./... 2>/dev/null)" ]; then
  echo "test: no Go packages yet"
  exit 0
fi

cleanup=()
trap 'for c in "${cleanup[@]}"; do eval "$c"; done' EXIT

# wait_port <port> waits until something listens on the port.
wait_port() {
  for _ in $(seq 50); do
    if (echo >"/dev/tcp/127.0.0.1/$1") 2>/dev/null; then
      return 0
    fi
    sleep 0.2
  done
  echo "test: nothing listens on port $1" >&2
  return 1
}

# free_port prints a port that nothing listens on.
free_port() {
  local p
  while :; do
    p=$((20000 + RANDOM % 20000))
    if ! (echo >"/dev/tcp/127.0.0.1/$p") 2>/dev/null; then
      echo "$p"
      return
    fi
  done
}

if [ -z "${SOLAS_DYNAMODB_ENDPOINT:-}" ] && command -v docker >/dev/null \
  && docker info >/dev/null 2>&1; then
  name="solas-ddb-test-$$"
  docker run -d --rm --name "$name" -p 127.0.0.1::8000 "$image" \
    -jar DynamoDBLocal.jar -inMemory >/dev/null
  cleanup+=("docker rm -f $name >/dev/null 2>&1 || true")
  port=$(docker port "$name" 8000/tcp | head -1 | sed 's/.*://')
  wait_port "$port"
  export SOLAS_DYNAMODB_ENDPOINT="http://127.0.0.1:$port"
  echo "test: dynamodb-local at $SOLAS_DYNAMODB_ENDPOINT"
elif [ -z "${SOLAS_DYNAMODB_ENDPOINT:-}" ]; then
  echo "test: no Docker; storage tests that need DynamoDB are skipped"
fi

if [ -z "${SOLAS_ETCD_ENDPOINT:-}" ] && command -v etcd >/dev/null; then
  dir=$(mktemp -d)
  client=$(free_port)
  peer=$(free_port)
  etcd --data-dir "$dir" --log-level error --unsafe-no-fsync \
    --listen-client-urls "http://127.0.0.1:$client" --advertise-client-urls "http://127.0.0.1:$client" \
    --listen-peer-urls "http://127.0.0.1:$peer" --initial-advertise-peer-urls "http://127.0.0.1:$peer" \
    --initial-cluster "default=http://127.0.0.1:$peer" >"$dir/log" 2>&1 &
  cleanup+=("kill $! 2>/dev/null || true; wait $! 2>/dev/null || true; rm -rf $dir")
  wait_port "$client"
  export SOLAS_ETCD_ENDPOINT="http://127.0.0.1:$client"
  echo "test: etcd at $SOLAS_ETCD_ENDPOINT"
elif [ -z "${SOLAS_ETCD_ENDPOINT:-}" ]; then
  echo "test: no etcd; storage tests that need etcd are skipped"
fi

go test -race "$@" ${TEST_PKGS:-./...}
