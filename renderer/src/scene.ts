import { Application, Container, Graphics, Sprite, Texture } from 'pixi.js';
import { fontFamily, fontWeight, loadDisplayFont } from './font';

const profile = 'noto-sans-jp-2.004-regular-400-v1';
const limit = 48 * 1024 * 1024;
type ObjectValue = Record<string, unknown>;
const object = (x: unknown): ObjectValue => { if (!x || typeof x !== 'object' || Array.isArray(x)) throw new Error('Expected object.'); return x as ObjectValue; };
const number = (x: unknown): number => { if (typeof x !== 'number' || !Number.isSafeInteger(x)) throw new Error('Expected integer.'); return x; };
const string = (x: unknown): string => { if (typeof x !== 'string') throw new Error('Expected string.'); return x; };
const color = (x: unknown): string => { const s = string(x); if (!/^#[\da-f]{6}$/i.test(s)) throw new Error('Invalid color.'); return s; };

// Bound the body while streaming, even if Content-Length is missing or incorrect.
export async function readEnvelope(response: Response): Promise<unknown> {
  const reader = response.body?.getReader();
  if (!reader) throw new Error('Missing display body.');
  const chunks: Uint8Array[] = []; let length = 0;
  try {
    while (true) {
      const { done, value } = await reader.read(); if (done) break;
      length += value.byteLength; if (length > limit) throw new Error('Display response exceeds limit.'); chunks.push(value);
    }
  } catch (error) { await reader.cancel(); throw error; }
  finally { reader.releaseLock(); }
  const bytes = new Uint8Array(length); let offset = 0;
  for (const chunk of chunks) { bytes.set(chunk, offset); offset += chunk.length; }
  return JSON.parse(new TextDecoder('utf-8', { fatal: true }).decode(bytes));
}
export interface Prepared { canvas: HTMLCanvasElement; dispose(): void }

// Every candidate has its own off-DOM canvas. Failure never draws over the active canvas.
export async function prepareScene(input: unknown, signal: AbortSignal): Promise<Prepared> {
  const envelope = object(input); const ast = object(envelope.ast);
  if (envelope.protocol !== 1 || ast.version !== 1 || ast.profile !== profile) throw new Error('Unsupported display profile.');
  string(envelope.generation); string(envelope.scene);
  const canvas = object(ast.canvas); const width = number(canvas.width), height = number(canvas.height);
  if (width !== 1920 || height !== 1080) throw new Error('Unsupported canvas.');
  const background = object(ast.background); const backgroundColor = color(background.color);
  if (!Array.isArray(ast.elements) || ast.elements.length > 128) throw new Error('Invalid element list.');
  const assets = object(envelope.assets);
  const textures = new Map<string, Texture>(); const bitmaps: ImageBitmap[] = [];
  const textTextures: Texture[] = [];
  let app: Application | undefined;
  const dispose = () => { app?.destroy(true, { children: true }); app = undefined; for (const texture of textures.values()) texture.destroy(true); textures.clear(); for (const texture of textTextures) texture.destroy(true); textTextures.length = 0; for (const bitmap of bitmaps) bitmap.close(); bitmaps.length = 0; };
  try {
    await loadDisplayFont(); signal.throwIfAborted();
    let bytesTotal = 0, pixelsTotal = 0;
    for (const [id, value] of Object.entries(assets)) {
      const asset = object(value); const media = string(asset.mediaType);
      if (!['image/png', 'image/jpeg'].includes(media)) throw new Error('Unsupported image format.');
      const raw = atob(string(asset.base64)); bytesTotal += raw.length;
      if (bytesTotal > 32 * 1024 * 1024) throw new Error('Asset bytes exceed limit.');
      const bytes = Uint8Array.from(raw, (c) => c.charCodeAt(0));
      const hash = Array.from(new Uint8Array(await crypto.subtle.digest('SHA-256', bytes)), (n) => n.toString(16).padStart(2, '0')).join('');
      if (hash !== asset.sha256) throw new Error('Asset digest mismatch.');
      const [imageWidth, imageHeight] = imageDimensions(bytes, media);
      pixelsTotal += imageWidth * imageHeight;
      if (pixelsTotal > 32_000_000) throw new Error('Decoded images exceed limit.');
      const bitmap = await createImageBitmap(new Blob([bytes], { type: media })); bitmaps.push(bitmap);
      if (bitmap.width !== imageWidth || bitmap.height !== imageHeight) throw new Error('Image dimensions disagree.');
      textures.set(id, Texture.from(bitmap)); signal.throwIfAborted();
    }
    app = new Application();
    await app.init({ width, height, background: backgroundColor, antialias: true, autoStart: false, resolution: 1 });
    signal.throwIfAborted();
    const image = (id: unknown, fit: unknown, x: number, y: number, w: number, h: number) => {
      if (fit !== 'contain' && fit !== 'cover') throw new Error('Invalid image fit.');
      const texture = textures.get(string(id)); if (!texture) throw new Error('Missing image.');
      const sprite = new Sprite(texture); const container = new Container(); container.position.set(x, y);
      const scale = fit === 'contain' ? Math.min(w / texture.width, h / texture.height) : Math.max(w / texture.width, h / texture.height);
      sprite.scale.set(scale); sprite.position.set((w - sprite.width) / 2, (h - sprite.height) / 2); container.addChild(sprite);
      const mask = new Graphics().rect(0, 0, w, h).fill(0xffffff); container.addChild(mask); container.mask = mask;
      app!.stage.addChild(container);
    };
    if (background.image !== undefined) image(background.image, background.fit, 0, 0, width, height);
    for (const item of ast.elements) {
      const element = object(item); const x = number(element.x), y = number(element.y), w = number(element.width), h = number(element.height);
      if (x < 0 || y < 0 || w <= 0 || h <= 0 || w > width || h > height || x > width - w || y > height - h) throw new Error('Invalid rectangle.');
      if (element.type === 'image') { image(element.asset, element.fit, x, y, w, h); continue; }
      if (element.type !== 'text') throw new Error('Unsupported element.');
      const size = number(element.fontSize), content = string(element.text);
      if (size <= 0 || size > 4096 || Array.from(content).length > 16384) throw new Error('Invalid text size.');
      const align = string(element.align); if (!['left', 'center', 'right'].includes(align)) throw new Error('Invalid alignment.');
      const lines = content.split('\n');
      if (lines.length * size * 1.2 > h) throw new Error('Text exceeds height.');
      for (const [i, line] of lines.entries()) {
        if (!line) continue;
        // Measure actual ink, not the font-wide ascender/descender box.
        const surface = document.createElement('canvas');
        const context = surface.getContext('2d');
        if (!context) throw new Error('Text canvas is unavailable.');
        const font = `${fontWeight} ${size}px "${fontFamily}"`;
        context.font = font;
        const metrics = context.measureText(line);
        const left = Math.min(0, -metrics.actualBoundingBoxLeft);
        const right = Math.max(metrics.width, metrics.actualBoundingBoxRight);
        const ascent = Math.max(0, metrics.actualBoundingBoxAscent);
        const descent = Math.max(0, metrics.actualBoundingBoxDescent);
        const inkWidth = right - left;
        if (inkWidth > w || ascent + descent > size * 1.2) throw new Error('Text does not fit in this renderer.');
        // A transparent border preserves antialiasing at punctuation edges.
        const padding = 2;
        surface.width = Math.max(1, Math.ceil(inkWidth)) + padding * 2;
        surface.height = Math.max(1, Math.ceil(ascent + descent)) + padding * 2;
        context.font = font; context.fillStyle = color(element.color);
        context.fillText(line, padding - left, padding + ascent);
        const texture = Texture.from(surface); textTextures.push(texture);
        const text = new Sprite(texture);
        text.position.set(x + (align === 'center' ? (w - inkWidth) / 2 : align === 'right' ? w - inkWidth : 0) - padding, y + i * size * 1.2 - padding);
        app.stage.addChild(text);
      }
    }
    app.render(); signal.throwIfAborted();
    app.canvas.setAttribute('role', 'img'); app.canvas.setAttribute('aria-label', 'Glypha scene');
    app.canvas.style.objectFit = 'contain';
    return { canvas: app.canvas, dispose };
  } catch (error) { dispose(); throw error; }
}

// Inspect dimensions before allocating decoded pixel buffers.
function imageDimensions(bytes: Uint8Array, media: string): [number, number] {
  const view = new DataView(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const checked = (width: number, height: number): [number, number] => {
    if (width < 1 || height < 1 || width * height > 32_000_000) throw new Error('Invalid image dimensions.');
    return [width, height];
  };
  if (media === 'image/png') {
    if (bytes.length < 33 || view.getUint32(0) !== 0x89504e47 || view.getUint32(4) !== 0x0d0a1a0a || view.getUint32(8) !== 13 || view.getUint32(12) !== 0x49484452) throw new Error('Invalid PNG header.');
    for (let at = 8; at + 12 <= bytes.length;) {
      const length = view.getUint32(at);
      if (length > bytes.length - at - 12) throw new Error('Truncated PNG.');
      if (view.getUint32(at + 4) === 0x6163544c) throw new Error('Animated PNG is unsupported.');
      at += length + 12;
    }
    return checked(view.getUint32(16), view.getUint32(20));
  }
  if (bytes.length < 4 || view.getUint16(0) !== 0xffd8) throw new Error('Invalid JPEG header.');
  let at = 2;
  while (at + 4 <= bytes.length) {
    if (bytes[at++] !== 0xff) throw new Error('Invalid JPEG marker.');
    while (bytes[at] === 0xff) at++;
    const marker = bytes[at++];
    if (marker === 0xda || marker === 0xd9) break;
    if (at + 2 > bytes.length) break;
    const size = view.getUint16(at);
    if (size < 2 || at + size > bytes.length) throw new Error('Truncated JPEG.');
    if (marker >= 0xc0 && marker <= 0xcf && ![0xc4, 0xc8, 0xcc].includes(marker)) {
      if (size < 8) throw new Error('Invalid JPEG frame.');
      return checked(view.getUint16(at + 5), view.getUint16(at + 3));
    }
    at += size;
  }
  throw new Error('JPEG dimensions are missing.');
}
