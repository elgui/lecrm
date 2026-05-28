import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';
import path from 'node:path';

// Vitest-only config. Kept separate from vite.config.ts so the app build
// (`tsc -b` + vite build) doesn't pull vitest's bundled vite types. Mirrors
// the app's @ alias and React plugin so component tests resolve and compile.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  test: {
    environment: 'jsdom',
    globals: true,
  },
});
