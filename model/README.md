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
