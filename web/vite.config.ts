import { defineConfig, type Plugin, type UserConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'
import { resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

const root = fileURLToPath(new URL('.', import.meta.url))

const allowedUrl =
  /^https?:\/\/(localhost|127\.0\.0\.1|www\.w3\.org)([:/]|$)/

/** Remove third-party URLs from the CLI-embedded bundle (offline-safe). */
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

/** Emit pages.html as index.html for GitHub Pages. */
function pagesIndexHtml(): Plugin {
  return {
    name: 'pages-index-html',
    enforce: 'post',
    generateBundle(_, bundle) {
      const page = bundle['pages.html']
      if (page && page.type === 'asset') {
        page.fileName = 'index.html'
      }
    },
  }
}

export default defineConfig(({ mode }): UserConfig => {
  const isPages = mode === 'pages'

  return {
    base: isPages ? './' : '/',
    plugins: [
      svelte(),
      ...(isPages ? [pagesIndexHtml()] : [stripExternalUrls()]),
    ],
    // Pages assets (WASM) live in public-pages/; CLI embed uses empty/default public/.
    publicDir: isPages ? 'public-pages' : false,
    server: {
      proxy: {
        '/api': 'http://127.0.0.1:8787',
      },
    },
    build: {
      outDir: isPages ? 'pages-dist' : 'dist',
      emptyOutDir: true,
      target: 'es2020',
      assetsInlineLimit: 4096,
      rollupOptions: isPages
        ? { input: resolve(root, 'pages.html') }
        : undefined,
    },
  }
})
