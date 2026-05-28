import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';

// Test config lives in vitest.config.ts so this file types against a single
// vite version under `tsc -b` (vitest bundles its own vite, which otherwise
// clashes with the app's when `test` is declared here).
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5173,
    proxy: {
      '/auth': 'http://localhost:8080',
      '/v1': 'http://localhost:8080',
      '/healthz': 'http://localhost:8080',
    },
  },
});
