import { defineConfig } from 'vite';

export default defineConfig({
  server: {
    port: 5173,
    strictPort: true,
    proxy: Object.fromEntries(
      ['/display', '/content', '/healthz'].map((path) => [path, {
        target: process.env.GLYPHA_API_URL ?? 'http://127.0.0.1:8080',
      }]),
    ),
  },
});
