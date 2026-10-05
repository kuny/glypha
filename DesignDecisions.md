# Behavioral Decisions Before Design

Status: Accepted. This document records the six agreed decisions that precede detailed design.
It turns the responsibilities in Concept.md into contracts that remain meaningful when failures occur.

## 1. Delayed requests catch up to the latest unconsumed scene

Given a Clock input at second-level precision, select the latest unconsumed entry whose start time is at or before the current time.
Consume earlier unconsumed entries together with it; do not replay intermediate scenes in sequence.
There is no ten-second matching window. Clock rollback does not move the consumption boundary backward. If no entry is due, retain the current target.

When time jumps beyond the generated period, determine the latest scene across the elapsed interval, including portions that have not yet been generated.
Starting generation at the current time would lose the last transition in that interval, so generation must continue from the boundary of the previously generated period.
The finite batch size and an efficient algorithm for evaluating elapsed intervals remain detailed design choices.

## 2. Publish the scene applicable at acceptance time

Package validation must establish that exactly one applicable scene can be determined at publication time.
Reject packages that contain only future transitions and cannot determine an initial display.
The representation of the initial display, such as a default scene, will be decided when designing the content format.

Publication commits the current package, new schedule, initial target, and its consumption boundary as one unit.
Because time may change during validation, recheck applicability using the Clock input at publication time.
Invalidate pending entries from the previous generation. Applying a new upload is distinct from replaying history after clock rollback.
Internal generation identities serve consistency; they do not introduce content history or a version-management UI.

## 3. The target can be retrieved repeatedly

The server retains the current target AST. It is not a one-time notification emitted when an entry is consumed.
The renderer can retrieve the same target after a lost response or rendering failure. Consumption never rolls back to obtain a rendering acknowledgment.
Eventual delivery is not guaranteed if communication or rendering fails indefinitely.

## 4. The renderer retains display assets

Retrieve the AST and required images, validate them, and complete rendering preparation before replacing the display.
If any step fails, retain the displayed scene and its assets.
Deleting an old server package must not delete assets used by the renderer's displayed scene.

If the package changes during retrieval, either retain the old snapshot until transfer completes or fail the retrieval and retry the current target.
Do not treat a mixture of AST and images from different generations as a successful retrieval.
The specific transfer and retention mechanisms remain detailed design choices.

## 5. Consumption does not roll back after a server restart

Persist the consumption boundary and target associated with the current package.
A transition updates both atomically and commits before responding.
After a restart, resume from that durable state without restoring consumed entries, even if the clock has moved backward.

The generated-period boundary and consumption boundary are different.
The persistence design must either store pending entries or reconstruct the same entries from saved state that includes the generation boundary.
Disk failure and corruption are outside this guarantee.
Whether the renderer restores its previous screen from durable storage after its own process restarts is outside this decision.

## 6. Allow temporarily stale responses and process requests serially

The renderer processes acquisition through rendering one operation at a time.
A previously retrieved valid scene may be displayed even after the server advances to another target.
If successful communication and rendering continue and the target becomes stable, subsequent retrievals converge to that target.
Continuous agreement with the server's latest state is not required.

## Detailed design work that follows

Use these decisions to design the content format, schedule generation rules, persistence transactions,
snapshots and asset transfer, HTTP API, and renderer loop including retries.
Map atomic Alloy operations to implementation transactions and publication boundaries.
See [model/README.md](model/README.md) for verification scopes and abstraction limits.
