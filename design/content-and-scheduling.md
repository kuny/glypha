# Content and Scheduling

Status: Proposed content format and scheduling algorithm. Daily time-of-day scheduling is the selected initial use case.

## Package format

A package consists of one JSON document and its referenced static image files.
[The daily example](../examples/daily/content.json) requires no assets and illustrates the complete proposed input shape.
It is a format example, not a fixture already validated by an implemented Glypha parser.

Required document fields:

| Field | Meaning |
|---|---|
| `format` | Integer `1`; reject unsupported versions |
| `canvas` | Positive integer width and height of the logical display area |
| `schedule.utcOffset` | Explicit fixed UTC offset in `±HH:MM`, from `-14:00` through `+14:00` |
| `schedule.defaultScene` | Existing scene ID; displayed from local midnight until the first daily transition |
| `schedule.daily` | Zero to 64 entries, each with `at` and an existing `scene` ID |
| `scenes` | Map of one to 64 scene IDs to scene definitions |

The configured display profile fixes the canvas dimensions. An upload with different dimensions is rejected.
Scene and asset IDs use 1–128 ASCII characters from letters, digits, `_`, `-`, and `.`; reject `.` and `..` as complete IDs.
Reject duplicate JSON keys, unknown fields, duplicate transition times, missing references, and unused uploaded assets.
Use strict parsing before constructing ordinary maps so duplicate keys cannot be silently overwritten.

Daily `at` values use exactly `HH:MM:SS`, range `00:00:01` through `23:59:59`. The default scene supplies the midnight entry, so an explicit `00:00:00` entry is rejected.
Sort accepted entries by time; input array order has no scheduling meaning. Multiple transitions may reference the same scene.
A schedule with no daily entries is a valid static scene schedule.

A fixed offset is a proposed initial simplification. It is independent of the server's OS timezone and never adjusts for daylight saving.
Named time zones, daylight-saving behavior, weekdays, holidays, and date-specific overrides are not part of format 1.
Deployments requiring those semantics need a deliberate extension; they must not be approximated by silently using the host timezone.

## Scene semantics

A scene contains `background` and an ordered `elements` array. Draw the background first and elements in array order, with later elements above earlier ones.

- Background: required opaque `color` in `#RRGGBB`; optional `image` asset ID and `fit` of `contain` or `cover` (required when an image is present). Position the image at the center of the canvas over the color.
- Text element: `type: "text"`, integer `x`, `y`, `width`, `height`, UTF-8 `text`, positive integer `fontSize`, opaque `color`, and `align` of `left`, `center`, or `right`.
- Image element: `type: "image"`, the same rectangle fields, an `asset` ID, and `fit` of `contain` or `cover`. Center the image in the rectangle; clip cover to that rectangle.

Rectangles have nonnegative origins, positive dimensions, and must lie inside the logical canvas. Reject arithmetic overflow.
Text is plain text, has no markup, and uses the bundled font for the rendering profile. Newlines create explicit lines; there is no automatic wrapping or shrink-to-fit in format 1.
Use a line height of 1.2 times the font size, top alignment, and the selected backend's shared font metrics. If glyphs or the measured text do not fit, reject rather than silently substitute or clip.
The drawing backend and font file must be selected together, and server compilation and renderer preparation must share their layout rules and profile identifier.

Accept static PNG and JPEG assets. Verify media type by decoding the bytes, not by trusting filenames or request headers.
Reject animation, external URLs, uploaded fonts, and unsupported formats. Check decoded dimensions and aggregate pixel limits before allocating full image buffers where possible.
The renderer may still fail at preparation time, so server validation does not authorize destructive screen updates.

## Compiled scene AST

The server compiles a scene into a versioned AST containing only:

- The logical canvas and rendering-profile identifier.
- A background operation and an ordered list of text/image drawing operations.
- Resolved rectangles, text layout inputs, colors, and local asset IDs.

An AST has no schedule, wall-clock timestamp, recurrence rule, or reason for selection.
Do not expose content source as executable HTML, CSS, or script. A concrete JSON schema for drawing operations should be added with the first compiler implementation and shared with the renderer.

## Daily semantics

Interpret the daily template as an infinite sequence of occurrences in a fixed offset:

1. At local midnight, select the default scene.
2. At each listed daily time, select that entry's scene.
3. A selected scene lasts until the next occurrence.

At publication, choose the last occurrence at or before the sampled time and set the consumption cursor to that sample.
Before the first listed transition each day, the default scene applies. Midnight resets the scene even when the previous evening's scene was different.
This midnight behavior is an explicit proposed format rule, not an inference from the original concept.

Let `offset` be the signed offset in seconds, `t` a Unix second, and `day = floor((t + offset) / 86400)`.
The UTC second of that local day's midnight is `day * 86400 - offset`.
Each template entry has a second-of-day value; include `(0, defaultScene)` as an implicit entry.
Occurrences are identified by `(generation, UTC second)`, not by the scene ID.
Use mathematical floor division and checked integer arithmetic. Do not implement negative timestamps with truncation toward zero.

`ApplicableAt(t)` selects the template entry with the largest second-of-day not exceeding `t + offset - day * 86400`.
There is always a result because the template contains midnight.

For an exclusive consumed cursor `c` and inclusive request time `n`:

```text
LatestBetween(c, n):
    if n <= c: return none
    occurrence = latest template occurrence at or before n
    if occurrence.time <= c: return none
    return occurrence
```

Every earlier occurrence in `(c, occurrence.time]` becomes consumed by advancing the cursor.
This direct lookup skips intermediate scenes without iterating across every elapsed day.
If the clock is outside the implementation's supported arithmetic/date range, return a scheduling error without updating durable state.

## Finite schedule cache and catch-up

The cache represents every occurrence in `(c, g]`, where `c` is the durable cursor and `g` is a generated-through second.
Initially set `g = c` and generate a 24-hour interval `(g, g + 86400]`. With at most 64 explicit transitions plus midnight, this contains at most 65 occurrences.
Generation uses exclusive/inclusive boundaries consistently, so adjacent batches neither repeat nor omit an occurrence.

If a request time `n` is within the cache horizon, select the latest pending entry at or before `n` and remove its consumed prefix after the durable update succeeds.
If `n > g`, compute `LatestBetween(c, n)` directly over the recurrence rules. Do not select only from the stale cache or start a new interval at `n` before accounting for the gap.
After committing any returned occurrence, discard the old cache and generate the next 24-hour interval after `n`.
The recurrence lookup has covered `(c, n]`, including any portion that was not materialized as entries.
If no occurrence is returned, the same coverage argument permits moving the in-memory horizon without moving the durable cursor.

Cache updates are installed only after the corresponding durable state is established. On a failed commit, discard speculative cache changes.
On restart, reconstruct from the durable cursor, even if wall-clock time is behind it. Generation is derived data; the cursor and target are authoritative.

The direct recurrence lookup refines multiple generation-and-consumption steps into one operation. Its completeness is not yet proven by the existing Alloy models; implementation verification must compare it with explicit occurrence enumeration.

## Examples

For the provided template at `+09:00`:

| Situation | Result |
|---|---|
| Publish at 09:00 | `closed`; cursor starts at publication time |
| First subsequent request at 14:00 | `afternoon`; the 10:00 and 13:00 occurrences are consumed together |
| Clock moves from 14:00 back to 11:00 | Keep `afternoon`; do not restore the 10:00 occurrence |
| Process restarts while clock is still at 11:00 | Restore the same target and cursor |
| Request at 18:00 | Select `closed`, even after a long communication gap |
| Request at 00:30 next day | Consume midnight; select `closed` |
| Clock jumps ahead by several months | Select the applicable occurrence near the new time directly; do not enumerate months of batches |
| Clock jumps far ahead and later returns | Keep the advanced target until time passes the consumed boundary, or a new package is published |

The last behavior follows the accepted no-replay policy. A corrected clock is not a command to reconstruct historical display state.
