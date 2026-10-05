# Operations

This runbook covers one local Go server, one SQLite volume, and a browser display. Run commands from the repository root. Docker Engine/Desktop with Compose v2 is required; the browser runs on the viewing device.

## Start and stop

Stop development before starting production because both use port 8080:

```sh
docker compose -p glypha-dev -f compose.dev.yml down
docker compose up --build -d --wait
curl --fail http://localhost:8080/healthz
```

Open `http://localhost:8080` in the display browser. Upload a package using the README's `curl` example. Before the first upload, `GET /display` returns `204` and the browser shows Builtin.

Production uses `restart: unless-stopped`: Docker restarts an exited container and starts it again after the engine restarts unless it was explicitly stopped. Docker Desktop/Engine itself must start on the host. Browser launch, kiosk mode, host sleep settings, and physical display configuration remain device-specific work deferred with the appliance trial. An unhealthy but still-running container is not automatically restarted by this policy.

```sh
docker compose ps
docker compose logs --tail=100 glypha
docker compose stop glypha
docker compose start --wait glypha
```

`docker compose down` removes containers but preserves the content volume. Do not add `-v` when preserving content. Keep the Compose project name and repository location stable, or consistently supply the same `-p` project name, so a different project does not appear to have an empty database. Never run two servers against the same SQLite volume.

## Health and recovery

| Observation | Action |
|---|---|
| `GET /healthz` returns `200` | Storage service is available; this does not prove a browser is displaying content. |
| Upload returns `400`, `413`, `415`, or `422` | Correct the submitted package; the previous package remains current. |
| `503` with a busy error | Retry after the supplied `Retry-After`; admission is deliberately bounded. |
| Storage error / unhealthy service | Inspect logs and disk space. Resolve the cause, then `docker compose restart glypha`. |
| Unsupported schema, font mismatch, or corrupt stored content on startup | Preserve the volume and logs. Use a compatible build or restore a known backup into a separate volume. Do not delete state to make startup succeed. |
| Browser console reports a retrieval or preparation failure | The last successful frame remains. Verify the package and server, then allow polling to retry. |

A failed commit disables state serving until restart. A lost upload response can still mean the upload committed; inspect the current display or retry knowing that retry creates a new publication. Server restart preserves the consumption cursor. Browser restart shows Builtin first and retrieves the retained target again.

To inspect Docker capacity:

```sh
docker system df
docker builder prune
```

The second command prompts before deleting rebuildable build cache and can affect other projects' caches. It does not remove content volumes. Volume pruning is not a disk-maintenance step for this application.

## Offline backup

Stop the server before copying SQLite. Copy the entire `/data` directory, including a rollback journal if present. Copying only a live `glypha.db` is not a supported backup procedure. Browser windows can keep displaying their retained frame during this maintenance window.

Choose a new backup directory, record the application revision, and copy from the stopped container:

```sh
backup_dir="backups/$(date -u +%Y%m%dT%H%M%SZ)"
mkdir -p "$backup_dir"
git rev-parse HEAD > "$backup_dir/revision.txt"
docker compose stop glypha
mkdir "$backup_dir/data"
docker compose cp glypha:/data/. "$backup_dir/data/"
docker compose start --wait glypha
curl --fail http://localhost:8080/healthz
```

Run the restart command even if copying fails, once the original service is safe to resume. Store another copy of the backup outside the Docker VM and preferably outside this host. `backups/` is excluded from Git and Docker build contexts. A backup is only established after the copy succeeds; validate restoration periodically using a separate volume.

## Restore into a new volume

Use the revision associated with the backup. The maintenance image supplies filesystem tools; the production image intentionally has no shell. Build it before stopping the application:

```sh
docker build --target server-dev -t glypha-maintenance .
```

Set an absolute path to the backup's `data` directory and choose a new, unused volume name:

```sh
backup_data="$PWD/backups/REPLACE_WITH_BACKUP_DIRECTORY/data"
export GLYPHA_RESTORE_VOLUME=glypha-restored-REPLACE_WITH_UNIQUE_NAME
docker volume create "$GLYPHA_RESTORE_VOLUME"
docker run --rm \
  --mount "type=bind,source=$backup_data,target=/backup,readonly" \
  --mount "type=volume,source=$GLYPHA_RESTORE_VOLUME,target=/data" \
  glypha-maintenance \
  sh -c 'test -z "$(ls -A /data)" && cp -a /backup/. /data/ && chown -R 65532:65532 /data'
```

The copy refuses a nonempty destination. Do not continue if it fails. Keep the original volume for recovery. Once the restore copy succeeds:

```sh
docker compose stop glypha
docker compose -f compose.yml -f ops/compose.restore.yml up -d --wait
curl --fail http://localhost:8080/healthz
curl --fail http://localhost:8080/display
```

The override replaces only the `/data` mount with the restored external volume. Inspect the displayed scene and logs. Keep `GLYPHA_RESTORE_VOLUME` in the local `.env` file and include both Compose files in all subsequent operations, including backup, restart, and updates. Commands above that use only `compose.yml` describe the default-volume deployment; after adopting a restored volume, apply the override consistently. Omitting it returns to the original volume.

An older backup also restores its older consumption cursor. Therefore, disaster recovery from an older backup can revisit transitions already consumed after that backup. The no-replay guarantee across normal restarts does not reconstruct state absent from a backup. Current wall-clock time determines catch-up after restoration.

## Update

1. Record the running revision and make an offline backup.
2. Review the new revision's schema/profile compatibility. There is currently no general database migration facility.
3. Update the checkout to the chosen revision and build before replacing the running container.
4. Recreate the service, check health/logs, and inspect the browser.

```sh
docker compose build
docker compose up -d --wait
curl --fail http://localhost:8080/healthz
```

For a restored-volume deployment, add `-f compose.yml -f ops/compose.restore.yml` to both commands. If a new version cannot load existing state, retain the volume and use the compatible prior revision. If restoration is necessary, use a separate volume as above. Do not assume an older binary understands a newer database format.

## Reproduce runtime checks

```sh
docker build --target server-dev -t glypha-ci-server .
docker build --target runtime -t glypha-ci-runtime .
sh scripts/smoke.sh glypha-ci-runtime glypha-ci-server
```

The script creates disposable containers and volumes on dynamically assigned loopback ports. It checks empty startup, upload, restart, offline copy from a stopped container, restoration with the production UID, and identical restored response bytes. It removes only its own temporary resources. It requires host `curl` and standard shell tools; it does not touch the running application's volume.

The listener remains loopback-only by default. Deployment beyond this local boundary requires a separate access-control/TLS policy; this runbook does not expose the unauthenticated upload API publicly.
