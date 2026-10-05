# HTTP API and Renderer

Status: Proposed protocol version 1. The renderer receives scenes, never schedules.

## Endpoints

| Method and path | Request | Successful result |
|---|---|---|
| `PUT /content` | `multipart/form-data`: exactly one `content` JSON part and zero or more `assets` file parts | `200` with publication generation and selected scene ID |
| `GET /display` | Optional `If-None-Match` for the last successfully displayed envelope | `200` with a complete scene envelope, `304` if unchanged, or `204` before any package is accepted |

No historical-content, acknowledgment, independent asset-download, or scheduling endpoint is required.
The existing external-tool workflow remains:

```sh
curl -X PUT http://localhost:8080/content \
  -F 'content=@examples/daily/content.json;type=application/json'
```

For packages with images, repeat `-F 'assets=@path/to/logo.png'`. Each asset part's filename must exactly match an asset ID in the content document.
Reject duplicate filenames, path components, missing assets, and extra parts. Do not use submitted filenames as filesystem paths.

## Upload results

A successful upload returns JSON such as:

```json
{"generation":"opaque-token","scene":"open"}
```

Generation is an opaque publication identity, not a content-history feature. A timeout after commit leaves acceptance uncertain to the caller.
Repeating the upload creates a new publication and reevaluates its initial scene; this endpoint does not promise deduplication or replay of a previous response.

Error responses use JSON:

```json
{
  "error": {
    "code": "validation_failed",
    "issues": [
      {"path":"/schedule/daily/0/scene","code":"unknown_scene","message":"The referenced scene does not exist."}
    ]
  }
}
```

| Status | Meaning |
|---|---|
| `400` | Malformed multipart or JSON, duplicate keys, or invalid request structure |
| `413` | Encoded request or configured resource limits exceeded |
| `415` | Unsupported request media type |
| `422` | Well-formed content fails semantic validation, including unsupported image formats or layout |
| `503` | Admission slot unavailable, storage unavailable, or an operation cannot safely complete; include `Retry-After` when appropriate |
| `500` | Unexpected internal failure; expose no stack trace or local path |

Issue codes and JSON paths are stable machine-readable fields; message wording is not a parsing contract.
Do not return a successful publication response before commit. An error is operational information and is never an AST.

## Self-contained display envelope

`GET /display` uses `application/json` and returns:

```text
{
  "protocol": 1,
  "generation": "opaque-token",
  "scene": "open",
  "ast": { ... compiled scene AST ... },
  "assets": {
    "logo.png": {
      "mediaType": "image/png",
      "sha256": "hex digest of decoded base64 bytes",
      "base64": "... encoded image file bytes ..."
    }
  }
}
```

Only assets referenced by the selected AST are included. The digest covers the original image file bytes, not decoded pixels.
Base64 adds transfer size, but a single envelope eliminates asset-lifetime races and additional download endpoints. Apply encoded and decoded size limits before allocation.

Return a strong ETag computed from deterministic serialized envelope bytes and `Cache-Control: no-cache`.
Use stable field/key ordering and the same immutable bytes for hashing and transmission. Different generations have different envelopes even if the pixels are identical.
Advance scheduling and persist any consumption before comparing `If-None-Match` with the current ETag.
A transition to the same scene can return `304` after still committing its cursor change.

A renderer stores an ETag only after that envelope has been successfully rendered and swapped into view.
An ETag from a failed preparation must never suppress a retry. A `304` without a known successful ETag is treated as a protocol failure and retried without a condition.
A `204` retains Builtin or the existing scene and continues polling. It does not clear the display.

## Renderer state machine

The renderer owns two distinct resource sets: the active scene and, during an attempt, a candidate scene.
Start with the bundled Builtin scene. The proposed initial renderer does not persist active scene resources across its own process restart; it returns to Builtin and retrieves again.
This is separate from the accepted requirement to preserve server consumption across server restart.

For each serial attempt:

1. Fetch the display envelope, conditionally using only the successful ETag.
2. Reject failed HTTP responses, malformed envelopes, unsupported versions/profiles, missing references, digest mismatches, or resource-limit violations.
3. Decode all images into candidate-owned resources and prepare all text layout and drawing operations without modifying active resources.
4. Produce a complete candidate frame offscreen. If any step fails, release candidate resources and keep the active frame and ETag.
5. Swap the visible frame on the rendering thread. Only after the backend reports a successful swap, adopt candidate resources and the response ETag.
6. Release old resources only after the backend no longer references them. Schedule the next attempt after completion, without overlapping attempts.

A valid old response may be displayed after a server publication; the next successful acquisition converges to the current target.
Timeouts, backoff, and periodic polling use monotonic elapsed time and do not inspect daily schedules or wall-clock values.
A `304` or valid `204` counts as a successful communication attempt for resetting retry backoff.

The backend contract must ensure that a failed candidate preparation or swap does not destroy the active frame.
If a platform API cannot provide that guarantee, adapt it with an offscreen surface and a retained active buffer before claiming conformance.
A renderer process crash, display disconnect, or loss of power is not an atomic-swap failure and is outside the uninterrupted-display guarantee.

The physical output preserves logical canvas aspect ratio, centers it, and fills unused area with black. It does not reflow text based on physical resolution.
The backend and bundled font are a platform decision that must be settled before implementing layout validation and presentation.

## Deployment boundary

The initial deployment is a local appliance or trusted network service; bind to loopback by default.
Expose the listener beyond that boundary only through an explicitly configured deployment policy. Internet authentication, user management, and TLS termination are not added to the Glypha protocol in this design.
Diagnostics go to process logs. They must never be substituted for display content.
