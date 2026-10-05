# Initial Implementation Status

The initial software implementation is complete within the agreed scope: daily times with a fixed UTC offset, one current package, static text and images, a Go server, SQLite persistence, and a PixiJS browser renderer. This document records verification scope; it is not a hardware qualification or a published release tag.

## Completed

- Alloy 6 models and the accepted behavioral decisions.
- Strict content and asset validation, pinned Noto Sans JP Regular (400), and scene compilation.
- Atomic publication, durable scheduling, rollback protection, and coherent asset snapshots.
- Bounded multipart HTTP uploads, serial browser polling, ETags, and complete frame replacement.
- Docker production/development configurations and persistent volumes.
- Go race checks, deterministic concurrent HTTP tests, and 12 process-kill recovery cases.
- Real TCP tests for unread display responses, simultaneous publication, disconnected readers, and interrupted uploads.
- 25 development-browser checks covering failures, recovery, and resource ownership.
- Offline backup/restore procedures and a disposable runtime smoke script.

The documented Go, renderer, runtime, browser, and Alloy checks are run locally. No GitHub Actions workflow is enabled.

## Deferred for this iteration

CI adoption and hosted automated checks are deferred at the user's request.

The user has deferred the appliance trial because target hardware is unavailable:

- Long-duration operation and memory/performance measurements on the target device.
- Physical display resolution, kiosk startup, and browser qualification on that device.
- Device power loss and storage behavior under real hardware faults.

Detailed I/O error injection inside SQLite's commit implementation is also follow-up work. Existing process termination and deferred-constraint tests establish narrower properties; they do not prove arbitrary disk-failure or power-loss durability.

## Operational boundaries

The initial deployment assumes a trusted local environment and one server per database. Internet authentication, multiple-server operation, content history, a management UI, and a dedicated CLI remain outside the agreed scope. Backups are external operational artifacts; restoring an older backup restores its older scheduling history.

See [operations](ops/README.md), [verification evidence](design/verification-and-delivery.md), and [browser checks](http://localhost:5173/tests/) for reproduction steps and limits.
