import { Application, Container, Text } from 'pixi.js';
import './style.css';
import { Display, pollDisplay } from './display';
import { fontFamily, fontWeight, loadDisplayFont } from './font';

const host = document.querySelector<HTMLElement>('#display')!;
const builtin = document.querySelector<HTMLElement>('#builtin')!;
const abort = new AbortController();
let application: Application | undefined;
let display: Display | undefined;

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
    if (!application || abort.signal.aborted) return;
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

  display = new Display(host, {
    canvas: app.canvas,
    dispose: () => { app.destroy(true, { children: true }); application = undefined; },
  });
  await pollDisplay(display, abort.signal);
}

function stop(): void {
  abort.abort();
  display?.dispose();
  display = undefined;
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
