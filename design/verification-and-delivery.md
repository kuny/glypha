# Verification and Implementation Order

Status: Implementation acceptance criteria. No server or renderer implementation exists yet.

## Traceability to the models

| Model contract | Implementation boundary | Required evidence |
|---|---|---|
| `OnlyValidatedPublished`, `ValidationBoundToStaging` | Private candidate and publication transaction | Rejected and interrupted uploads leave the previous package usable |
| `NoMixedPackage`, `CoherentGeneration` | One database transaction plus state mutex | Concurrent retrieval/publication returns a complete old or new snapshot |
| `NoDanglingAsset`, `ResponseOwnsAssets` | AST compilation and complete response envelope | Missing assets fail validation; old envelopes survive package replacement |
| `NoReplay`, `DurableNoReplay` | Generation-local durable cursor | Backward time and server restart never resurrect consumed occurrences |
| `EveryDuePollHasTarget`, `CatchUpLatest` | `LatestBetween` and finite cache | Delayed requests select the latest due occurrence, including gaps beyond the cache |
| `NoHistoricalRegeneration` | Cache reconstructed after the durable cursor | Restart and rollback do not regenerate an already consumed occurrence |
| `DisplayOwnsAssets`, `FailurePreservesScene` | Candidate resource set and atomic presentation | Failures at acquisition, decode, layout, and swap preserve the active scene |
| `DeliveryWithRecovery` | Serial polling and retry loop | Stable target eventually displays when fetch and rendering repeatedly succeed |

The Alloy lifecycle model assumes atomic durable updates and atomic complete-snapshot acquisition. SQLite transactions and response snapshot ownership are proposed implementations of those assumptions, not consequences proven by Alloy.
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

The target OS/window system, renderer backend, bundled font, and SQLite driver remain implementation selections. None is required to understand or verify the accepted behavioral contracts, but backend/font selection is required before finalizing exact text-layout validation.
