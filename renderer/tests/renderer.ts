import { Application, Container, Sprite, Texture } from 'pixi.js';
import { Display, pollDisplay, waitForRetry } from '../src/display';
import { prepareScene, type Prepared } from '../src/scene';
import { loadDisplayFont } from '../src/font';

const button = document.querySelector<HTMLButtonElement>('#run')!;
const status = document.querySelector<HTMLElement>('#status')!;
const results = document.querySelector<HTMLOListElement>('#results')!;
const host = document.querySelector<HTMLElement>('#preview')!;
let display: Display | undefined;
const assert: (condition: unknown, message: string) => asserts condition = (condition, message) => {
  if (!condition) throw new Error(message);
};
async function rejects(action: () => Promise<unknown>, message?: string): Promise<void> {
  let error: unknown;
  try { await action(); } catch (caught) { error = caught; }
  assert(error instanceof Error, 'Expected rejection.');
  if (message) assert(error.message.includes(message), `Unexpected rejection: ${error.message}`);
}
function gate() {
  let release!: () => void;
  const promise = new Promise<void>((resolve) => { release = resolve; });
  return { promise, release };
}
function frame(color: string): Prepared & { disposals: number } {
  const canvas = document.createElement('canvas'); canvas.width = 1920; canvas.height = 1080;
  const context = canvas.getContext('2d')!; context.fillStyle = color; context.fillRect(0, 0, 1920, 1080);
  return { canvas, disposals: 0, dispose() { this.disposals++; canvas.remove(); } };
}
function initial(): ReturnType<typeof frame> {
  display?.dispose(); display = undefined; host.replaceChildren();
  const first = frame('#183828'); host.appendChild(first.canvas); return first;
}
const response = (value: unknown, tag = 'candidate') => new Response(JSON.stringify(value), { headers: { ETag: `"${tag}"` } });
const signal = () => new AbortController().signal;
const png = 'iVBORw0KGgoAAAANSUhEUgAAAAIAAAACCAIAAAD91JpzAAAAFElEQVR4nGP4z8DAAMIM////ZwAAHu8E/KPItPcAAAAASUVORK5CYII=';
async function asset(base64 = png) {
  const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
  const digest = await crypto.subtle.digest('SHA-256', bytes);
  return { mediaType: 'image/png', base64, sha256: Array.from(new Uint8Array(digest), (b) => b.toString(16).padStart(2, '0')).join('') };
}
async function envelope(label = 'Recovered: English and 日本語') {
  return {
    protocol: 1, generation: label, scene: 'sample',
    ast: { version: 1, profile: 'noto-sans-jp-2.004-regular-400-v1', canvas: { width: 1920, height: 1080 },
      background: { color: '#182028' }, elements: [
        { type: 'text', x: 100, y: 100, width: 1720, height: 160, text: label, fontSize: 64, color: '#FFFFFF', align: 'left' },
        { type: 'image', x: 100, y: 300, width: 400, height: 400, asset: 'tile.png', fit: 'contain' },
      ] },
    assets: { 'tile.png': await asset() },
  };
}

button.addEventListener('click', () => { void run(); });
async function run(): Promise<void> {
  button.disabled = true; results.replaceChildren(); status.textContent = 'Running';
  let passed = 0, failed = 0;
  const check = async (name: string, test: () => Promise<void>) => {
    const item = document.createElement('li'); results.appendChild(item);
    try { await test(); passed++; item.dataset.result = 'pass'; item.textContent = `PASS: ${name}`; }
    catch (error) { failed++; item.dataset.result = 'fail'; item.textContent = `FAIL: ${name}: ${String(error)}`; }
    status.textContent = `Running: ${passed} passed, ${failed} failed`;
  };
  try {
    await check('Swap commits the frame and ETag before releasing the old frame', async () => {
      const old = initial(); const next = frame('#283848');
      old.dispose = () => {
        assert(host.firstChild === next.canvas && display?.etag === '"new"', 'Old frame released before swap.');
        old.disposals++;
      };
      display = new Display(host, old, async () => next);
      await display.accept(response({}, 'new'), signal());
      assert(old.disposals === 1 && next.disposals === 0, 'Wrong resource ownership.');
      await display.accept(new Response(null, { status: 304 }), signal());
      await display.accept(new Response(null, { status: 204 }), signal());
      assert(host.firstChild === next.canvas && next.disposals === 0, 'Empty or conditional response changed frame.');
      display.dispose(); display.dispose(); assert(Number(next.disposals) === 1, 'Repeated disposal released twice.');
    });
    await check('A failed DOM swap disposes only the candidate', async () => {
      const old = initial(); const candidate = frame('#283848');
      display = new Display(host, old, async () => candidate);
      const original = host.replaceChild;
      host.replaceChild = () => { throw new Error('Injected swap failure'); };
      try { await rejects(() => display!.accept(response({}), signal()), 'Injected swap failure'); }
      finally { host.replaceChild = original; }
      assert(host.firstChild === old.canvas && old.disposals === 0 && candidate.disposals === 1 && !display.etag, 'Swap failure changed active state.');
    });
    await check('Abort during preparation releases the late candidate', async () => {
      const old = initial(); const candidate = frame('#283848'); const entered = gate(), release = gate();
      const abort = new AbortController();
      display = new Display(host, old, async () => { entered.release(); await release.promise; return candidate; });
      const attempt = display.accept(response({}), abort.signal);
      await entered.promise; abort.abort(); release.release();
      await rejects(() => attempt);
      assert(host.firstChild === old.canvas && old.disposals === 0 && candidate.disposals === 1 && !display.etag, 'Aborted candidate was adopted.');
    });
    await check('Stopping during preparation cannot resurrect a disposed display', async () => {
      const old = initial(); const candidate = frame('#283848'); const entered = gate(), release = gate();
      display = new Display(host, old, async () => { entered.release(); await release.promise; return candidate; });
      const attempt = display.accept(response({}), signal());
      await entered.promise; display.dispose(); release.release();
      await rejects(() => attempt, 'Display has stopped');
      assert(host.childElementCount === 0 && old.disposals === 1 && candidate.disposals === 1, 'Stopped display was resurrected.');
    });
    await check('Serial polling waits for preparation and keeps only successful ETags', async () => {
      const old = initial(); const entered = gate(), release = gate(); const abort = new AbortController();
      const sent: Array<string | null> = []; let prepares = 0, errors = 0, waits = 0;
      display = new Display(host, old, async () => {
        prepares++;
        if (prepares === 1) { entered.release(); await release.promise; }
        if (prepares === 2) throw new Error('Injected preparation failure');
        return frame('#283848');
      });
      const task = pollDisplay(display, abort.signal, {
        fetch: async (_input, init) => { sent.push(new Headers(init?.headers).get('If-None-Match')); return response({}, `attempt-${sent.length}`); },
        wait: async () => { if (++waits === 3) abort.abort(); }, report: () => { errors++; },
      });
      await entered.promise; await Promise.resolve();
      assert(sent.length === 1 && waits === 0, 'Polling overlapped preparation.');
      release.release(); await task;
      assert(JSON.stringify(sent) === JSON.stringify([null, '"attempt-1"', '"attempt-1"']), 'Failed ETag suppressed retry.');
      assert(errors === 1 && display.etag === '"attempt-3"', 'Polling did not recover.');
    });
    await check('Retry backoff is bounded and resets after successful communication', async () => {
      display = new Display(host, initial()); const abort = new AbortController(); const delays: number[] = []; let requests = 0;
      await pollDisplay(display, abort.signal, {
        fetch: async () => { requests++; return new Response(null, { status: requests === 7 ? 204 : 503 }); },
        wait: async (delay) => { delays.push(delay); if (delays.length === 8) abort.abort(); }, report: () => {},
      });
      assert(JSON.stringify(delays) === JSON.stringify([1000, 2000, 4000, 8000, 15000, 15000, 1000, 1000]), `Wrong backoff: ${delays}`);
      await waitForRetry(15000, abort.signal);
    });
    await check('An unsolicited 304 fails without replacing Builtin', async () => {
      const old = initial(); display = new Display(host, old);
      await rejects(() => display!.accept(new Response(null, { status: 304 }), signal()), 'Unexpected conditional');
      assert(host.firstChild === old.canvas && old.disposals === 0 && !display.etag, 'Unsolicited 304 changed state.');
    });

    const apps: Application[] = [], bitmaps: ImageBitmap[] = [], textures: Texture[] = [];
    let fault: 'none' | 'font' | 'decode' | 'init' | 'render' = 'none';
    const platform = {
      loadFont: async () => { if (fault === 'font') throw new Error('Injected font failure'); await loadDisplayFont(); },
      decodeImage: async (blob: Blob) => { if (fault === 'decode') throw new Error('Injected decode failure'); const bitmap = await createImageBitmap(blob); bitmaps.push(bitmap); return bitmap; },
      createApplication: () => {
        const app = new Application(); apps.push(app);
        const render = app.render.bind(app);
        app.render = () => {
          const visit = (node: Container) => { if (node instanceof Sprite) textures.push(node.texture); for (const child of node.children) visit(child); };
          visit(app.stage);
          if (fault === 'render') throw new Error('Injected render failure');
          render();
        };
        if (fault === 'init') app.init = async () => { throw new Error('Injected init failure'); };
        return app;
      },
    };
    await check('Real PixiJS preparation renders Japanese, English, and an image', async () => {
      display = new Display(host, initial(), (value, s) => prepareScene(value, s, platform));
      await display.accept(response(await envelope('Active: English and 日本語'), 'active'), signal());
      assert(display.etag === '"active"' && host.querySelector('canvas[aria-label="Glypha scene"]'), 'Real frame missing.');
    });
    const retained = host.firstChild;
    const preserve = async (bad: Response, expected?: string) => {
      const counts = [apps.length, bitmaps.length, textures.length];
      try {
        await rejects(() => display!.accept(bad, signal()), expected);
        assert(host.firstChild === retained && display?.etag === '"active"', 'Failed candidate changed visible frame or ETag.');
        assert(apps.slice(counts[0]).every((app) => !app.renderer && (!app.stage || app.stage.destroyed)), 'Candidate application leaked.');
        assert(bitmaps.slice(counts[1]).every((bitmap) => bitmap.width === 0), 'Candidate bitmap leaked.');
        assert(textures.slice(counts[2]).every((texture) => texture.destroyed), 'Candidate texture leaked.');
        assert(apps[0]?.renderer && bitmaps[0]?.width === 2, 'Active resources were destroyed.');
      } finally { fault = 'none'; }
    };
    await check('Malformed JSON retains the active frame', async () => preserve(new Response('{'), 'JSON'));
    await check('Malformed UTF-8 retains the active frame', async () => preserve(new Response(new Uint8Array([255]))));
    await check('HTTP failure retains the active frame', async () => preserve(new Response(null, { status: 503 }), '503'));
    await check('Oversized streaming responses are cancelled and retain the frame', async () => {
      let cancelled = false;
      const chunk = new Uint8Array(1024 * 1024);
      const stream = new ReadableStream<Uint8Array>({ pull(controller) { controller.enqueue(chunk); }, cancel() { cancelled = true; } });
      await preserve(new Response(stream), 'exceeds limit'); assert(cancelled, 'Oversized stream was not cancelled.');
    });
    await check('Unsupported profile retains the active frame', async () => {
      const bad = await envelope(); bad.ast.profile = 'unknown'; await preserve(response(bad), 'Unsupported display profile');
    });
    await check('Asset digest mismatch retains the active frame', async () => {
      const bad = await envelope(); bad.assets['tile.png'].sha256 = '0'.repeat(64); await preserve(response(bad), 'digest mismatch');
    });
    await check('Undecodable PNG with a matching digest retains the active frame', async () => {
      const bad = await envelope();
      // Keep the PNG signature and IHDR, but remove all pixel data.
      bad.assets['tile.png'] = await asset(btoa(atob(png).slice(0, 33)));
      await preserve(response(bad));
    });
    await check('Missing image references release the candidate application', async () => {
      const bad = await envelope(); bad.ast.elements[1].asset = 'missing.png'; await preserve(response(bad), 'Missing image');
    });
    await check('Text overflow releases decoded images and the candidate application', async () => {
      const bad = await envelope(); bad.ast.elements[0].width = 1; await preserve(response(bad), 'Text does not fit');
    });
    for (const stage of ['font', 'decode', 'init', 'render'] as const) {
      await check(`Injected ${stage} failure preserves active resources and releases staging`, async () => {
        const bad = await envelope(); fault = stage; await preserve(response(bad), `Injected ${stage} failure`);
      });
    }
    await check('A later valid frame replaces the retained scene and releases its resources', async () => {
      await display!.accept(response(await envelope(), 'recovered'), signal());
      assert(host.firstChild !== retained && display?.etag === '"recovered"', 'Recovery did not replace frame.');
      assert(!apps[0].renderer && bitmaps[0].width === 0, 'Previous resources were not released.');
    });
    await check('Repeated replacement releases all previous applications, bitmaps, and textures', async () => {
      for (let i = 0; i < 8; i++) {
        const oldCanvas = host.firstChild;
        const counts = [apps.length, bitmaps.length, textures.length];
        await display!.accept(response(await envelope(`Replacement ${i + 1}: English and 日本語`), `repeat-${i}`), signal());
        assert(host.firstChild !== oldCanvas && host.childElementCount === 1, 'Replacement left multiple canvases.');
        assert(apps.slice(0, counts[0]).every((app) => !app.renderer), 'Old application survived replacement.');
        assert(bitmaps.slice(0, counts[1]).every((bitmap) => bitmap.width === 0), 'Old bitmap survived replacement.');
        assert(textures.slice(0, counts[2]).every((texture) => texture.destroyed), 'Old texture survived replacement.');
      }
    });
  } finally {
    button.disabled = false;
    status.textContent = `${failed ? 'FAIL' : 'PASS'}: ${passed} passed, ${failed} failed`;
    status.dataset.result = failed ? 'fail' : 'pass';
  }
}
window.addEventListener('pagehide', () => display?.dispose(), { once: true });
