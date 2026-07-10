import { defineConfig, type Plugin } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

const allowedUrl =
  /^https?:\/\/(localhost|127\.0\.0\.1|www\.w3\.org)([:/]|$)/

/** Remove third-party URLs from production chunks so the embedded viewer stays offline-safe. */
function stripExternalUrls(): Plugin {
  const strip = (source: string) =>
    source.replace(/https?:\/\/[^"'\s<>\\]+/g, (url) =>
      allowedUrl.test(url) ? url : '',
    )

  return {
    name: 'strip-external-urls',
    apply: 'build',
    renderChunk(code) {
      return strip(code)
    },
    transformIndexHtml(html) {
      return strip(html)
    },
  }
}

// During `npm run dev`, proxy /api to a running `erdlens view` on 8787.
// In production the same server serves the built bundle directly.
export default defineConfig({
  plugins: [svelte(), stripExternalUrls()],
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
