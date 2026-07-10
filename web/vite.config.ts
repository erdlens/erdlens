import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// During `npm run dev`, proxy /api to a running `erdlens view` on 8787.
// In production the same server serves the built bundle directly.
export default defineConfig({
  plugins: [svelte()],
  server: {
    proxy: {
      '/api': 'http://127.0.0.1:8787',
    },
  },
  build: {
    outDir: 'dist',
    emptyOutDir: true,
    target: 'es2020',
    // Keep assets inlined so the embedded bundle stays a small handful of files.
    assetsInlineLimit: 4096,
  },
})
