#!/usr/bin/env bash

set -euo pipefail
IFS='
'
cd -P "$(dirname "$0")"

docker build -t runner .
docker container rm --force runner 2>/dev/null
docker run \
    --detach \
    --name runner \
    --log-opt max-size=100m \
    --log-opt max-file=2 \
    --env REGISTRY_PASSWORD \
    --restart unless-stopped \
    --volume /var/run/docker.sock:/var/run/docker.sock:ro \
    runner
