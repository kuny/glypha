# Verification and Implementation Order

Status: Acceptance criteria and implementation evidence. Content compilation, SQLite publication, HTTP snapshots, and PixiJS scene rendering are implemented.

## Traceability to the models

| Model contract | Implementation boundary | Required evidence |
|---|---|---|
| `OnlyValidatedPublished`, `ValidationBoundToStaging` | Private candidate and publication transaction | Rejected and interrupted uploads leave the previous package usable |
| `NoMixedPackage`, `CoherentGeneration` | One database transaction plus state mutex | Concurrent retrieval/publication returns a complete old or new snapshot |
| `NoDanglingAsset`, `ResponseOwnsAssets` | AST compilation and complete response envelope | Missing assets fail validation; old envelopes survive package replacement |
| `NoReplay`, `DurableNoReplay` | Generation-local durable cursor | Backward time and server restart never resurrect consumed occurrences |
| `EveryDuePollHasTarget`, `CatchUpLatest` | `LatestBetween` over the daily template | Delayed requests select the latest due occurrence, including gaps beyond the cache |
| `NoHistoricalRegeneration` | Direct lookup strictly after the durable cursor | Restart and rollback do not regenerate an already consumed occurrence |
| `DisplayOwnsAssets`, `FailurePreservesScene` | Candidate resource set and atomic presentation | Failures at acquisition, decode, layout, and swap preserve the active scene |
| `DeliveryWithRecovery` | Serial polling and retry loop | Stable target eventually displays when fetch and rendering repeatedly succeed |

The Alloy lifecycle model assumes atomic durable updates and atomic complete-snapshot acquisition. SQLite transactions and response snapshot ownership are implementations of those assumptions, not consequences proven by Alloy.
The concrete cursor compresses consumed history, the generator expands daily recurrence, and the renderer uses a platform graphics API. These refinements require implementation checks in addition to the existing 41 Alloy commands.

## Scheduling checks

Use an injected Clock and deterministic pure functions. Compare the optimized `LatestBetween` implementation against explicitly enumerated daily occurrences over bounded ranges.
Exercise:

- Exact midnight, exact transition seconds, one second before and after each transition.
- A static schedule, unsorted input, duplicate times, and successive entries selecting the same scene.
- Positive and negative UTC offsets, day boundaries, and negative Unix seconds where supported.
- Publication between transitions and before the first explicit transition.
- Forward jumps within the cache, beyond its horizon, across multiple months, and beyond the supported arithmetic range.
- Stopped time and rollback before publication time or before the last consumed occurrence.
- Consecutive generated intervals with no duplicate or missing boundary entry.
- Cache loss followed by reconstruction from a persisted cursor with a backward Clock sample.

The oracle enumerates occurrences independently; do not write an oracle that calls the optimized lookup under test.
Check equivalence of selected target and final cursor, not just that a function returns a value.

## Persistence and publication checks

Test against the actual selected SQLite driver and connection settings, using a subprocess that can be terminated at explicit hooks:

1. Before transaction start.
2. After replacement writes but before commit.
3. Immediately after commit but before publishing runtime state.
4. After runtime publication but before an HTTP response.
5. During a consumption update, including a transition that retains the same target scene.

After reopening, assert that package, target, cursor, AST references, and assets form one coherent state.
An old state or a new state is acceptable where commit completion was uncertain; a mixed state is never acceptable.
Inject storage failures separately to verify that ambiguous commits stop state-serving until recovery.
Process-kill checks validate application crash behavior; they do not establish device-level power-loss durability.

Hold a copied response while another upload commits. Its bytes, hashes, and references must remain valid after the old rows are deleted.
Verify that a slow HTTP client holds a transfer slot but not the state mutex or a database transaction.

## Protocol and renderer checks

- A lost successful upload response may be followed by a new publication on retry; do not promise upload deduplication.
- Advance a due entry before deciding to return `304`.
- Return only assets referenced by the selected AST, with hashes matching the transferred bytes.
- Fail malformed and oversized content without allocating an unbounded buffer.
- Supply a valid AST that fails actual rendering; retain the previous frame and successful ETag.
- Replace a server package while an older candidate is being prepared; allow a coherent old frame, then converge on retry.
- Ensure no second acquisition starts before the current attempt renders or fails.
- Exercise the real backend's preparation, swap, and resource-release ordering with visible-frame checks.
- Restart the renderer and verify the proposed Builtin startup behavior without resetting server scheduling state.

Logs and response error bodies must not appear on the signage surface.

## Build order

1. **Content and time core.** Add Go module/build settings, document the selected rendering profile, implement strict parsing, validation, daily recurrence, and the enumeration comparison checks.
2. **Durable server state.** Select the SQLite driver, implement publication and cursor/target transactions, and run restart and failure-injection checks before exposing a network API.
3. **HTTP snapshots.** Add bounded upload parsing, immutable envelopes, conditional retrieval, and concurrency checks.
4. **Renderer.** Implement the platform backend, Builtin frame, candidate preparation, atomic presentation, and serial retries.
5. **Appliance trial.** Run the example package through real clock jumps, server termination, network interruption, and package replacement while observing a real display.

Each step should leave a small reviewable change with its relevant checks. No deployment, background service installation, or external publication is implied by creating this design.

## Review points before implementation

Daily scheduling and the six behavioral decisions are accepted. The following concrete proposals should be reviewed as part of adopting this draft:

- Fixed UTC offset rather than a named timezone; no daylight-saving transitions.
- A default scene reapplied at every midnight, including when the previous evening selected another scene.
- SQLite containing both assets and state; a complete JSON/base64 display envelope.
- Builtin on renderer process restart, with durable replay protection provided by the server.
- The resource caps and poll/retry defaults.

The renderer backend is now selected as browser-based PixiJS. Noto Sans JP Regular (400) is the selected bundled font. The SQLite driver is modernc.org/sqlite v1.60.1; target-browser qualification remains deployment work. None is required to understand or verify the accepted behavioral contracts, but exact server/browser text-layout agreement still requires implementation verification.

## Current evidence and remaining checks

Automated tests cover font coverage and fitting, empty text round-trips, canonical restore, daily catch-up, rollback across restart, same-scene cursor persistence, immutable old snapshots, rejected uploads, conditional responses, failed writes, and commit-error fail-closed behavior. A parent process now kills a subprocess at four explicit service boundaries for publication, scene-changing consumption, and same-scene consumption. Reopening verifies the complete previous or committed state in all 12 cases. These tests do not simulate device power loss or interruption inside SQLite's commit implementation.

The renderer build performs TypeScript checking. Local browser trials cover the daily example, mixed Japanese/English text with an image, and preservation of that frame while the server is stopped. A read-only, non-root production container accepted an upload and restored the same generation after restart, returning `304` for its ETag. After clearing Docker build caches, the same upload/restart/conditional-response trial also passed with the production Compose named volume. Real-socket backpressure and disconnection tests now pass; sustained throughput measurements, storage I/O fault injection within SQLite commit, and target-device qualification remain follow-up work. Browser staging failures are now exercised by the development checks described below. Model checks remain separate evidence, not a proof of the Go or browser implementation.

### Deterministic HTTP integration coverage

`internal/httpserver/integration_test.go` exercises the real handlers, compiler, and SQLite store with controlled I/O boundaries:

- Two display responses pause at their first body write. A third receives `503` with `Retry-After`, while a replacement upload commits successfully. Released old responses retain their original generation and image pixels; the next response contains the new generation. Asset SHA-256 hashes and response ETags are checked against the actual bytes.
- An upload pauses while reading its multipart body. Another upload receives `503`, while display retrieval continues. Injecting an unexpected EOF returns `400` without changing the current envelope, and a subsequent upload succeeds.
- A conditional request at an exact daily transition returns `304` only after committing the new target. Reopening SQLite with a backward clock retains that target. The next midnight restores the default scene.

These tests use channel gates rather than elapsed sleeps. They pass under the Go race detector. They verify handler-level ownership and admission behavior; operating-system socket backpressure and target-device throughput remain separate acceptance work.

### Browser failure checks

`renderer/tests/index.html` runs 23 checks in a local browser using `Display`, `pollDisplay`, and the real PixiJS scene preparer. The test page is served by Vite in development and excluded from the production build. It supplies isolated responses and never changes server content. Start the development environment, open `/tests/`, and select **Run checks**. TypeScript checks compile these sources; browser execution is a separate step.

The suite covers complete frame/ETag replacement before old-frame disposal, failed DOM swaps, abort and shutdown during preparation, serial requests, successful ETag retention after preparation failure, bounded retry delays, unsolicited `304`, malformed JSON/UTF-8, oversized streamed bodies, unsupported profiles, asset digest mismatches, undecodable PNG data, missing references, and overflowing text. It injects font, decode, graphics initialization, and rendering failures. A later valid response must recover successfully.

Actual PixiJS applications, decoded bitmaps, and observed textures are checked for disposal after failures and repeated replacement. These checks verify API-level resource ownership; they do not measure driver memory or establish long-duration GPU stability. Injected faults use explicit platform dependencies instead of changing global browser APIs. The suite identified and fixed cleanup of an Application whose initialization failed before its renderer was created.

### Deferred appliance validation

Target hardware is not currently available. At the user's direction, target-device trials are deferred for this iteration. Long-duration operation, memory/performance measurements, physical-display resolution checks, browser qualification on that device, and device power-loss behavior remain unverified. Local browser checks and Docker integration tests continue independently; deferral does not count as a passing appliance trial.

### Process termination at transaction boundaries

`internal/store/crash_test.go` tests three operations at four boundaries:

| Operation | Before transaction | After write, before commit | After commit, before runtime update | After runtime update, before return |
|---|---|---|---|---|
| Replace package, including different image bytes | Previous state | Previous state | New state | New state |
| Consume a transition to another scene | Previous cursor and target | Previous cursor and target | Advanced cursor and target | Advanced cursor and target |
| Consume a transition retaining the same scene | Previous cursor | Previous cursor | Advanced cursor | Advanced cursor |

The child executes the real `Publish` or `Snapshot` method, signals arrival at an internal checkpoint, and blocks. The parent then kills it without running deferred rollback or store cleanup. Each case uses a private temporary database. Recovery samples a clock behind both possible cursors and compares generation, canonical package bytes, target, cursor, complete response bytes, ETag, and SQLite `integrity_check`. Different image bytes expose accidental mixing between package generations.

Checkpoints are unexported per-store callbacks installed only by tests. Production exposes no environment variable, HTTP control, or public option for activating them. The child-process environment is read only by the test helper.

Deferred-constraint fault injection also exercises commit errors in both publication and consumption. Once commit returns an error, snapshots and further publication are rejected until reopening. Reopening with a backward clock restores the unchanged durable snapshot for this known rollback case. This demonstrates conservative error handling, not arbitrary disk-failure recovery or power-loss durability.

### Final software verification

`internal/httpserver/network_test.go` uses real loopback TCP sockets with small buffers. Two clients stop reading multi-megabyte image envelopes, a third request receives admission failure, and another upload still commits. Closing the stalled sockets releases their slots. A separate test sends an incomplete multipart upload, confirms display retrieval remains available, closes the connection, and verifies the old ETag remains current before a successful retry.

The repository's Docker toolchains provide local race checks, Go static analysis, renderer type checks, production compilation, and `scripts/smoke.sh`. The smoke script uses fresh disposable volumes, a non-root read-only runtime, stopped-container backup, and restoration into another volume. It compares complete snapshots across restart and restoration. CI adoption is deferred at the user's request and no GitHub Actions workflow is enabled. Browser and Alloy suites are separate local checks.

See [initial implementation status](../RELEASE_STATUS.md) for completion boundaries and [operations](../ops/README.md) for the tested maintenance workflow.
