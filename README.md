# Glypha

> "Stand out of my sun."\
> — Diogenes

A small digital signage system built around retaining the last successful scene.
The server is written in Go; the browser renderer uses PixiJS 8 and TypeScript.

## Current implementation

The first end-to-end content path is implemented:

- Strict multipart/JSON validation and PNG/JPEG asset validation.
- Noto Sans JP Regular (400) glyph checks and scene AST compilation.
- Atomic SQLite publication and durable daily scheduling with clock-rollback protection.
- Self-contained display envelopes, SHA-256 asset hashes, and conditional ETags.
- PixiJS candidate preparation, complete canvas replacement, and retention of the last successful scene on failure.
- Docker production and development environments, with separate persistent content volumes.

`GET /display` returns `204` until the first accepted package. Upload the daily example:

```sh
curl --fail-with-body -X PUT http://localhost:8080/content \
  -F 'content=@examples/daily/content.json;type=application/json'
```

The scene is selected using the package's fixed UTC offset and daily times. Repeat `-F 'assets=@path/to/image.png'` for referenced images. An upload replaces the complete current package; invalid uploads retain the previous package. See the [protocol](design/http-and-rendering.md).

## Run the built application

Prerequisite: Docker Engine/Desktop with Compose v2, running Linux containers.
Host Go and Node installations are not needed.

```sh
docker compose up --build -d
```

Open [http://localhost:8080](http://localhost:8080). The Go server serves both the built renderer and the API.
The browser performs graphics rendering; the container does not run a graphical desktop or need direct GPU access.
A modern browser with WebGL/WebGPU support is expected. An HTML version of Builtin remains visible if graphics initialization fails.

```sh
curl -i http://localhost:8080/healthz
curl -i http://localhost:8080/display
docker compose ps
docker compose logs -f
```

Health returns `200` with `{"status":"ok"}` when storage is available. The runtime runs as a non-root user with a read-only root filesystem and a writable `/data` volume containing SQLite state. A commit failure stops state serving and requires a server restart.

Stop it before starting development, since both modes use host port 8080:

```sh
docker compose down
```

## Develop in Docker

```sh
docker compose -p glypha-dev -f compose.dev.yml up --build -d
```

Open [http://localhost:5173](http://localhost:5173). Vite forwards `/display`, `/content`, and `/healthz` to the Go container.
Browser requests remain same-origin; no permissive CORS configuration is required.

Renderer source edits reload through Vite. Go source is bind-mounted; restart the Go service after editing it:

```sh
docker compose -p glypha-dev -f compose.dev.yml restart server
```

The renderer's Linux dependencies live in a named volume so they do not conflict with host dependencies.
Startup runs `npm ci` to synchronize that volume with the lockfile. After changing dependencies, update `renderer/package-lock.json` and restart the renderer.

```sh
docker compose -p glypha-dev -f compose.dev.yml down
```

Volumes retain development caches, dependencies, and server content. Stop commands do not delete them. Production and development content volumes are separate.
Compose publishes ports on loopback only. Container processes bind internally on all interfaces so port forwarding works.

## Checks

Go compiler, content, schedule, persistence, and handler tests run during the application image build; the renderer build runs TypeScript checking.
For explicit checks without installing host runtimes:

```sh
docker compose -p glypha-dev -f compose.dev.yml run --rm --no-deps server go test ./...
docker compose -p glypha-dev -f compose.dev.yml run --rm --no-deps renderer sh -c 'npm ci && npm run build'
```

After changing Dockerfiles or dependency manifests, rebuild the relevant images before running checks.
The scheduler tests compare catch-up lookup against independent occurrence enumeration, including negative Unix seconds, boundary times, and clock rollback. Content tests include malformed JSON, missing references, PNG/JPEG decoding, animation rejection, and resource bounds.
Persistence tests kill child processes at 12 publication/consumption boundaries, then reopen SQLite and verify complete state and image snapshots. Commit-error tests cover both publication and scheduling updates. These use temporary databases and do not touch the running service's content.
HTTP integration tests pause response writes and request reads at deterministic boundaries. They verify admission limits, publication during slow transfers, coherent image snapshots across replacement, interrupted-upload recovery, and durable scheduling before a conditional `304` response. Run concurrency checks with:

```sh
docker compose -p glypha-dev -f compose.dev.yml exec -T server go test -race ./...
```


Browser failure checks run against the actual PixiJS renderer in the development environment. Open [renderer checks](http://localhost:5173/tests/) and select **Run checks**. All 25 checks should pass. Responses are isolated fixtures; the checks do not upload or modify server content. The final test frame remains visible for inspection. This page is not included in the production build.

The checks cover failed responses, malformed payloads and images, font/layout/graphics failures, failed swaps, cancellation, serial polling, ETag ownership, retry backoff, recovery, and repeated resource disposal. TypeScript checking includes the test sources, but `npm run build` does not execute browser checks.

For a host workflow, use Go 1.26 or newer and Node 24:

```sh
go test ./...
cd renderer
npm ci
npm run build
```

Then run `go run ./cmd/glypha` from the repository root. The server defaults to `127.0.0.1:8080` and `renderer/dist`.
For a host Vite session, run `npm run dev` in `renderer/` alongside the server.

## Configuration

| Variable | Host default | Container value |
|---|---|---|
| `GLYPHA_ADDR` | `127.0.0.1:8080` | `:8080` |
| `GLYPHA_WEB_DIR` | `renderer/dist` | `/web` in the runtime image |
| `GLYPHA_DB_PATH` | `data/glypha.db` | `/data/glypha.db` |
| `GLYPHA_FONT_PATH` | `renderer/public/fonts/noto-sans-jp/NotoSansJP.ttf` | `/web/fonts/noto-sans-jp/NotoSansJP.ttf` in runtime |
| `GLYPHA_API_URL` | `http://127.0.0.1:8080` | `http://server:8080` in Vite development |

The image health-check command targets internal port 8080. If overriding the internal port, update its health check too.
`GLYPHA_API_URL` is a Vite proxy setting, never a URL embedded in browser code.

Dependencies are pinned in `renderer/package.json` and its lockfile. The Docker build uses Go 1.27.1 and the Node 24 Debian image family; base-image updates remain a deliberate rebuild concern.
Builtin uses the locally bundled Noto Sans JP at Regular (400). PixiJS waits for the face to load before measuring or rendering text; the HTML startup scene remains available if font loading fails. Font bytes, version, selected variation, checksum, and license are recorded in [font provenance](renderer/public/fonts/noto-sans-jp/PROVENANCE.txt). The compiler verifies the font checksum, glyph coverage, and shaped ink bounds. The browser checks its actual ink bounds before display; browser rasterization differences can still reject a candidate.
Errors are logged to the server or browser console and never replace the active scene.

## Operations and completion

See [operations](ops/README.md) for startup, updates, health recovery, offline backup, and restoration. [Initial implementation status](RELEASE_STATUS.md) separates completed software checks from the deferred appliance trial. CI adoption is deferred; no GitHub Actions workflow is enabled. Run the documented Docker, browser, and Alloy checks locally.

## Design and models

- [Concept](Concept.md)
- [Development method](DevelopmentMethod.md)
- [Accepted behavioral decisions](DesignDecisions.md)
- [Implementation design](design/README.md)
- [Alloy models and verification](model/README.md)

The backend is now selected as browser-based PixiJS. Noto Sans JP Regular (400) is the selected font. Target-device memory/performance trials and broader browser fault-injection coverage remain follow-up work.

## License

Glypha is licensed under the [MIT License](LICENSE).
The bundled Noto Sans JP font remains under the [SIL Open Font License 1.1](renderer/public/fonts/noto-sans-jp/OFL.txt). Third-party dependencies retain their respective licenses.
