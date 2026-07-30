#!/usr/bin/env zsh
set -euo pipefail

image=${1:-modary-f0:amd64}
platform=${2:-linux/amd64}
arch=${platform##*/}
binary="$PWD/dist/modary-integration-linux-$arch"

GOOS=linux GOARCH="$arch" CGO_ENABLED=0 go test -c \
  -o "$binary" ./tests/integration

docker run --rm --platform "$platform" \
  --cpus 2 --memory 2g --pids-limit 128 \
  --tmpfs /tmp:rw,nosuid,nodev,size=256m \
  --env GOMAXPROCS=2 \
  --volume "$binary:/modary-integration:ro" \
  --entrypoint /modary-integration \
  "$image" \
  -test.run '^TestPreviewPerformance1000Rows$' -test.count=1 -test.v
