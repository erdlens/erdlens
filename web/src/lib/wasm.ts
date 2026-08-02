import type { Schema } from './types'

export type ErdLensWasm = {
  parse: (erdText: string) => { schemaJSON?: string; error?: string }
  write: (schemaJSON: string) => { erd?: string; error?: string }
}

declare global {
  interface Window {
    erdlens?: ErdLensWasm
    Go?: new () => {
      importObject: WebAssembly.Imports
      run: (instance: WebAssembly.Instance) => Promise<void>
    }
  }
}

let ready: Promise<ErdLensWasm> | null = null

function loadScript(src: string): Promise<void> {
  return new Promise((resolve, reject) => {
    const existing = document.querySelector(`script[src="${src}"]`)
    if (existing) {
      resolve()
      return
    }
    const s = document.createElement('script')
    s.src = src
    s.onload = () => resolve()
    s.onerror = () => reject(new Error(`failed to load ${src}`))
    document.head.appendChild(s)
  })
}

/** Initialize the parse-only WASM module (no DB / network code). */
export function initWasm(base = import.meta.env.BASE_URL): Promise<ErdLensWasm> {
  if (ready) return ready
  ready = (async () => {
    const prefix = base.endsWith('/') ? base : `${base}/`
    await loadScript(`${prefix}wasm/wasm_exec.js`)
    if (!window.Go) throw new Error('Go WASM runtime missing (wasm_exec.js)')

    const go = new window.Go()
    const wasmURL = `${prefix}wasm/erdlens.wasm`
    const resp = await fetch(wasmURL)
    if (!resp.ok) throw new Error(`failed to fetch WASM (${resp.status})`)
    const buf = await resp.arrayBuffer()
    const { instance } = await WebAssembly.instantiate(buf, go.importObject)
    // go.run blocks forever (select {}); kick it off without awaiting.
    void go.run(instance)

    // Tiny delay so the JS global is registered.
    for (let i = 0; i < 50 && !window.erdlens; i++) {
      await new Promise((r) => setTimeout(r, 10))
    }
    if (!window.erdlens) throw new Error('erdlens WASM API not registered')
    return window.erdlens
  })()
  return ready
}

export async function parseErd(text: string): Promise<Schema> {
  const api = await initWasm()
  const res = api.parse(text)
  if (res.error) throw new Error(res.error)
  if (!res.schemaJSON) throw new Error('parse returned empty schema')
  return JSON.parse(res.schemaJSON) as Schema
}

export async function writeErd(schema: Schema): Promise<string> {
  const api = await initWasm()
  const res = api.write(JSON.stringify(schema))
  if (res.error) throw new Error(res.error)
  if (res.erd == null) throw new Error('write returned empty document')
  return res.erd
}
