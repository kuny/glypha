import { Application, Container, Text } from 'pixi.js';
import './style.css';
import { fontFamily, fontWeight, loadDisplayFont } from './font';

const host = document.querySelector<HTMLElement>('#display')!;
const builtin = document.querySelector<HTMLElement>('#builtin')!;
const abort = new AbortController();
let application: Application | undefined;

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

  // The bootstrap API has no content yet. Never replace Builtin with errors.
  let delay = 1000;
  while (!abort.signal.aborted) {
    try {
      const response = await fetch('/display', {
        cache: 'no-store', signal: AbortSignal.any([abort.signal, AbortSignal.timeout(15000)]),
      });
      if (response.status !== 204) {
        await response.body?.cancel();
        throw new Error(`Unsupported display response: ${response.status}`);
      }
      delay = 1000;
    } catch (error) {
      if (abort.signal.aborted) break;
      console.warn('Display retrieval failed; retaining the current scene.', error);
    }
    await new Promise<void>((resolve) => {
      const finish = () => { clearTimeout(timer); abort.signal.removeEventListener('abort', finish); resolve(); };
      const timer = setTimeout(finish, delay);
      abort.signal.addEventListener('abort', finish, { once: true });
    });
    delay = Math.min(delay * 2, 15000);
  }
}

function stop(): void {
  abort.abort();
  application?.destroy(true, { children: true });
  builtin.hidden = false;
}
window.addEventListener('pagehide', stop, { once: true });
window.addEventListener('pageshow', (event) => { if (event.persisted) window.location.reload(); });
if (import.meta.hot) import.meta.hot.dispose(stop);
void start().catch((error: unknown) => {
  // The HTML Builtin remains visible if font loading or graphics initialization fails.
  console.error('Renderer initialization failed.', error);
});
