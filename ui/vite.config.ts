import { svelte } from '@sveltejs/vite-plugin-svelte';
import { defineConfig } from 'vite';

export default defineConfig({
  plugins: [svelte()],
  base: './',
  server: {
    host: '127.0.0.1',
    port: 3173,
    strictPort: true
  },
  build: {
    outDir: '../cmd/jide/frontend/dist',
    emptyOutDir: true,
    sourcemap: false
  }
});
