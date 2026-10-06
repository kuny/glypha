# Running Glypha with an AI Agent

## Introduction

You can ask Claude Code or Codex, in natural language, to start Glypha and create a display. The agent can handle startup, content creation, upload, and verification. This guide describes the workflow with prompts you can reuse. For manual startup instructions and implementation details, see the [README](../README.md) and [design documentation](../design/README.md).

Glypha content consists of one JSON file (`content.json`) and, optionally, PNG/JPEG images. The JSON describes the canvas dimensions, scenes (backgrounds and text/image placement), and daily switching times. Validation is strict: the server rejects out-of-bounds elements and characters absent from the font, with an explanation. An agent can help with the cycle of writing content, submitting it, reading errors, and making corrections.

| Agent | Provider | Usage in this guide |
| --- | --- | --- |
| Claude Code | Anthropic | Build content through conversation, using images or design references as input |
| Codex CLI | OpenAI | Use an interactive session, or submit a task with `codex exec` |

The workflow is the same for both. Examples use Claude Code, with Codex alternatives where relevant.

## 1. Preparation

You need an agent and a runtime environment for Glypha: Go and Node, or Docker. You can ask the agent to help set up the runtime, but install the agent itself first.

| Requirement | Installation | Notes |
| --- | --- | --- |
| Claude Code | `curl -fsSL https://claude.ai/install.sh \| bash`, or `npm install -g @anthropic-ai/claude-code` | Sign in when first running `claude` |
| Codex CLI | `npm install -g @openai/codex` | Sign in when first running `codex` |
| Go 1.26 or newer and Node | Official installers or a package manager such as Homebrew | The project recommends Node 24. The source guide reports a successful build with Node 22 |
| Docker, as an alternative | Docker Desktop or Docker Engine with Compose v2 | Host Go and Node are unnecessary. The source guide's walkthrough used the host workflow; see the repository's documented Docker workflow |

Clone the repository and start the agent from its root so that it uses the repository as its working directory:

```sh
git clone https://github.com/kuny/glypha.git
cd glypha
claude        # For Codex, use codex
```

## 2. Ask the Agent to Start Glypha

Once the agent is running, give it a prompt such as the following. Depending on its permission settings, it may ask for approval before executing commands. Review those commands before approving them.

```text
Read README.md and start Glypha locally without Docker.
Follow these steps:
1. Run npm ci and npm run build in renderer/.
2. Run go run ./cmd/glypha in the background from the repository root.
3. Verify that curl http://localhost:8080/healthz returns {"status":"ok"}.
If Go or Node is missing, suggest how to install it.
```

After startup, open [http://localhost:8080](http://localhost:8080) in a browser. Until content is published, `/display` returns `204` with no body.

For Docker, ask the agent to follow **Run the built application** in the [README](../README.md#run-the-built-application), which uses `docker compose up --build -d`.

Keep these points in mind:

- Explicitly request background execution because the server keeps running. Otherwise, the agent may wait for the process to finish. You can also run `go run ./cmd/glypha` yourself in another terminal.
- The host workflow stores content in `data/glypha.db`, which survives restarts. Docker uses a persistent volume instead.
- Agent sandbox or network restrictions may prevent `npm ci` or Go dependency downloads. Approve the specific operation through the agent's permission mechanism, or run that command yourself.

## 3. Create Content with the Agent

Describe the display you want and ask the agent to create the JSON, upload it, and correct validation errors. Point it to the format documentation so that it can follow the actual rules.

### Workflow

1. If you use images, place PNG or JPEG files in a working directory inside the repository, such as `content/cafe/`. Animated PNG, GIF, and WebP are unsupported.
2. Give the agent a prompt such as the following.
3. The agent writes `content.json` and uploads it with `curl`. If the server rejects it, the agent reads the errors and revises the content.
4. Open [http://localhost:8080](http://localhost:8080) and inspect the result.

```text
Create and publish content for Glypha.
Read design/content-and-scheduling.md and examples/daily/content.json,
and follow their format rules.

Requirements:
- A cafe storefront sign, 1920x1080, Japan time (+09:00).
- From 10:00 to 19:00, show “Welcome” and today's recommendation,
  with content/cafe/logo.png in the upper-right corner.
- Outside those hours, show “We are closed for today” with centered text.
- Use a calm dark-green and cream palette.

Save the result to content/cafe/content.json and upload it to PUT /content
using the curl pattern in README.md.
If the server returns 422, inspect each issue's path and code, correct the
content, and retry until it returns 200.
Finally, use GET /display to tell me which scene is currently selected.
```

### Example JSON

The following example uses Japanese display text to demonstrate Japanese font support; the prompts and documentation are in English. The source guide reports that this JSON, together with a 400×400 PNG, was accepted with status `200` and displayed Japanese text successfully.

```json
{"format":1,"canvas":{"width":1920,"height":1080},
 "schedule":{"utcOffset":"+09:00","defaultScene":"closed",
   "daily":[{"at":"10:00:00","scene":"open"},{"at":"19:00:00","scene":"closed"}]},
 "scenes":{
  "open":{"background":{"color":"#1E3A2F"},"elements":[
    {"type":"text","x":120,"y":120,"width":1200,"height":200,"text":"いらっしゃいませ","fontSize":120,"color":"#FFFFFF","align":"left"},
    {"type":"text","x":120,"y":400,"width":1200,"height":300,"text":"本日のおすすめ\n季節のブレンド 480円","fontSize":80,"color":"#F5E6C8","align":"left"},
    {"type":"image","x":1400,"y":120,"width":400,"height":400,"asset":"logo.png","fit":"contain"}]},
  "closed":{"background":{"color":"#182028"},"elements":[
    {"type":"text","x":120,"y":120,"width":1680,"height":200,"text":"本日の営業は終了しました","fontSize":100,"color":"#FFFFFF","align":"center"}]}}}
```

Upload it with the following command. The uploaded image filename must match the name in `asset`.

```sh
cd content/cafe
curl --fail-with-body -X PUT http://localhost:8080/content \
  -F 'content=@content.json;type=application/json' \
  -F 'assets=@logo.png'
```

### Rules Worth Including in the Prompt

These rules are documented in the repository, but repeating them in a prompt can reduce validation errors.

| Rule | Details |
| --- | --- |
| No automatic wrapping | Text that does not fit its box is rejected. Insert explicit `\n` line breaks. Line height is 1.2 times the font size |
| Boxes stay within the canvas | `x`, `y`, `width`, and `height` are integers, and the box must not extend beyond the canvas |
| Colors | Only opaque `#RRGGBB` colors are supported |
| Font | Noto Sans JP Regular only. Characters absent from the font, including unsupported emoji, are rejected |
| Images | Static PNG/JPEG only. Uploaded images not referenced by the JSON are also rejected |
| Schedule | Times use `HH:MM:SS`. Use `defaultScene` instead of an entry at `00:00:00`. Weekday and holiday rules are unsupported |
| Replacement | Every upload replaces the entire package. A rejected upload preserves the previous content |

## 4. Make Verification and Repeated Updates Easier

### Preview Any Scene Immediately

The display shows the scene selected for the current time. To inspect another scene, ask the agent to upload a temporary copy with that scene as `defaultScene` and an empty `daily` array. Because this replaces the whole package, restore the intended scheduled package afterward.

```text
For previewing, create a copy in /tmp that always displays the open scene,
and upload it with the required assets.
When I say “OK,” upload content/cafe/content.json with its assets again.
```

An agent with browser tools can also inspect the layout. For example, ask it to open `http://localhost:8080` at 1920×1080, take a screenshot, and check text placement, spacing, and colors. This requires browser automation support, such as Playwright or the agent's available browser tools.

### Avoid Repeating the Rules

Repository instruction files can provide recurring context: Claude Code uses `CLAUDE.md`, and Codex uses `AGENTS.md`. You can ask an agent to create them with guidance such as the following. If an instruction file already exists, incorporate the guidance without replacing unrelated instructions.

```markdown
# Glypha Content Authoring Rules

- Startup: run npm ci and npm run build in renderer/, then run go run ./cmd/glypha in the background from the repository root.
- Format: follow design/content-and-scheduling.md; use examples/daily/content.json as a reference.
- Keep content/<name>/content.json and its images in the same directory.
- Use a 1920x1080 canvas and utcOffset +09:00.
- Publish with curl --fail-with-body -X PUT http://localhost:8080/content -F 'content=@content.json;type=application/json' -F 'assets=@<image>'.
- On 422, inspect each issue's path and code, correct the content, and retry until accepted with 200.
- Avoid emoji. Leave room inside text boxes and use explicit \n line breaks.
- Do not modify Go or renderer source code for content-authoring tasks.
```

With that context, a request can be as brief as “Prepare an autumn specials version for tomorrow.” Glypha only supports recurring daily schedules; an agent must not represent a one-time future publication as a supported schedule feature.

### Run a Single Noninteractive Task

Routine updates can also be requested from the command line. File-editing and execution permissions may still require intervention.

```sh
claude -p "Change the recommendation in content/cafe to 'Chestnut Mont Blanc, 620 yen' and publish it."
codex exec "Change the recommendation in content/cafe to 'Chestnut Mont Blanc, 620 yen' and publish it."
```

## 5. Troubleshooting and Operational Notes

When an upload is rejected, the server returns `422` with issue paths and codes. For example, `{"path":"/scenes/open/elements/0/text","code":"invalid_text_layout"}` indicates an invalid layout for the first text element in `open`, such as text that does not fit. Give the error to the agent so it can investigate and correct the content.

| Symptom | Common cause | Action |
| --- | --- | --- |
| `invalid_text_layout` | Text is too large or its box is too small | Enlarge the box or reduce `fontSize`. Allow at least line count × font size × 1.2 in height, and leave room for actual glyph bounds |
| `missing_asset` | The JSON asset name differs from the uploaded filename, or an image was omitted | Add `-F 'assets=@filename'` and align the names |
| Unsupported-character error | Emoji or symbols absent from the font | Use another character or an image |
| Scene does not switch at the expected time | Incorrect `utcOffset` | Check the offset; Japan uses `+09:00` |
| No published content appears | No upload has succeeded yet | A `204` response from `curl -i http://localhost:8080/display` indicates no published package |
| `curl` cannot connect | The server is stopped | Ask the agent to check `/healthz` and start Glypha if needed |

Before publishing to an operating sign, verify the destination URL. An accepted upload changes the server's current package, and the renderer adopts the selected scene after its next successful fetch and preparation.

Content authoring does not require changes to Go or renderer source code. If the agent starts changing application code, redirect it to the content task. A fresh conversation can help when changing topics.

### Verification Scope

The supplied source guide reports testing the host startup procedure and the Japanese JSON example without Docker. It does not report testing the actual Claude Code or Codex conversations. Those reports are retained here as source observations, not new verification performed during translation. For the project's own completed checks and deferred work, see [implementation status](../RELEASE_STATUS.md).
