# Server and Persistence

Status: Initial implementation of the [accepted decisions](../DesignDecisions.md).

## Ownership

The server has four internal responsibilities, without introducing separate services:

- Content validation and compilation produce an immutable candidate package.
- The scheduler is pure logic over compiled rules, a cursor, and an explicit second timestamp.
- The store owns atomic publication and durable scheduling progress.
- HTTP handlers translate requests and copy immutable response snapshots.

Only the composition root reads configuration and constructs the system Clock. Scheduler functions receive integer Unix seconds. No scheduler function calls the operating system clock directly.

Use one application mutex around operations that inspect or mutate the current generation and target. Do not hold it while receiving uploads, performing preliminary validation, or writing a network response. A single server process owns the database; running competing server processes against it is unsupported in the initial deployment.

## Durable representation

Use a single SQLite database on local storage. Store package metadata, compiled scenes, asset bytes, and scheduling state in that database. This avoids a commit protocol spanning a database and a separately renamed asset directory.

Schema version 1 uses one `current` row with singleton key `1`, generation token, canonical package BLOB, `cursor` Unix second, and target scene ID. The BLOB contains normalized source, compiled scenes, and image bytes. This replaces the draft's separate scene and asset tables: a whole-package replacement is the only mutation needed, so one row keeps the transaction and ownership boundary explicit.

JSON-contained references are validated before publication and again at startup. Startup recompiles the stored source and compares the canonical result, rejecting unsupported profiles or inconsistent ASTs. The driver is pinned to `modernc.org/sqlite` v1.60.1, allowing a CGO-free Go binary.

The design term `consumed_through` corresponds to the `cursor` column below.

`consumed_through` is a generation-local high-water mark: all occurrences at or before it are logically consumed or superseded by initial publication. It is initialized to the publication timestamp. A later catch-up advances it to the latest consumed occurrence, not necessarily to the current Clock sample. When no occurrence is due, there is no scheduling write.

Do not persist all consumed entries. The initial service uses direct `LatestBetween` lookup over the bounded daily template, without materializing a pending cache. This also handles long gaps without enumerating intervening days. The standalone `Generate` API remains available; any future finite cache must preserve an exclusive lower bound and inclusive horizon.

SQLite rollback journaling with `journal_mode=DELETE`, `synchronous=EXTRA`, and `foreign_keys=ON` is the initial storage policy. Apply and verify connection settings at startup; do not depend on driver defaults. The selected driver runs with one connection; settings are applied through connection pragmas.

SQLite documents atomic transactions in [Atomic Commit](https://www.sqlite.org/atomiccommit.html). Its [synchronous setting](https://www.sqlite.org/pragma.html#pragma_synchronous) explains the additional directory synchronization provided by EXTRA in DELETE mode. Durability still depends on the filesystem and device honoring synchronization; database corruption and faulty storage remain outside the model.

## Publication algorithm

1. Acquire the upload admission slot. Read the bounded multipart request into a private candidate workspace; never overwrite current assets while uploading.
2. Parse and validate the document, assets, dimensions, resource limits, rules, and scene references. Compile every scene into an AST. Perform image decoding checks and any backend-independent layout checks.
3. Acquire the state mutex. Begin a database transaction and sample `Clock.Now().Unix()` exactly once for this publication. Determine the applicable initial scene from the compiled rules at that sample.
4. Generate a new opaque generation token. Replace all current package rows, set the target, and initialize `consumed_through` to the sampled second. Validate the resulting durable representation before committing.
5. Commit. Only then publish the corresponding in-memory generation and invalidate any old pending cache. Release the mutex and return success.
6. Release staging resources and the upload slot on every path.

A candidate that fails before commit leaves all current rows and runtime pointers unchanged. Existing snapshots already copied for network transmission remain usable. Sequential accepted uploads replace each other in publication order; there is no conditional-update protocol in the initial API.

The publication Clock sample is the logical applicability point, not the time at which a slow disk finishes committing. If the clock moves during commit, the next retrieval advances from the persisted cursor using a new sample. Sampling again after commit would break the single atomic decision.

## Retrieval and advancement

1. Acquire a transfer admission slot, then the state mutex.
2. If there is no package, return the empty-content result.
3. Sample Clock once. Compute the latest unconsumed due occurrence directly with `LatestBetween`. A backward sample never decreases the durable cursor.
4. If an occurrence is due, update target and cursor in one transaction and commit. Persist cursor progress even if the new scene is the same as the previous target scene.
5. Read the selected AST and all referenced asset bytes from that coherent generation. Build an immutable, bounded response envelope and its ETag while still excluding publication. A cached envelope for the same generation and scene may be reused.
6. Release database resources and the mutex before writing any network bytes. Apply the request's conditional ETag only after advancement.
7. Release the transfer slot after transmission succeeds or fails.

For an unchanged target, no transaction write is needed. A response lost after commit does not undo consumption. A retry retrieves the retained target. No acknowledgment endpoint is required.

A response envelope already copied into server-owned memory has a lifetime independent of SQLite package replacement. Deleting old rows cannot produce a mixture of generations in that envelope. This bounds how long the state mutex is held without pinning a database transaction across a slow network connection.

## Failures and restart

| Failure point | Required outcome |
|---|---|
| Upload, parsing, validation, or compilation | Current package, target, and cursor stay unchanged |
| Before a transaction commits | Recovery exposes the previous complete state |
| After commit but before HTTP success arrives | Recovery exposes the committed complete state; the client may not know acceptance succeeded |
| Network transfer after a target commit | Keep the committed target; retry can retrieve it |
| Snapshot preparation after a target commit | Return failure; the committed target remains available for a later retry |
| Commit returns an ambiguous I/O error | Stop serving state, discard runtime caches, and reopen/recover the store before serving again |
| Restart with clock behind the cursor | Restore target and cursor; do not replay earlier occurrences |
| Unsupported or corrupt stored schema | Do not silently create an empty store or publish Builtin; fail server startup with an operational diagnostic |

A commit error must not be treated as proof that the old generation is still current. The implementation must reestablish durable truth before publishing runtime pointers or returning another snapshot.

On ordinary startup, open and validate the store, restore target and cursor, and reconstruct pending state lazily. The initial renderer scene is a renderer responsibility, not a database recovery substitute.

## Internal API shape

These are responsibility boundaries, not a requirement to introduce an interface for every function:

```text
// Content and scheduling functions do not perform I/O.
Compile(document, assets, profile) -> Candidate | ValidationErrors
ApplicableAt(rules, nowSecond) -> SceneID
LatestBetween(rules, exclusiveStart, inclusiveEnd) -> OptionalOccurrence
Generate(rules, exclusiveStart, inclusiveEnd) -> []Occurrence

// Service operations own serialization and commit ordering.
Publish(candidate) -> Publication | Error
Snapshot(ifNoneMatch) -> SnapshotResult | Error
```

The service injects Clock and Store. The renderer receives neither rules nor occurrences. Domain failures carry stable machine-readable codes rather than HTTP status numbers inside scheduling logic.
