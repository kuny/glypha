# Invariants Before Design — Alloy 6

These models capture [Concept.md](../Concept.md) and the six accepted [design decisions](../DesignDecisions.md).
Three models isolate individual responsibilities; an integration model covers their boundaries.
They examine specification contracts, necessary assumptions, and counterexamples rather than proving an implementation correct.
They use Alloy 6's `var`, `always`, `eventually`, `after`, and the next-state operator `'`.

## Models and properties

| Model | Scope | Invariants and checks |
|---|---|---|
| [content.als](content.als) | Validation and publication | Never publish an unvalidated package. Publish AST and assets together. Never reuse validation results for another upload. Preserve current content on rejection. |
| [scheduler.als](scheduler.als) | Time and finite batches | Separate pending and consumed entries, consume monotonically, prevent historical regeneration and early transitions, and catch up to the latest unconsumed scene. |
| [renderer.als](renderer.als) | Rendering and failure | Display valid content from startup onward. Change the display only after successful rendering. Preserve it on failure. Converge after recovery under explicit conditions. |
| [lifecycle.als](lifecycle.als) | Generations, restarts, and assets | Keep package, pending entries, and target coherent. Preserve consumption across restarts. Retain assets for old responses and displayed scenes. Support retries and serial processing. |

Assertions are checked against initial states and operation guards and updates; they are not assumed as facts.
Each operation explicitly preserves fields it does not change. JSON syntax, pixel layout, HTTP formats, and filesystem behavior are outside the scope.

## How the accepted decisions are modeled

### 1. Catch-up and clock rollback

`scheduler.als` covers one content generation and permits stopped time, rollback, and forward jumps.
`due` contains unconsumed entries whose start seconds are at or before the current time. `winner` is the latest such entry.
There is no matching-window expiry. A poll consumes all due entries and sets the target to the winner's AST.
An input constraint prohibits conflicting entries with the same start second within a generation.

`consumed` includes skipped intermediate scenes, not just displayed scenes. It does not mean delivered or rendered.
When pending is empty, another batch can be added after the previously issued interval.
**New batches are not restricted to the current time or later.** If the clock jumps beyond the generated period,
transitions in the elapsed interval must also be added before catching up.
`RefillPastDue` explores consuming elapsed entries from a batch added after exhaustion.

Continuous period coverage and the ability to generate indefinitely are not proven.
The abstraction permits arbitrary subsets as batches, so generating every required entry remains an implementation contract.
`issued` and `consumed` are verification history; they do not require the implementation to retain all history.
The design must distinguish the generated-period boundary from the consumption boundary and map them to a compact representation.

### 2. Selecting the initial scene and switching generations

`content.als` models staging through `upload → validate → commit`.
Another upload discards the validation result; commit publishes AST and assets atomically.
Packages are immutable. Validation abstracts well-formed JSON, AST construction, and the presence of referenced assets.
Current content is empty before the first publication and contains at most one package thereafter. Staging is a workspace, not history.

In `lifecycle.als`, activate accepts only validated packages and additionally requires that the latest applicable scene
can be determined at publication time. It switches the current package, target, pending entries, and consumption state together.
Pending entries from the old generation are discarded. A static Package atom represents an upload generation, so the same atom is not published again.
This does not prohibit uploading identical content again: another upload is represented by a new generation.
Unlike the standalone content model's single-AST abstraction, the integration model permits multiple entries per package.

### 3. Repeated retrieval

The target remains after consumption and is not lost on fetch or failure.
`RetryAfterLoss` in the integration model checks reachability of fetch → failure → fetch → render.
Activate, advance, and fetch cannot execute while the server is down.

### 4. Asset retention

The integration model's response and copied fields represent a retrieved snapshot owned by the renderer.
Render can execute only when referenced assets are present, and updates display and retained assets together.
Displayed assets survive independently of package replacement.
`ReplaceDuringFetch` explores an old response retaining assets absent from the new package and remaining renderable after replacement.

Fetch abstracts a successful transfer of a complete snapshot as one operation.
Partial transfers or generation mismatches are discarded as failures, like other failures before the display swap.
Races involving deletion of old files during transfer and image decoding are not explored directly.
The implementation must either complete a coherent transfer or fail safely; atomic fetch expresses that contract.

### 5. Persistence and restart

The integration model treats current, target, pending, consumed, and used as durable state.
Restart switches the server between Up and Down while preserving these fields.
The specification assumes that a crash before or after an atomic operation cannot expose a partial publication.
It does not verify partial filesystem writes or persistence ordering.
`RestartThenRollback` explores shutdown after consumption, clock rollback, and recovery.

Pending entries must either be stored or reconstructed from saved state that yields the same set.
The integration model prepares a finite set of entries when publishing a generation; batch regeneration is covered by the standalone scheduler model.
This is therefore not a complete composition proof including the generator and persistence layer.
Renderer process restarts, disk corruption, and loss of power to the physical display are outside the model.

### 6. Serial processing and temporary staleness

Fetch in the integration model is enabled only when response is empty.
No new acquisition starts until the previous one has rendered or failed.
An old response may be rendered even if the target changes during acquisition.
The standalone renderer model covers rendering failure despite a valid AST, invalid responses, and the built-in initial scene.
Builtin is valid initial content; no technical error screen is introduced.

## Counterexamples and accepted decisions

The original model's `EveryDuePollChangesTarget` exposed a counterexample: polling at now=18 for start=8 left the target empty.
The matching window has been removed, and `EveryDuePollHasTarget` is now checked with `expect 0`.
`CatchUpAfterGap` confirms a reachable example in which a poll at least ten seconds late still selects a target.

Two hypotheses remain deliberately false and are checked with `check ... expect 1` to require counterexamples:

- **UnconditionalDelivery**: a target may never be displayed if communication or rendering keeps failing.
  `DeliveryWithRecovery` checks convergence under the strong assumption that the target eventually stabilizes and
  successful fetch-then-render operations recur. A single communication recovery does not establish that assumption.
- **AlwaysLatest**: if the target changes after retrieval, an old valid response can still render successfully.
  This agrees with the accepted policy of allowing temporary staleness.

## Correspondence to implementation tests

Assertion names do not appear in the test sources. This table records the correspondence by comparing each
assertion's formula with what the tests actually observe.
[Verification and implementation order](../design/verification-and-delivery.md) maps the same assertions to
implementation boundaries and required evidence; this table maps them to the checks that produce that evidence.

Short names used below: `content` = `internal/content/validate_test.go`, `compiler` = `internal/compiler/compiler_test.go`,
`schedule` = `internal/schedule/daily_test.go`, `store` = `internal/store/store_test.go`, `crash` = `internal/store/crash_test.go`,
`api` = `internal/httpserver/api_test.go`, `integration` = `internal/httpserver/integration_test.go`,
`network` = `internal/httpserver/network_test.go`, `browser` = `renderer/tests/renderer.ts`.

Correspondence strength: **Direct** means a check observes the assertion's own conclusion. **Partial** means it observes
only some of the transitions or fields the assertion covers. **Indirect** means the modeled state does not exist in the
implementation and a refined property stands in for it. **None** means no corresponding implementation check exists. The two hypotheses checked with `expect 1`
are deliberately false; implementation tests may illustrate their counterexamples. These labels describe
observations in finite tests, not proofs of the Alloy formulas over all executions.

### content.als

| Assertion | Implementation checks | Correspondence |
|---|---|---|
| `OnlyValidatedPublished` | `content`: TestStrictJSON, TestSemanticRejections, TestAssets, TestResourceLimits, FuzzValidateJSON; `compiler`: TestCompileRejectsTextBeforePublication; `api`: TestUploadRetrieveAndReject. Accepting side: `content`: TestDailyExample, TestImageElementAndJPEG, TestIssuePathsAndStaticContent; `compiler`: TestEmptyTextRoundTrip | Direct for the exercised publication cases. The 26 strict-JSON and semantic rejection cases test validation alone; they do not inspect stored state. The API test separately verifies unchanged ETags for three rejected uploads (duplicate JSON keys, an unused asset, and text overflow), and successful publication of the daily example. |
| `NoMixedPackage` | `integration`: TestSlowTransfersDoNotBlockPublication through `assertImageEnvelope`; `crash`: TestCrashRecoveryBoundaries; `compiler`: TestCompileAndRestore | Direct. Distinct image bytes per generation make mixing observable rather than only scene identifiers. |
| `NoDanglingAsset` | `content`: TestAssets; `integration`: `assertImageEnvelope`; `browser`: missing image reference | Direct. Covers missing, unused, invalid, animated, and oversized assets. |
| `ValidationBoundToStaging` | `api`: TestMultipartAndLimits; `integration`: TestInterruptedUploadRetainsCurrentAndReleasesAdmission; `network`: TestSocketInterruptedUpload | Partial. No durable field corresponds to `checked`; staging is confined to one request, so only discarding an interrupted or duplicated upload is observed. |
| `RejectionPreservesCurrent` | `api`: TestUploadRetrieveAndReject; `integration`: TestInterruptedUploadRetainsCurrentAndReleasesAdmission; `network`: TestSocketInterruptedUpload; `store`: TestSameSceneConsumesAndFailedWritePreservesDurableState, TestCommitFailureStopsServing | Direct, and extended beyond the model to failed writes and ambiguous commits. |

### scheduler.als

The implementation keeps no `issued`, `pending`, or `consumed` set. It compresses them into the daily template plus one
durable cursor, so assertions about those sets have no direct counterpart.

| Assertion | Implementation checks | Correspondence |
|---|---|---|
| `Partition` | `schedule`: TestConsecutiveBatches, TestLookupMatchesEnumeration | Indirect. Stands in as consecutive generated intervals with no duplicate, missing, or out-of-range occurrence. |
| `NoReplay` | `schedule`: TestCatchUpRollbackAndRestart; `store`: TestRestartRollbackAndSnapshots; `integration`: TestConditionalSchedulingPersistsBefore304 | Direct. Lookups at the cursor, one second before it, and one day before it all return nothing. |
| `SingleWinner` | `schedule`: TestInvalidInputs, TestLookupMatchesEnumeration; `content`: TestSemanticRejections (`duplicate_time`) | Direct. The model's input constraint on distinct start seconds is implemented as input validation. |
| `NoEarlyTransition` | `schedule`: TestDailyBoundaries, TestLookupMatchesEnumeration; `store`: TestRestartRollbackAndSnapshots | Direct. Exact transition seconds, one second before and after, and negative Unix seconds. |
| `PollDrainsObsolete` | `store`: TestSameSceneConsumesAndFailedWritePreservesDurableState; `schedule`: TestCatchUpRollbackAndRestart; `crash`: TestCrashRecoveryBoundaries (`advance`, `same_scene`) | Direct. The cursor advances past skipped occurrences, including a transition that retains the same scene. |
| `NoHistoricalRegeneration` | `schedule`: TestCatchUpRollbackAndRestart, TestConsecutiveBatches; `integration`: TestConditionalSchedulingPersistsBefore304 | Indirect. The model orders newly issued entries beyond every previously issued entry. The implementation has no issued set or generation horizon; tests observe strictly-after-cursor lookup and ordered generated intervals, including a five-month forward jump. |
| `EveryDuePollHasTarget` | `schedule`: TestCatchUpRollbackAndRestart, TestStaticScheduleStillConsumesMidnight; `store`: TestRestartRollbackAndSnapshots; `integration`: TestConditionalSchedulingPersistsBefore304 | Direct. Includes a request one hour after the due transition and a static schedule consuming midnight. |

### renderer.als

| Assertion | Implementation checks | Correspondence |
|---|---|---|
| `DisplayAlwaysValid` | `browser`: unsolicited `304`, real PixiJS preparation, and the 13 checks sharing the `preserve` helper | Direct. The startup frame stands for Builtin and is never replaced by an invalid response. |
| `FailurePreservesDisplay` | `browser`: the 13 `preserve` checks — malformed JSON and UTF-8, `503`, oversized stream, unsupported profile, digest mismatch, undecodable PNG, missing reference, text overflow, and injected font, decode, initialization, and rendering failures | Direct. |
| `ChangeRequiresSuccessfulRender` | `browser`: swap ordering before old-frame disposal, failed DOM swap, abort during preparation, shutdown during preparation | Direct. |
| `UnconditionalDelivery` (`expect 1`) | None | None by intent; the assertion is deliberately false. `browser`'s bounded retry backoff shows the matching behavior: repeated `503` never changes the frame. |
| `AlwaysLatest` (`expect 1`) | `integration`: TestSlowTransfersDoNotBlockPublication; `browser`: An old response renders after target replacement, then polling converges | The browser check directly observes a finite counterexample: A renders after the simulated target changes to B, then the next poll renders B. It uses real PixiJS preparation with simulated transport. Coverage across the real server/browser boundary remains partial; the integration test checks publication separately. |
| `DeliveryWithRecovery` | `browser`: A stable target survives fetch and render failures, then remains displayed on 304; serial polling and backoff reset | Partial. With simulated transport and real PixiJS preparation, a fixed target recovers after fetch and render failures, then survives two conditional `304` responses with the same canvas and ETag. These finite examples support the implementation behavior; they do not establish the temporal assertion of eventual permanent convergence under recurring successful fetch/render. |

### lifecycle.als

| Assertion | Implementation checks | Correspondence |
|---|---|---|
| `CoherentGeneration` | `crash`: TestCrashRecoveryBoundaries; `store`: TestRestartRollbackAndSnapshots, TestUnknownSchemaFails; `integration`: `assertImageEnvelope` | Direct. Three operations at four boundaries compare generation, canonical package bytes, target, cursor, response bytes, ETag, and `integrity_check`. |
| `DurableNoReplay` | `store`: TestRestartRollbackAndSnapshots, TestSameSceneConsumesAndFailedWritePreservesDurableState; `integration`: TestConditionalSchedulingPersistsBefore304; `crash`: TestCrashRecoveryBoundaries | Direct. Recovery samples a clock behind both possible cursors. |
| `DisplayOwnsAssets` | `browser`: active-resource assertions inside `preserve`, later valid frame, repeated replacement | Direct. Candidate applications, bitmaps, and textures are released while the active ones stay alive. |
| `ResponseOwnsAssets` | `integration`: TestSlowTransfersDoNotBlockPublication, `assertImageEnvelope`; `network`: TestSocketBackpressureAndDisconnectedReaders | Direct. Two stopped transfers retain their own pixels and ETag after the old rows are replaced. |
| `RestartPreservesTarget` | `store`: TestRestartRollbackAndSnapshots, TestCommitFailureStopsServing; `integration`: TestConditionalSchedulingPersistsBefore304; `crash`: TestCrashRecoveryBoundaries | Direct. Reopening with a backward clock retains the consumed target. |
| `CatchUpLatest` | `schedule`: TestCatchUpRollbackAndRestart, TestLookupMatchesEnumeration; `store`: TestRestartRollbackAndSnapshots; `api`: TestUploadRetrieveAndReject | Direct. An independent enumeration oracle checks the latest due occurrence over 1000 randomized trials. |
| `FailurePreservesScene` | `browser`: the `preserve` checks, failed DOM swap, HTTP failure | Direct. Display and retained assets are checked together. |

### Reachability scenarios

The `run` predicates also have implementation counterparts, though they are examples rather than contracts.

| Scenario | Implementation check |
|---|---|
| `ReplaceAfterRejection`, `MissingAssetRejected`, `CrashAfterValidation` | `api`: TestUploadRetrieveAndReject; `content`: TestAssets; `crash`: `before_begin` and `after_write` phases |
| `RollbackAfterConsumption`, `CatchUpAfterGap`, `SkipIntermediate`, `SkipSeveral` | `schedule`: TestCatchUpRollbackAndRestart |
| `ExhaustAndRefill`, `RefillPastDue` | `schedule`: TestConsecutiveBatches |
| `StoppedClockProgress` | `store`: TestSameSceneConsumesAndFailedWritePreservesDurableState |
| `RecoverAfterFailure`, `RetryAfterLoss` | `browser`: serial polling, later valid frame |
| `InvalidWhileDisplaying`, `ValidButRenderFails` | `browser`: malformed responses, injected rendering failure |
| `ReplaceDuringFetch` | `integration`: TestSlowTransfersDoNotBlockPublication (old responses retain image bytes absent from the replacement package). The browser target-replacement check uses identical image assets in A and B, so it does not cover this asset-difference scenario. |
| `RestartThenRollback` | `integration`: TestConditionalSchedulingPersistsBefore304; `store`: TestRestartRollbackAndSnapshots |

### Known gaps in the correspondence

`Partition` and `NoHistoricalRegeneration` have indirect evidence through interval generation and cursor-based lookup.
The implementation does not expose the modeled sets or issued-entry frontier, so their formulas are not observed directly.
`ValidationBoundToStaging` remains partial for the same kind of reason.
`UnconditionalDelivery` has no dedicated implementation check; its Alloy counterexample is intentional.
`DeliveryWithRecovery` has finite recovery examples rather than a proof of its temporal formula.

Several checks have no counterpart here because the model excludes their subject: route and status handling
(`internal/httpserver/server_test.go`), font coverage and fitting (`compiler`: TestFontLayout), UTC offset syntax
(`schedule`: TestInvalidInputs), request size and media type limits (`api`: TestMultipartAndLimits), and socket
backpressure with transfer admission (`network`). JSON syntax, pixel layout, HTTP formats, and filesystem behavior
are outside the models' scope.

The Go checks run under `go test`; the 25 browser checks are started by hand from `/tests/` in the development
environment. The six renderer-side assertions therefore depend on a manual run for their evidence.

## Execution and results

All 41 commands were checked with Alloy 6.2.0 / SAT4J. The 23 invariant and conditional-property checks were UNSAT (no counterexample),
the two hypotheses above were SAT (expected counterexamples), and all 16 reachability scenarios were SAT.

The usual scope is at most four atoms per signature and 1–8 states, with ten states for a longer recovery scenario.
The integration model uses at most three atoms per signature and 1–7 states, or eight states for scenarios; ordering fixes Tick to exactly three atoms.
Some scheduler scenarios use three atoms and six states. See each command for its exact scope.
Scheduler seconds use 6-bit Int, with start=0..20 and now=0..30.
The scenario expression start+10 is at most 30, so it cannot overflow.
Tick in the integration model abstracts the ordering of seconds, not their numeric spacing.
Sub-second truncation, time zones, and daylight-saving conversions are not modeled.
These are bounded checks of looping traces represented by a finite number of states, not proofs for arbitrary sizes or trace lengths.

Obtain the JAR from the [official release](https://github.com/AlloyTools/org.alloytools.alloy/releases/tag/v6.2.0)
and run from the repository root. Java and Python 3 are required.

```sh
python3 model/check.py /path/to/org.alloytools.alloy.dist.jar
```

The script compares each command's SAT/UNSAT result with `expect` and checks for missing commands.
Each run writes to a fresh temporary directory, retaining receipt.json and XML traces for visualization.
To execute one model, specify an unused output directory:

```sh
java -jar /path/to/org.alloytools.alloy.dist.jar exec -c '*' -t xml -o /tmp/glypha-lifecycle-check model/lifecycle.als
```

In the GUI, open an `.als` file and choose a command under Execute. A counterexample for a check with `expect 0` requires investigation.
For a check with `expect 1`, a counterexample is the expected result. An UNSAT run means the intended scenario is unreachable.
See the [official Alloy 6 overview](https://alloytools.org/alloy6.html) for temporal operators and looping-trace semantics.
