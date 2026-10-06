# A Tour of Glypha

Glypha displays a static scene and changes it at daily times you choose. A scene can contain text and images on a colored background. When a new scene cannot be prepared, the browser keeps the last successful scene visible.

This tour introduces the implemented content format, one small example at a time. You do not need to know Go or TypeScript to author content. You write JSON, supply any image files it references, and publish the package through HTTP.

## 1. Meet the Content Package

A package is one `content.json` file plus its referenced images. It describes the entire current program for the sign, not an incremental edit.

| Part | What it describes |
| --- | --- |
| `format` | The content format version; currently `1` |
| `canvas` | The logical coordinate space for every scene |
| `scenes` | Named arrangements of backgrounds, text, and images |
| `schedule` | Which scene to select at midnight and at later daily times |
| Image files | Static PNG or JPEG assets referenced by scenes |

The server validates and stores the package, then selects a scene. The browser receives that scene and its images together. Scheduling belongs to the server; the browser concentrates on preparing and displaying the result.

## 2. Your First Scene

Save this complete package as `content.json`:

```json
{
  "format": 1,
  "canvas": { "width": 1920, "height": 1080 },
  "schedule": {
    "utcOffset": "+09:00",
    "defaultScene": "welcome",
    "daily": []
  },
  "scenes": {
    "welcome": {
      "background": { "color": "#183828" },
      "elements": [
        {
          "type": "text",
          "x": 120, "y": 120,
          "width": 1680, "height": 200,
          "text": "Hello, Glypha!",
          "fontSize": 96,
          "color": "#FFFFFF",
          "align": "left"
        }
      ]
    }
  }
}
```

This displays white text on a dark-green background. An empty `daily` array means the default scene stays selected throughout the day.

The shipped application uses a **1920×1080 logical canvas** and rejects other canvas dimensions. Coordinates start at the upper-left corner: `x` increases to the right and `y` downward. Every element occupies a rectangle within that canvas.

The browser scales the whole canvas to the available display area, preserving its aspect ratio and filling unused space with black. It does not rearrange elements or reflow text for a smaller screen. Choosing a different logical canvas currently requires an application change; it is not a content-level option.

## 3. Text Is Plain Text

Text supports explicit line breaks and horizontal alignment. For example, replace the first scene's `elements` array with:

```json
[
  {
    "type": "text",
    "x": 120, "y": 240,
    "width": 1680, "height": 300,
    "text": "Welcome\nToday's special: seasonal coffee",
    "fontSize": 64,
    "color": "#F5E6C8",
    "align": "center"
  }
]
```

Use `left`, `center`, or `right` for `align`. Text starts at the top of its rectangle; centering text horizontally does not center it vertically. Adjust the rectangle's position to place it lower on the screen.

The bundled font is **Noto Sans JP Regular (400)**. English and Japanese text are supported when the font contains the required glyphs. UTF-8 input does not mean every Unicode character is available: unsupported characters are rejected rather than replaced with another font.

There is no automatic line wrapping or shrinking. Insert `\n` where you want a new line and leave sufficient room in both dimensions. Line spacing is 1.2 times `fontSize`, and actual glyph bounds must also fit. Oversized text is rejected rather than silently clipped.

Text has no Markdown, HTML, font selection, bold, or italic setting. If you need a particular logo or typographic treatment, prepare it as a static image outside Glypha.

## 4. Add Images and Layers

Place a static PNG or JPEG alongside your JSON. Add an image element to the scene's `elements` array:

```json
{
  "type": "image",
  "x": 1400, "y": 680,
  "width": 400, "height": 280,
  "asset": "logo.png",
  "fit": "contain"
}
```

`asset` refers to an uploaded filename, not a URL or a path on the server.

| Fit | Result |
| --- | --- |
| `contain` | Preserve the image's aspect ratio and show the whole image, centered within the rectangle |
| `cover` | Preserve the aspect ratio and fill the rectangle, cropping overflow at its edges |

A background can also have an image. Replace a scene's `background` with:

```json
{
  "color": "#182028",
  "image": "backdrop.jpg",
  "fit": "cover"
}
```

The background color is drawn first, then its optional image, then elements in array order. Later elements appear above earlier ones. Overlapping rectangles are allowed, but every rectangle must stay inside the canvas.

Colors use opaque `#RRGGBB` values. There is no element-opacity field or gradient primitive. PNG transparency can be part of the image itself.

Image bytes are decoded and checked; renaming a GIF to `.png` does not make it supported. Animated PNG, GIF, WebP, SVG, video, remote image URLs, and uploaded fonts are outside the current format. Assets may be shared by several scenes, but every reference must resolve and every uploaded image must be used somewhere in the package.

## 5. Give the Day a Schedule

A package can hold several scenes while displaying one selected scene at a time. Suppose you define scenes named `closed`, `open`, and `afternoon`. The following schedule selects them each day:

```json
{
  "utcOffset": "+09:00",
  "defaultScene": "closed",
  "daily": [
    { "at": "10:00:00", "scene": "open" },
    { "at": "13:00:00", "scene": "afternoon" },
    { "at": "18:00:00", "scene": "closed" }
  ]
}
```

| Local time | Selected scene |
| --- | --- |
| Midnight to before 10:00 | `closed` |
| 10:00 to before 13:00 | `open` |
| 13:00 to before 18:00 | `afternoon` |
| 18:00 to midnight | `closed` |

Every midnight resets selection to `defaultScene`. Do not add an explicit `00:00:00` entry. Other times must use `HH:MM:SS`, and two entries cannot share a time. Input order does not matter: accepted entries are sorted by time.

The offset is fixed and independent of the server's operating-system timezone. `+09:00` means UTC plus nine hours all year. Named timezones, daylight-saving changes, weekdays, holidays, calendar dates, and one-time future publication are unsupported.

The [daily example](../examples/daily/content.json) is a complete package implementing this schedule. It needs no image files.

## 6. Publish the Whole Package

Start Glypha using the [README](../README.md#run-the-built-application), then upload your first package:

```sh
curl --fail-with-body -X PUT http://localhost:8080/content \
  -F 'content=@content.json;type=application/json'
```

For a package that references `logo.png` and `backdrop.jpg`, upload both with the JSON:

```sh
curl --fail-with-body -X PUT http://localhost:8080/content \
  -F 'content=@content.json;type=application/json' \
  -F 'assets=@logo.png' \
  -F 'assets=@backdrop.jpg'
```

Only send images the package references. Each image part's filename must match the corresponding asset ID exactly.

A successful upload returns `200`, a publication generation, and the initially selected scene. The server selects the scene applicable at publication time, so publishing the daily example at 14:00 selects `afternoon` immediately on the server. The browser adopts it after successful retrieval and preparation.

Open [http://localhost:8080](http://localhost:8080) to see the display. `GET /display` exposes the selected scene envelope for inspection; before any package has been accepted, it returns `204` and the loaded renderer uses its built-in startup scene.

Each accepted upload replaces all scenes, assets, and scheduling rules together. There is no patch endpoint, draft area, content-history browser, or undo command. Keep source packages in Git or another external store, and upload an earlier package if you want to restore its content. That upload is a new publication, with selection evaluated at its publication time.

## 7. Validation Is Part of Authoring

Glypha rejects ambiguous or unsupported input. Unknown fields, duplicate JSON keys, missing scene references, duplicate asset filenames, unused images, and rectangles outside the canvas are errors.

For example, a semantic validation response can identify a bad reference:

```json
{
  "error": {
    "code": "validation_failed",
    "issues": [
      {
        "path": "/schedule/daily/0/scene",
        "code": "unknown_scene",
        "message": "The referenced scene does not exist."
      }
    ]
  }
}
```

Read the issue's `path` and `code`, correct the source, and submit the whole package again. Rejected validation does not replace the stored package. Malformed input can return `400`, semantic errors `422`, and resource-limit violations `413`; not every failure is a content error. The [HTTP reference](../design/http-and-rendering.md#upload-results) explains the other responses.

Useful format limits include:

| Data | Current limit or rule |
| --- | --- |
| Scenes | 1–64 per package |
| Elements | At most 128 per scene |
| Explicit daily transitions | At most 64, plus implicit midnight |
| JSON document | At most 1 MiB |
| JSON plus image file bytes | At most 32 MiB; HTTP transfer limits also apply |
| Decoded image pixels | At most 32,000,000 across uploaded assets |
| Text | At most 16,384 Unicode code points per element |
| Font size | Integer from 1 through 4096, subject to fitting in the box |
| Rectangles | Integer coordinates; nonnegative origin, positive size, entirely inside the canvas |
| Scene and asset IDs | 1–128 ASCII letters, digits, `_`, `-`, or `.`; neither `.` nor `..` alone |

These are acceptance limits, not a promise that every device can render every accepted package. The browser performs its own preparation checks before replacing a frame.

## 8. What Happens When Time or Communication Goes Wrong?

Glypha favors a complete, successful frame over a partially updated one. It prepares the next scene separately, swaps it into view only on success, then releases the previous scene's resources.

| Situation | Behavior |
| --- | --- |
| An upload fails validation | Keep the current server package |
| A running renderer loses contact with the server | Keep its last successful scene and retry |
| An image cannot be decoded or a candidate cannot be rendered | Keep the active scene; report diagnostics outside the signage surface |
| Several transitions pass between requests | Select the latest due scene; skip intermediate scenes |
| The server clock moves backward | Do not replay consumed transitions; keep the current target until a later unconsumed occurrence becomes due |
| The server restarts | Recover its durable package, target, and scheduling cursor |
| The browser reloads or restarts | Start from Builtin once the application loads, then retrieve content again; the previous frame is not persisted by the renderer |

A server response already in flight may still display after the server has selected a newer scene. Subsequent successful polling catches up. Switching is not synchronized to an exact screen refresh, and multiple browsers are not guaranteed to change at the same instant.

Clock rollback protection also has a visible consequence: after a large forward jump followed by clock correction, the advanced target may remain until time passes the consumed boundary or a new package is published. Glypha does not reconstruct historical display state.

Retaining a frame requires the browser and its graphics resources to remain alive. It does not protect against power loss, browser crashes, display disconnection, or hardware failure. A fresh browser also needs access to the application files; Glypha is not an offline application installer.

## 9. Where Glypha Stops

Glypha is suited to a welcome sign, daily opening/closing notice, static menu, or announcement board whose content is prepared elsewhere.

| Need | Current boundary |
| --- | --- |
| Text and still-image layouts | Supported through JSON scenes |
| Daily time-based changes | Supported with a fixed UTC offset |
| Video, audio, animation, or slide transitions | Unsupported |
| Clickable content, forms, HTML, CSS, or JavaScript | Unsupported in content packages |
| Weather, news, database queries, or live variables | No built-in integration or template engine; an external tool can generate and upload replacement content |
| Visual editing and content approval | No editor, administration UI, or approval workflow |
| Per-device playlists and fleet management | No device registry or per-display targeting API; browsers using the same server follow its current target |
| Public service with user accounts | No built-in authentication, user management, or TLS termination; deployment is intended for loopback or a trusted environment |
| Proof that viewers saw a scene | No display acknowledgment or playback-history API |

An AI agent can help write packages and interpret validation errors, but AI is external tooling rather than a requirement for running Glypha. Humans, scripts, and agents use the same HTTP interface.

## Continue Exploring

- [Running Glypha with an AI Agent](using-ai-agents.md): prompts for startup, content creation, and previewing.
- [Content and scheduling](../design/content-and-scheduling.md): precise format and time semantics.
- [HTTP API and renderer](../design/http-and-rendering.md): publication, responses, and display behavior.
- [Operations](../ops/README.md): updates, backups, and recovery.
- [Implementation status](../RELEASE_STATUS.md): completed checks and deferred work, including target-device trials and CI.
