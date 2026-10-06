# Setting Up Playwright for Glypha on macOS and Windows WSL

## About This Guide

An optional `e2e/` directory can hold Playwright tests for health checks, display inspection, screenshots, and automated execution of the renderer's 25 browser checks. Playwright MCP can also let an AI agent open Glypha in a browser and inspect its appearance.

This guide covers two uses:

| Use | Capabilities | Sections |
| --- | --- | --- |
| Automated tests | Check server health and rendering, save screenshots, and run `renderer/tests/` checks | 2–3 |
| Browser access for an AI agent | Let Claude Code or Codex inspect the display and help revise content | 4 |

The target environments are macOS and WSL2 with Ubuntu. The supplied source guide reports building Glypha and passing four tests on Linux with Node 22 and Playwright 1.56, and taking a screenshot through Playwright MCP. It does not report testing on macOS or WSL hardware. Those are source observations, not verification performed while adding this English guide.

The steps below are optional setup instructions. The repository does not include the proposed `e2e/` suite or MCP configuration merely because this guide is present. CI adoption and target-device trials remain deferred; see [implementation status](../RELEASE_STATUS.md).

Keep Playwright in a separate `e2e/` package so that the renderer's pinned dependencies in `renderer/package.json` and the Docker workflow remain unchanged. The examples below make small adjustments to the source guide: they configure an HTML report, allow enough time for the renderer checks, and wait for a published scene canvas instead of a fixed delay.

## 1. Prerequisites and Starting Glypha

Playwright runs on Node.js. For the host workflow, install the Go and Node versions used by Glypha as well.

| Requirement | macOS | WSL2 with Ubuntu |
| --- | --- | --- |
| Go 1.26 or newer | `brew install go` | Install the Linux archive from [Go downloads](https://go.dev/dl/); distribution packages may provide an older version |
| Node 24, recommended | `brew install node@24`, or use nvm | With nvm installed, run `nvm install 24` |
| Git | Xcode Command Line Tools | `sudo apt install git` |

In WSL, keep the repository in the Linux filesystem, such as under `~/`, rather than `/mnt/c/`. Working across the Windows filesystem boundary can slow dependency installation and tests.

Start Glypha using the host workflow from the [README](../README.md):

```sh
git clone https://github.com/kuny/glypha.git ~/glypha
cd ~/glypha/renderer
npm ci
npm run build
cd ..
go run ./cmd/glypha          # Leave this terminal running.
```

In another terminal, check server health and publish the sample package. This replaces the server's current content, so use a development instance:

```sh
cd ~/glypha
curl -i http://localhost:8080/healthz        # Expect 200 and {"status":"ok"}.
curl --fail-with-body -X PUT http://localhost:8080/content \
  -F 'content=@examples/daily/content.json;type=application/json'
```

To automate the renderer checks in section 3, also start Vite in a separate terminal:

```sh
cd ~/glypha/renderer
npm run dev                               # http://localhost:5173
```

WSL2 normally forwards localhost services so that a Windows browser can open `http://localhost:8080`; the exact behavior depends on the WSL networking setup. With Docker, the production application exposes port 8080, while the development environment also exposes Vite on 5173. Use the corresponding [Docker instructions](../README.md#develop-in-docker) instead of starting duplicate services on those ports.

## 2. Install Playwright

Create a separate package at the repository root:

```sh
cd ~/glypha
mkdir -p e2e
cd e2e
npm init -y
npm install -D @playwright/test
```

Install Chromium first for the browser tests:

| Platform | Command | Notes |
| --- | --- | --- |
| macOS | `npx playwright install chromium` | Installs the test browser |
| WSL2 with Ubuntu | `npx playwright install --with-deps chromium` | Also installs Linux system dependencies and may request sudo access |

Merge these scripts into `e2e/package.json`:

```json
{
  "scripts": {
    "test": "playwright test",
    "test:ui": "playwright test --ui",
    "report": "playwright show-report"
  }
}
```

Add generated files to the repository's `.gitignore`:

```gitignore
e2e/node_modules/
e2e/test-results/
e2e/playwright-report/
e2e/screenshots/
```

If you adopt this suite, keep its package manifest and lockfile together so that subsequent installs can use `npm ci`.

Create `e2e/playwright.config.ts`:

```ts
import { defineConfig, devices } from '@playwright/test';

export default defineConfig({
  testDir: './tests',
  timeout: 60_000,
  reporter: [['list'], ['html', { open: 'never' }]],
  use: {
    baseURL: process.env.GLYPHA_URL ?? 'http://localhost:8080',
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
  },
  projects: [
    {
      name: 'chromium',
      use: {
        ...devices['Desktop Chrome'],
        viewport: { width: 1280, height: 720 },
      },
    },
  ],
});
```

This configuration connects to servers you start separately. If you later use Playwright's `webServer` option to start Glypha automatically, run the server from the repository root and set `GLYPHA_DB_PATH` to a dedicated test database rather than your usual `data/glypha.db`.

## 3. Write and Run the First Tests

Create `e2e/tests/` and add the following three files. Together they define four tests. The source guide reports passing its original four tests; the adjusted examples below have not been executed as part of this documentation change.

### `tests/display.spec.ts`: Health and Basic Rendering

```ts
import { test, expect } from '@playwright/test';

test('the server responds successfully', async ({ request }) => {
  const res = await request.get('/healthz');
  expect(res.status()).toBe(200);
  expect(await res.json()).toEqual({ status: 'ok' });
});

test('the home page displays a rendering canvas', async ({ page }) => {
  await page.goto('/');
  // Successful PixiJS initialization adds a canvas and hides the HTML fallback.
  await expect(page.locator('#display canvas')).toBeVisible({ timeout: 30_000 });
  await expect(page.locator('#builtin')).toBeHidden();
  await page.screenshot({ path: 'screenshots/top.png' });
});
```

### `tests/content.spec.ts`: Capture Published Content

```ts
import { test, expect } from '@playwright/test';

test('published content is displayed', async ({ page, request }) => {
  const display = await request.get('/display');
  expect(display.status(), 'Unpublished content returns 204.').toBe(200);

  // Register before navigation so the initial response cannot be missed.
  const fetched = page.waitForResponse(
    (r) => new URL(r.url()).pathname === '/display' && r.status() === 200,
  );
  await page.goto('/');
  await fetched;
  // This label belongs to a prepared content scene, not the startup canvas.
  await expect(page.locator('#display canvas[aria-label="Glypha scene"]'))
    .toBeVisible({ timeout: 30_000 });
  await page.screenshot({ path: 'screenshots/content.png' });
});
```

This checks that a published scene reaches the display. It does not prove that a particular generation or exact pixel layout was displayed; inspect the screenshot or add explicit visual assertions for your content.

### `tests/renderer-checks.spec.ts`: Automate the Browser Checks

The development-only `/tests/` page is served by Vite, not by the production build. This test automates opening it and selecting **Run checks**.

```ts
import { test, expect } from '@playwright/test';

test('all renderer browser checks pass', async ({ page }) => {
  test.setTimeout(150_000);
  const devURL = process.env.GLYPHA_DEV_URL ?? 'http://localhost:5173';
  await page.goto(new URL('/tests/', devURL).toString());
  await page.getByRole('button', { name: 'Run checks' }).click();
  const status = page.locator('#status');
  // The final status is currently "PASS: 25 passed, 0 failed" on success.
  await expect(status).toHaveText(/^(PASS|FAIL):/, { timeout: 120_000 });
  await page.screenshot({ path: 'screenshots/renderer-checks.png', fullPage: true });
  await expect(status).toHaveAttribute('data-result', 'pass');
  await expect(status).toHaveText('PASS: 25 passed, 0 failed');
});
```

Update the expected count deliberately if the repository adds or removes checks.

Run from `e2e/`:

```sh
cd ~/glypha/e2e
npx playwright test                 # Run all tests headlessly.
npx playwright test --headed        # Watch the browser run.
npx playwright test --ui            # Explore tests in UI mode.
npx playwright show-report          # Open the generated HTML report.
```

The source guide recorded `4 passed (10.8s)` for its original examples. With the daily package published, `screenshots/content.png` shows the scene selected at the current time, such as “Welcome.” The checks screenshot shows the final status and test frame.

Headless Chromium can use software graphics rendering, but WebGL availability depends on the environment and browser configuration. A passing software-rendered test does not qualify a physical device or GPU.

For automatic visual comparison, use `await expect(page).toHaveScreenshot()`. Use content with no time-dependent scene changes and maintain appropriate baselines for each operating system, since font rasterization can differ between macOS and Linux.

## 4. Let an AI Agent Operate the Browser with Playwright MCP

Playwright MCP provides browser actions such as opening pages, clicking, capturing screenshots, and inspecting console output. It can add visual inspection to the workflow in [Running Glypha with an AI Agent](using-ai-agents.md).

The MCP browser setup is separate from the `e2e/` test runner. If you only want agent-driven browsing, you do not need to create the test suite in sections 2–3, but the MCP server still needs a compatible browser and its system dependencies.

### Register the Server

The following are setup examples carried over from the source guide. Agent commands, supported platforms, and browser installation requirements may vary by installed version; consult the tools' current documentation when setting them up.

Run from the Glypha repository root:

| Agent | Registration | Inspection |
| --- | --- | --- |
| Claude Code | `claude mcp add playwright -- npx @playwright/mcp@latest` | `claude mcp list`, or `/mcp` in a session |
| Codex CLI | Add the TOML block below to `~/.codex/config.toml` | `/mcp` in a session |

```toml
[mcp_servers.playwright]
command = "npx"
args = ["@playwright/mcp@latest"]
```

Claude Code's `--scope project` option stores project configuration in `.mcp.json` for sharing. These examples use `@latest`; teams that need repeatable setup should select and record a tested MCP version.

### Platform Considerations

The source workflow uses a headed Chrome browser. With a local MCP process inside WSL, install and run the browser inside WSL as well; using Windows Chrome requires a separate connection arrangement, not this basic configuration.

| Topic | macOS | WSL2 with Ubuntu |
| --- | --- | --- |
| Browser | Use an installed compatible Chrome browser | The source guide uses `npx playwright install --with-deps chrome` inside WSL |
| Visible window | A local browser window opens | Requires working GUI support such as WSLg; otherwise use headless mode |
| Headless option | Add `--headless` to the MCP arguments if desired | Use `--headless` when GUI support is unavailable |

Useful MCP options include `--headless`, `--isolated` for a separate session profile, `--viewport-size 1920x1080`, and `--output-dir` for output files. For example:

```sh
claude mcp add playwright -- npx @playwright/mcp@latest --headless --isolated --viewport-size 1920x1080
```

### Example Prompts

```text
Use Playwright MCP to open http://localhost:8080 and take a screenshot
of the displayed scene. Check for clipped text and verify the layout
and colors against the requirements. If necessary, revise content.json,
upload it again, and inspect the result once more.
```

```text
The renderer development server is running.
Use Playwright MCP to open http://localhost:5173/tests/, select Run checks,
and verify that the result is "PASS: 25 passed, 0 failed".
If anything fails, report the check name and relevant console errors.
```

The source guide reports successfully opening Glypha and capturing a screenshot through Playwright MCP. It also observed a `favicon.ico` 404 and a WebGL `GPU stall due to ReadPixels` warning without a visible rendering problem. Those messages alone do not establish a content failure; investigate them in context rather than automatically modifying the application.

## 5. Troubleshooting

| Symptom | Likely cause | Action |
| --- | --- | --- |
| `Executable doesn't exist` and a request to install browsers | Browser binaries do not match the installed Playwright version | Run `npx playwright install chromium` in `e2e/`; add `--with-deps` on WSL when needed |
| Missing libraries such as `libnss3.so` | Linux system dependencies are absent | Run `npx playwright install-deps chromium` and authorize system dependency installation if prompted |
| Headed tests or MCP cannot open a window in WSL | GUI support is unavailable | Use headless mode and inspect screenshots or the HTML report |
| The report does not open in a Windows browser | Browser launch from WSL failed | Copy the printed report URL, usually `http://localhost:9323`, into the Windows browser |
| `content.spec.ts` receives `204` | No content has been published | Upload the sample package to the development instance |
| Renderer checks get `ERR_CONNECTION_REFUSED` | Vite is not running or the URL differs | Start `npm run dev` in `renderer/`, use Docker development, or set `GLYPHA_DEV_URL` |
| Application tests cannot connect | Glypha is stopped or uses another port | Check `/healthz`; override the URL with `GLYPHA_URL=http://localhost:PORT npx playwright test` |
| Dependency installation or tests are unusually slow in WSL | Repository files are under `/mnt/c/` | Clone into the Linux filesystem, such as `~/glypha` |
| A failure is hard to diagnose | More execution context is needed | Open a saved trace with `npx playwright show-trace <path-to-trace.zip>` |

Server and renderer diagnostics appear in logs or the browser console. When candidate preparation fails, Glypha retains the active scene. If the screen does not change after an upload, inspect the upload response first to see whether the package was accepted.
