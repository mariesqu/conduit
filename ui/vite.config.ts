import { defineConfig } from 'vite';

export default defineConfig({
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'es2022',
    cssCodeSplit: false,
    // Emit no inline module-preload polyfill script. The es2022 target
    // supports <link rel="modulepreload"> natively, and dropping the
    // inline <script> lets the server enforce a strict `script-src 'self'`
    // CSP without an 'unsafe-inline' escape hatch.
    modulePreload: { polyfill: false },
  },
  server: {
    port: 5173,
    strictPort: true,
    proxy: {
      '/ws': { target: 'ws://127.0.0.1:7180', ws: true, changeOrigin: true },
      '/api': 'http://127.0.0.1:7180',
    },
  },
});
