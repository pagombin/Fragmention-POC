import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// The backend binds to 8080 by default for local dev. All API and WebSocket
// URLs in the SPA are relative, so the dev server proxies them through.
export default defineConfig({
  plugins: [react()],
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    sourcemap: false,
  },
  server: {
    port: 5173,
    proxy: {
      '/api': { target: 'http://127.0.0.1:8080', changeOrigin: true, ws: true },
      '/metrics': { target: 'http://127.0.0.1:8080', changeOrigin: true },
      '/health': { target: 'http://127.0.0.1:8080', changeOrigin: true },
      '/ready': { target: 'http://127.0.0.1:8080', changeOrigin: true },
    },
  },
});
