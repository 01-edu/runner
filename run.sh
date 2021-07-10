#!/usr/bin/env bash

set -euo pipefail
IFS='
'
cd -P "$(dirname "$0")"

docker build -t test-runner .
docker container rm --force test-runner 2>/dev/null
docker run \
    --detach \
    --name test-runner \
    --network https \
    --env REGISTRY_PASSWORD \
    --restart unless-stopped \
    --volume /var/run/docker.sock:/var/run/docker.sock:ro \
    --label org.01-edu.https=test-runner.01-edu.org:8080 \
    test-runner
