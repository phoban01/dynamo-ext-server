#!/usr/bin/env bash
# Builds the solas, solas-demo, and solas-pivot images for linux/amd64 and
# linux/arm64, and pushes them tagged <version> and latest.
#   REGISTRY  where to push (default ghcr.io/phoban01)
#   PUSH=0    build only, for a local check
set -euo pipefail
cd "$(dirname "${BASH_SOURCE[0]}")/.."
version=${1:?usage: push-images.sh <version>}
registry=${REGISTRY:-ghcr.io/phoban01}
output=--push
[ "${PUSH:-1}" = 0 ] && output=--output=type=cacheonly

# The default docker driver cannot build for two platforms at once.
docker buildx inspect solas-release >/dev/null 2>&1 ||
  docker buildx create --name solas-release --driver docker-container >/dev/null

for target in solas demo pivot; do
  name=solas
  [ "$target" = solas ] || name=solas-$target
  docker buildx build --builder solas-release --platform linux/amd64,linux/arm64 \
    --target "$target" \
    --label org.opencontainers.image.source=https://github.com/phoban01/solas \
    --label org.opencontainers.image.version="$version" \
    -t "$registry/$name:$version" -t "$registry/$name:latest" "$output" .
  echo "push-images: $registry/$name:$version"
done
