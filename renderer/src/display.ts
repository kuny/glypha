import { prepareScene, readEnvelope, type Prepared } from './scene';

// A displayed frame and its successful ETag always change together.
export class Display {
  private active: Prepared;
  private stopped = false;
  private preparing = false;
  private tag: string | undefined;

  constructor(
    private readonly host: HTMLElement,
    initial: Prepared,
    private readonly prepare: typeof prepareScene = prepareScene,
  ) { this.active = initial; }

  get etag(): string | undefined { return this.tag; }

  async accept(response: Response, signal: AbortSignal): Promise<void> {
    signal.throwIfAborted();
    if (this.stopped) throw new Error('Display has stopped.');
    if (this.preparing) throw new Error('Display preparation must be serial.');
    this.preparing = true;
    try {
      if (response.status === 200) {
        const candidate = await this.prepare(await readEnvelope(response), signal);
        try {
          signal.throwIfAborted();
          if (this.stopped) throw new Error('Display has stopped.');
          this.host.replaceChild(candidate.canvas, this.active.canvas);
        } catch (error) { candidate.dispose(); throw error; }
        const previous = this.active;
        this.active = candidate;
        this.tag = response.headers.get('ETag') ?? undefined;
        previous.dispose();
      } else if (response.status === 304) {
        if (!this.tag) throw new Error('Unexpected conditional response.');
      } else if (response.status !== 204) {
        await response.body?.cancel();
        throw new Error(`Display request failed: ${response.status}`);
      }
    } finally { this.preparing = false; }
  }

  dispose(): void {
    if (this.stopped) return;
    this.stopped = true;
    this.active.dispose();
  }
}

export function waitForRetry(milliseconds: number, signal: AbortSignal): Promise<void> {
  if (signal.aborted) return Promise.resolve();
  return new Promise((resolve) => {
    const finish = () => { clearTimeout(timer); signal.removeEventListener('abort', finish); resolve(); };
    const timer = setTimeout(finish, milliseconds);
    signal.addEventListener('abort', finish, { once: true });
  });
}

interface Polling {
  fetch: typeof fetch;
  wait: typeof waitForRetry;
  report(error: unknown): void;
}
const defaults: Polling = {
  fetch: (...args) => fetch(...args),
  wait: waitForRetry,
  report: (error) => console.warn('Display retrieval failed; retaining the current scene.', error),
};

// No second request starts before acquisition, preparation, and replacement finish.
export async function pollDisplay(display: Display, signal: AbortSignal, options: Partial<Polling> = {}): Promise<void> {
  const polling = { ...defaults, ...options };
  let delay = 1000;
  while (!signal.aborted) {
    let failed = false;
    try {
      const attemptSignal = AbortSignal.any([signal, AbortSignal.timeout(15000)]);
      const response = await polling.fetch('/display', {
        cache: 'no-store', headers: display.etag ? { 'If-None-Match': display.etag } : {}, signal: attemptSignal,
      });
      await display.accept(response, attemptSignal);
      delay = 1000;
    } catch (error) {
      if (signal.aborted) break;
      failed = true;
      polling.report(error);
    }
    await polling.wait(delay, signal);
    if (failed) delay = Math.min(delay * 2, 15000);
  }
}
