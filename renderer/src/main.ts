import { Application, Container, Text } from 'pixi.js';
import './style.css';
import { prepareScene, readEnvelope, type Prepared } from './scene';
import { fontFamily, fontWeight, loadDisplayFont } from './font';

const host = document.querySelector<HTMLElement>('#display')!;
const builtin = document.querySelector<HTMLElement>('#builtin')!;
const abort = new AbortController();
let application: Application | undefined;
let active: Prepared | undefined;

async function start(): Promise<void> {
  // Do not measure or rasterize PixiJS text with a fallback font.
  await loadDisplayFont();
  if (abort.signal.aborted) return;
  const app = new Application();
  await app.init({
    background: '#182028', resizeTo: host, antialias: true, autoStart: false,
    resolution: Math.min(window.devicePixelRatio, 2), autoDensity: true,
  });
  if (abort.signal.aborted) { app.destroy(true, { children: true }); return; }
  application = app;
  const scene = new Container();
  const lines = [
    { text: 'Glypha', y: -110, size: 112 },
    { text: '“Stand out of my sun.”', y: 50, size: 42 },
    { text: '— Diogenes', y: 120, size: 28 },
  ];
  for (const line of lines) {
    const text = new Text({
      text: line.text,
      style: {
        fontFamily, fontWeight, fontSize: line.size, fill: '#f4f1e9',
        // Preserve glyph overhangs when PixiJS rasterizes the text texture.
        padding: Math.ceil(line.size * 0.2),
      },
    });
    text.anchor.set(0.5);
    text.y = line.y;
    scene.addChild(text);
  }
  app.stage.addChild(scene);
  const draw = () => {
    if (active || abort.signal.aborted) return;
    app.resize();
    scene.position.set(app.screen.width / 2, app.screen.height / 2);
    scene.scale.set(Math.min(app.screen.width / 1280, app.screen.height / 720));
    app.render();
  };
  draw();
  app.canvas.setAttribute('role', 'img');
  app.canvas.setAttribute('aria-label', 'Glypha. Stand out of my sun. Diogenes.');
  host.appendChild(app.canvas);
  builtin.hidden = true;
  window.addEventListener('resize', draw, { signal: abort.signal });

  let successfulETag: string | undefined;
  let delay = 1000;
  let failed = false;
  while (!abort.signal.aborted) {
    failed = false;
    try {
      const response = await fetch('/display', {
        cache: 'no-store', headers: successfulETag ? { 'If-None-Match': successfulETag } : {}, signal: AbortSignal.any([abort.signal, AbortSignal.timeout(15000)]),
      });
      if (response.status === 200) {
        const candidate = await prepareScene(await readEnvelope(response), abort.signal);
        if (abort.signal.aborted) { candidate.dispose(); break; }
        const previous = active;
        try { host.replaceChild(candidate.canvas, previous?.canvas ?? app.canvas); }
        catch (error) { candidate.dispose(); throw error; }
        active = candidate;
        successfulETag = response.headers.get('ETag') ?? undefined;
        if (previous) previous.dispose();
        else { app.destroy(true, { children: true }); application = undefined; }
      } else if (response.status === 304) {
        if (!successfulETag) throw new Error('Unexpected conditional response.');
      } else if (response.status !== 204) {
        await response.body?.cancel();
        throw new Error(`Display request failed: ${response.status}`);
      }
      delay = 1000;
    } catch (error) {
      if (abort.signal.aborted) break;
      failed = true;
      console.warn('Display retrieval failed; retaining the current scene.', error);
    }
    await new Promise<void>((resolve) => {
      const finish = () => { clearTimeout(timer); abort.signal.removeEventListener('abort', finish); resolve(); };
      const timer = setTimeout(finish, delay);
      abort.signal.addEventListener('abort', finish, { once: true });
    });
    if (failed) delay = Math.min(delay * 2, 15000);
  }
}

function stop(): void {
  abort.abort();
  active?.dispose();
  active = undefined;
  application?.destroy(true, { children: true });
  application = undefined;
  builtin.hidden = false;
}
window.addEventListener('pagehide', stop, { once: true, signal: abort.signal });
window.addEventListener('pageshow', (event) => { if (event.persisted) window.location.reload(); });
if (import.meta.hot) import.meta.hot.dispose(stop);
void start().catch((error: unknown) => {
  // The HTML Builtin remains visible if font loading or graphics initialization fails.
  console.error('Renderer initialization failed.', error);
});
