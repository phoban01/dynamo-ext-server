#!/usr/bin/env bash
# The shared part of the demo: one Docker network and one dynamodb-local.
#   mesh.sh up        create the network and the table container
#   mesh.sh down      remove them (delete the clusters first)
#   mesh.sh endpoint  print the table endpoint
# The endpoint is an IP address, because pods cannot resolve Docker
# container names.
set -euo pipefail

network=solas-mesh
ddb=solas-ddb
image=amazon/dynamodb-local:3.3.1

endpoint() {
  local ip
  ip=$(docker inspect -f "{{(index .NetworkSettings.Networks \"$network\").IPAddress}}" "$ddb")
  echo "http://$ip:8000"
}

case ${1:-} in
up)
  docker network inspect "$network" >/dev/null 2>&1 || docker network create "$network" >/dev/null
  if ! docker inspect "$ddb" >/dev/null 2>&1; then
    docker run -d --name "$ddb" --network "$network" "$image" -jar DynamoDBLocal.jar -inMemory >/dev/null
  fi
  endpoint
  ;;
down)
  docker rm -f "$ddb" >/dev/null 2>&1 || true
  docker network rm "$network" >/dev/null 2>&1 || true
  ;;
endpoint)
  endpoint
  ;;
*)
  echo "usage: mesh.sh up|down|endpoint" >&2
  exit 2
  ;;
esac
