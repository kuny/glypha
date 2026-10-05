#!/bin/sh
# Exercise the runtime image, offline backup, and restore using disposable resources.
set -eu
runtime_image=${1:-glypha-ci-runtime}
helper_image=${2:-glypha-ci-server}
smoke_dir=$(mktemp -d)
original_volume=
restored_volume=
original_container=
restored_container=
cleanup() {
  for container in "$restored_container" "$original_container"; do
    if [ -n "$container" ]; then docker rm -f "$container" >/dev/null 2>&1 || true; fi
  done
  for volume in "$restored_volume" "$original_volume"; do
    if [ -n "$volume" ]; then docker volume rm "$volume" >/dev/null 2>&1 || true; fi
  done
  rm -rf "$smoke_dir"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
original_volume=$(docker volume create)
restored_volume=$(docker volume create)
start_runtime() {
  docker run -d --read-only --tmpfs /tmp -p 127.0.0.1::8080 \
    --mount "type=volume,source=$1,target=/data" "$runtime_image"
}
wait_healthy() {
  attempts=0
  until docker exec "$1" /glypha healthcheck; do
    attempts=$((attempts + 1))
    if [ "$attempts" -ge 20 ]; then docker logs "$1"; return 1; fi
    sleep 1
  done
}
original_container=$(start_runtime "$original_volume")
wait_healthy "$original_container"
address="http://$(docker port "$original_container" 8080/tcp)"
code=$(curl --silent --show-error -o "$smoke_dir/empty" -w '%{http_code}' "$address/display")
[ "$code" = 204 ]
cat > "$smoke_dir/content.json" <<'JSON'
{"format":1,"canvas":{"width":1920,"height":1080},"schedule":{"utcOffset":"+00:00","defaultScene":"sample","daily":[]},"scenes":{"sample":{"background":{"color":"#182028"},"elements":[{"type":"text","x":100,"y":100,"width":1700,"height":160,"text":"Glypha smoke check","fontSize":64,"color":"#FFFFFF","align":"left"}]}}}
JSON
curl --fail --silent --show-error -X PUT -F "content=@$smoke_dir/content.json" "$address/content" > "$smoke_dir/publication"
curl --fail --silent --show-error "$address/display" > "$smoke_dir/before.json"
docker restart "$original_container" >/dev/null
wait_healthy "$original_container"
# Docker may allocate a different dynamic host port after restart.
address="http://$(docker port "$original_container" 8080/tcp)"
curl --fail --silent --show-error "$address/display" > "$smoke_dir/restarted.json"
cmp "$smoke_dir/before.json" "$smoke_dir/restarted.json"
docker stop "$original_container" >/dev/null
mkdir "$smoke_dir/backup"
docker cp "$original_container:/data/." "$smoke_dir/backup/"
docker run --rm --mount "type=bind,source=$smoke_dir/backup,target=/backup,readonly" \
  --mount "type=volume,source=$restored_volume,target=/data" "$helper_image" \
  sh -c 'test -z "$(ls -A /data)" && cp -a /backup/. /data/ && chown -R 65532:65532 /data'
restored_container=$(start_runtime "$restored_volume")
wait_healthy "$restored_container"
address="http://$(docker port "$restored_container" 8080/tcp)"
curl --fail --silent --show-error "$address/display" > "$smoke_dir/restored.json"
cmp "$smoke_dir/before.json" "$smoke_dir/restored.json"
code=$(curl --silent --show-error -o "$smoke_dir/conditional" -w '%{http_code}' -H 'If-None-Match: *' "$address/display")
[ "$code" = 304 ]
printf '%s\n' 'PASS: runtime upload, restart, offline backup, and restore preserve the complete snapshot.'
