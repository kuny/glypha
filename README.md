# Glypha

A small digital signage system built around retaining the last successful scene.
The server is written in Go; the browser renderer uses PixiJS 8 and TypeScript.
All documentation and source comments are written in English.

## Current implementation

This is the development-environment bootstrap. It includes:

- A Go HTTP server with graceful shutdown, health checks, and static renderer delivery.
- A PixiJS Builtin scene and serial polling of the display endpoint.
- Vite development with hot module replacement and a same-origin API proxy.
- A multi-stage Docker build and separate development Compose configuration.

`GET /display` currently returns `204` because no content service exists yet.
`PUT /content` returns a structured `501 not_implemented` error.
Content validation, SQLite storage, schedule generation, AST rendering, and ETag handling remain implementation work.
The sample content under `examples/` describes the proposed format and cannot be uploaded successfully yet.

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

Health returns `200` with `{"status":"ok"}`; display returns an empty `204`.
The runtime runs as a non-root user with a read-only filesystem. Persistent storage will be added with the content service; there is currently no database or content volume.

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

Volumes retain only development caches/dependencies. Stop commands do not delete them.
Compose publishes ports on loopback only. Container processes bind internally on all interfaces so port forwarding works.

## Checks

Go handler tests run during the application image build; the renderer build runs TypeScript checking.
For explicit checks without installing host runtimes:

```sh
docker compose -p glypha-dev -f compose.dev.yml run --rm --no-deps server go test ./...
docker compose -p glypha-dev -f compose.dev.yml run --rm --no-deps renderer sh -c 'npm ci && npm run build'
```

After changing Dockerfiles or dependency manifests, rebuild the relevant images before running checks.
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
| `GLYPHA_API_URL` | `http://127.0.0.1:8080` | `http://server:8080` in Vite development |

The image health-check command targets internal port 8080. If overriding the internal port, update its health check too.
`GLYPHA_API_URL` is a Vite proxy setting, never a URL embedded in browser code.

Dependencies are pinned in `renderer/package.json` and its lockfile. The Docker build uses Go 1.27.1 and the Node 24 Debian image family; base-image updates remain a deliberate rebuild concern.
Builtin currently uses a system serif font. It is not yet the bundled, reproducible rendering profile described in the design.
Errors are logged to the server or browser console and never replace Builtin.

## Design and models

- [Concept](Concept.md)
- [Accepted behavioral decisions](DesignDecisions.md)
- [Implementation design](design/README.md)
- [Alloy models and verification](model/README.md)

The backend is now selected as browser-based PixiJS. Exact font/layout compatibility, graphics resource ownership, and complete AST-to-frame validation are the next renderer design work.
