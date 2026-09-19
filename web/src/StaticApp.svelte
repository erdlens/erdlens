<script lang="ts">
  import { SvelteFlowProvider } from '@xyflow/svelte'
  import Viewer from './lib/Viewer.svelte'
  import type { Schema } from './lib/types'
  import { initWasm, parseErd, writeErd } from './lib/wasm'
  import sampleErd from './lib/sample.erd?raw'
  import logoUrl from './assets/logo.svg'

  let schema: Schema | null = null
  let filename = 'schema.erd'
  let error: string | null = null
  let loading = true
  let busy = false
  let fileInput: HTMLInputElement

  initWasm()
    .catch((e) => {
      error = (e as Error).message
    })
    .finally(() => {
      loading = false
    })

  async function openText(text: string, name: string) {
    busy = true
    error = null
    try {
      schema = await parseErd(text)
      filename = name
    } catch (e) {
      error = (e as Error).message
      schema = null
    } finally {
      busy = false
    }
  }

  async function onFileChange(e: Event) {
    const input = e.target as HTMLInputElement
    const file = input.files?.[0]
    if (!file) return
    const text = await file.text()
    await openText(text, file.name || 'schema.erd')
    input.value = ''
  }

  async function onDrop(e: DragEvent) {
    e.preventDefault()
    const file = e.dataTransfer?.files?.[0]
    if (!file) return
    const text = await file.text()
    await openText(text, file.name || 'schema.erd')
  }

  function onDragOver(e: DragEvent) {
    e.preventDefault()
  }

  async function openSample() {
    await openText(sampleErd, 'sample.erd')
  }

  async function persistSchema(s: Schema) {
    const erd = await writeErd(s)
    const blob = new Blob([erd], { type: 'text/plain;charset=utf-8' })
    const a = document.createElement('a')
    a.href = URL.createObjectURL(blob)
    a.download = filename.endsWith('.erd') ? filename : `${filename}.erd`
    a.click()
    URL.revokeObjectURL(a.href)
  }

  function closeFile() {
    schema = null
    error = null
  }
</script>

{#if schema}
  <SvelteFlowProvider>
    {#key filename + String(schema.tables.length)}
      <Viewer
        initialSchema={schema}
        {persistSchema}
        onClose={closeFile}
      />
    {/key}
  </SvelteFlowProvider>
{:else}
  <div class="landing">
    <div class="hero">
      <div class="brand">
        <img class="logo" src={logoUrl} alt="" width="28" height="28" />
        erdlens
      </div>
      <h1>Visualize a <code>.erd</code> file in your browser</h1>
      <p class="lede">
        Upload a schema — parsing runs locally via WASM. No server, no database
        connection, nothing leaves your machine.
      </p>

      <div
        class="drop"
        class:busy
        role="button"
        tabindex="0"
        on:dragover={onDragOver}
        on:drop={onDrop}
        on:click={() => fileInput?.click()}
        on:keydown={(e) => e.key === 'Enter' && fileInput?.click()}
      >
        {#if loading}
          Loading parser…
        {:else if busy}
          Parsing…
        {:else}
          Drop a <code>.erd</code> file here, or click to browse
        {/if}
      </div>
      <input
        bind:this={fileInput}
        type="file"
        accept=".erd,.hcl,text/plain"
        hidden
        on:change={onFileChange}
      />

      <div class="actions">
        <button class="btn" disabled={loading || busy} on:click={openSample}>
          Open sample schema
        </button>
        <a class="link" href="https://github.com/erdlens/erdlens">
          CLI &amp; docs →
        </a>
      </div>

      {#if error}
        <div class="error">{error}</div>
      {/if}

      <p class="note">
        Live database introspection stays in the
        <code>erdlens</code> CLI — this page only reads <code>.erd</code> /
        HCL text.
      </p>
    </div>
  </div>
{/if}

<style>
  .landing {
    min-height: 100vh;
    display: grid;
    place-items: center;
    padding: 32px 20px;
    background:
      radial-gradient(ellipse 80% 60% at 50% -10%, rgba(59, 130, 246, 0.12), transparent),
      var(--bg);
  }
  .hero {
    width: min(560px, 100%);
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .brand {
    font-weight: 700;
    font-size: 15px;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .logo {
    width: 28px;
    height: 28px;
    flex-shrink: 0;
    display: block;
  }
  h1 {
    font-size: 1.6rem;
    font-weight: 650;
    letter-spacing: -0.02em;
    line-height: 1.25;
    margin: 0;
  }
  h1 code {
    font-size: 0.9em;
  }
  .lede {
    margin: 0;
    color: var(--muted);
    line-height: 1.5;
  }
  .drop {
    border: 1.5px dashed var(--border);
    border-radius: 10px;
    padding: 48px 24px;
    text-align: center;
    color: var(--muted);
    cursor: pointer;
    background: var(--bg-alt);
    transition: border-color 0.15s ease, background 0.15s ease;
  }
  .drop:hover,
  .drop:focus-visible {
    border-color: var(--accent);
    outline: none;
  }
  .drop.busy {
    opacity: 0.7;
    cursor: wait;
  }
  .actions {
    display: flex;
    align-items: center;
    gap: 16px;
    flex-wrap: wrap;
  }
  .btn {
    background: var(--accent);
    color: white;
    border: none;
    border-radius: 6px;
    padding: 8px 14px;
    font-size: 13px;
    cursor: pointer;
  }
  .btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }
  .link {
    color: var(--accent);
    font-size: 13px;
    text-decoration: none;
  }
  .link:hover {
    text-decoration: underline;
  }
  .note {
    margin: 0;
    font-size: 12px;
    color: var(--muted);
    line-height: 1.45;
  }
  .error {
    color: crimson;
    font-size: 13px;
    background: rgba(220, 20, 60, 0.06);
    border: 1px solid rgba(220, 20, 60, 0.25);
    border-radius: 6px;
    padding: 10px 12px;
  }
  code {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 0.92em;
  }
</style>
