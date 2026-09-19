<script lang="ts">
  import { onMount, onDestroy, tick } from 'svelte'
  import { writable, get } from 'svelte/store'
  import {
    SvelteFlow,
    Background,
    Controls,
    MiniMap,
    useSvelteFlow,
  } from '@xyflow/svelte'
  import type { Node, Edge } from '@xyflow/svelte'
  import '@xyflow/svelte/dist/style.css'
  import { toPng, toSvg } from 'html-to-image'

  import type { Schema, View, Table } from './types'
  import { autoLayout, isolatedLayout, nodeHeight, NODE_WIDTH } from './layout'
  import { matchesView } from './glob'
  import {
    tableId,
    refTableId,
    tableLabel,
    schemaName,
    distinctSchemas,
  } from './tableId'
  import TableNode from './TableNode.svelte'
  import logoUrl from '../assets/logo.svg'

  /** When set, skip GET /api/schema and use this schema (static / Pages mode). */
  export let initialSchema: Schema | null = null
  /**
   * Optional layout persist hook for static mode.
   * Receives the schema with updated table.layout values.
   * When unset, falls back to POST /api/layout (CLI embedded viewer).
   */
  export let persistSchema: ((s: Schema) => Promise<void> | void) | null = null
  /** Optional header control rendered in the sidebar footer area. */
  export let onClose: (() => void) | null = null

  const nodeTypes = { table: TableNode }
  const nodes = writable<Node[]>([])
  const edges = writable<Edge[]>([])
  const { setCenter, fitView } = useSvelteFlow()

  let schema: Schema | null = null
  let search = ''
  let selected: string | null = null
  let locked = false
  let isolate = false
  let activeViewName = ''
  let error: string | null = null
  let saveTimer: ReturnType<typeof setTimeout> | null = null
  let searchInput: HTMLInputElement

  let adjacency = new Map<string, Set<string>>()

  // empty Set means "all" until schemas discovered
  let enabledSchemas: Set<string> = new Set()
  let schemasInitialized = false

  // Snapshot of pre-isolate positions. Non-null while isolate mode is active.
  // We restore from this when isolate is turned off, so users don't lose their
  // hand-arranged layout after exploring a subgraph.
  let savedPositions: Map<string, { x: number; y: number }> | null = null

  // --- Derived state ---------------------------------------------------------

  $: activeView =
    activeViewName && schema?.views
      ? (schema.views.find((v) => v.name === activeViewName) ?? null)
      : null

  $: availableSchemas = schema ? distinctSchemas(schema.tables) : []
  $: if (schema && !schemasInitialized && availableSchemas.length > 0) {
    enabledSchemas = new Set(availableSchemas)
    schemasInitialized = true
  }

  $: tableById = schema
    ? new Map(schema.tables.map((t) => [tableId(t), t]))
    : new Map<string, Table>()

  $: matchedColumns = computeColumnMatches(schema, search)

  // relatedColumns: for the currently selected table, which columns in every
  // table participate in the relationships that touch the selection.
  // - Selected table T: its FK source columns + its columns referenced by
  //   incoming FKs.
  // - Neighbor N: FK columns in N that point to T + columns in N referenced
  //   by outgoing FKs from T.
  $: relatedColumns = computeRelatedColumns(schema, selected)

  $: filteredTables = schema
    ? schema.tables
        .filter(
          (t) =>
            !activeView ||
            matchesView(t.name, activeView.include, activeView.exclude),
        )
        .filter((t) => schemaEnabled(t))
        .filter((t) => tableMatchesSearch(t, search, matchedColumns))
    : []

  $: edgeCount = $edges.length

  function schemaEnabled(t: Table): boolean {
    if (availableSchemas.length <= 1) return true
    if (enabledSchemas.size === 0) return true
    return enabledSchemas.has(schemaName(t))
  }

  function toggleSchema(s: string) {
    const next = new Set(enabledSchemas)
    if (next.has(s)) {
      // Don't allow unchecking the last schema
      if (next.size <= 1) return
      next.delete(s)
    } else {
      next.add(s)
    }
    enabledSchemas = next
  }

  function tableMatchesSearch(
    t: Table,
    term: string,
    cols: Map<string, Set<string>>,
  ): boolean {
    if (!term) return true
    const q = term.toLowerCase()
    const id = tableId(t)
    return (
      t.name.toLowerCase().includes(q) ||
      id.toLowerCase().includes(q) ||
      cols.has(id)
    )
  }

  function computeColumnMatches(
    sch: Schema | null,
    term: string,
  ): Map<string, Set<string>> {
    const result = new Map<string, Set<string>>()
    if (!sch || !term) return result
    const q = term.toLowerCase()
    for (const t of sch.tables) {
      const hits = t.columns
        .filter((c) => c.name.toLowerCase().includes(q))
        .map((c) => c.name)
      if (hits.length > 0) result.set(tableId(t), new Set(hits))
    }
    return result
  }

  function computeRelatedColumns(
    sch: Schema | null,
    sel: string | null,
  ): Map<string, Set<string>> {
    const result = new Map<string, Set<string>>()
    if (!sch || !sel) return result
    const add = (table: string, col: string) => {
      if (!result.has(table)) result.set(table, new Set())
      result.get(table)!.add(col)
    }
    // Highlight every PK and every FK column on the selected table itself,
    // so a click makes the table's "identity" (PKs) and "relationships" (FKs)
    // pop even when a neighbor isn't visible on screen.
    const selTable = sch.tables.find((t) => tableId(t) === sel)
    if (selTable) {
      for (const c of selTable.primary_key ?? []) add(sel, c)
      for (const fk of selTable.foreign_keys ?? []) {
        fk.columns.forEach((c) => add(sel, c))
      }
    }
    for (const t of sch.tables) {
      const tid = tableId(t)
      for (const fk of t.foreign_keys ?? []) {
        const refId = refTableId(fk)
        if (tid === sel) {
          // outgoing FK from the selected table
          fk.columns.forEach((c) => add(tid, c))
          fk.ref_columns.forEach((c) => add(refId, c))
        } else if (refId === sel) {
          // incoming FK to the selected table
          fk.columns.forEach((c) => add(tid, c))
          fk.ref_columns.forEach((c) => add(sel, c))
        }
      }
    }
    return result
  }

  // --- Load + build ----------------------------------------------------------

  async function load() {
    try {
      schemasInitialized = false
      if (initialSchema) {
        schema = initialSchema
      } else {
        const res = await fetch('/api/schema')
        if (!res.ok) throw new Error(`HTTP ${res.status}`)
        schema = await res.json()
      }
      buildAdjacency()
      build()
      const hash = decodeURIComponent(location.hash.slice(1))
      if (hash) {
        const byId = schema!.tables.find((t) => tableId(t) === hash)
        const byBare =
          byId ??
          schema!.tables.find(
            (t) => (!t.schema || t.schema === 'public') && t.name === hash,
          )
        if (byBare) {
          await tick()
          selectTable(tableId(byBare), true)
        }
      }
    } catch (e) {
      error = (e as Error).message
    }
  }

  function buildAdjacency() {
    adjacency = new Map()
    if (!schema) return
    const known = new Set(schema.tables.map((t) => tableId(t)))
    for (const t of schema.tables) {
      const tid = tableId(t)
      if (!adjacency.has(tid)) adjacency.set(tid, new Set())
      for (const fk of t.foreign_keys ?? []) {
        const refId = refTableId(fk)
        if (!known.has(refId)) continue
        adjacency.get(tid)!.add(refId)
        if (!adjacency.has(refId)) adjacency.set(refId, new Set())
        adjacency.get(refId)!.add(tid)
      }
    }
  }

  function build() {
    if (!schema) return
    const allSaved =
      schema.tables.length > 0 && schema.tables.every((t) => t.layout)
    const auto = allSaved ? new Map() : autoLayout(schema.tables)

    nodes.set(
      schema.tables.map((t) => {
        const tid = tableId(t)
        return {
          id: tid,
          type: 'table',
          position: t.layout ?? auto.get(tid)!,
          data: {
            table: t,
            highlighted: false,
            dimmed: false,
            matched: new Set<string>(),
          },
        }
      }),
    )

    const known = new Set(schema.tables.map((t) => tableId(t)))
    const es: Edge[] = []
    for (const t of schema.tables) {
      const tid = tableId(t)
      for (const fk of t.foreign_keys ?? []) {
        const refId = refTableId(fk)
        if (!known.has(refId)) continue
        // Emit one edge per column pair so composite FKs render accurately and
        // each edge terminates at the exact column row on both ends.
        const pairs = Math.min(fk.columns.length, fk.ref_columns.length)
        for (let i = 0; i < pairs; i++) {
          es.push({
            id: `${tid}|${fk.name ?? fk.columns.join('_')}|${refId}|${i}`,
            source: tid,
            sourceHandle: fk.columns[i],
            target: refId,
            targetHandle: fk.ref_columns[i],
            type: 'smoothstep',
          })
        }
      }
    }
    edges.set(es)
  }

  // --- Visibility + highlight (combined) ------------------------------------

  $: applyState(
    selected,
    isolate,
    activeView,
    matchedColumns,
    relatedColumns,
    enabledSchemas,
    availableSchemas,
  )

  function applyState(
    sel: string | null,
    iso: boolean,
    view: View | null,
    matched: Map<string, Set<string>>,
    related: Map<string, Set<string>>,
    _enabled: Set<string>,
    _schemas: string[],
  ) {
    const inView = (id: string) => {
      if (!view) return true
      const t = tableById.get(id)
      return matchesView(t?.name ?? id, view.include, view.exclude)
    }
    const inSchema = (id: string) => {
      const t = tableById.get(id)
      return t ? schemaEnabled(t) : true
    }
    const neighbors = sel
      ? (adjacency.get(sel) ?? new Set<string>())
      : new Set<string>()
    const focusSet = sel ? new Set([sel, ...neighbors]) : null

    nodes.update((ns) =>
      ns.map((n) => {
        const outsideView = !inView(n.id)
        const outsideSchema = !inSchema(n.id)
        const outsideFocus = focusSet !== null && !focusSet.has(n.id)
        const hiddenByIsolate = iso && sel !== null && outsideFocus
        return {
          ...n,
          hidden: outsideView || outsideSchema || hiddenByIsolate,
          data: {
            ...n.data,
            highlighted: n.id === sel,
            dimmed: focusSet !== null && outsideFocus,
            matched: matched.get(n.id) ?? new Set(),
            related: related.get(n.id) ?? new Set(),
          },
        }
      }),
    )
    edges.update((es) =>
      es.map((e) => {
        const bothInView = inView(e.source) && inView(e.target)
        const bothInSchema = inSchema(e.source) && inSchema(e.target)
        const touches = sel && (e.source === sel || e.target === sel)
        const hiddenByIsolate = iso && sel !== null && !touches
        return {
          ...e,
          hidden: !bothInView || !bothInSchema || hiddenByIsolate,
          animated: !!touches,
          style:
            focusSet !== null && !touches
              ? 'opacity: 0.1; stroke: var(--muted);'
              : touches
                ? 'stroke: var(--accent); stroke-width: 2;'
                : '',
        }
      }),
    )
  }

  // --- Selection + navigation -----------------------------------------------

  function selectTable(id: string, focusCanvas = true) {
    selected = id
    history.replaceState(null, '', `#${encodeURIComponent(id)}`)

    queueMicrotask(() => {
      document
        .querySelector(`[data-tid="${CSS.escape(id)}"]`)
        ?.scrollIntoView({ block: 'nearest' })
    })

    if (focusCanvas && schema) {
      const node = get(nodes).find((n) => n.id === id)
      const table = schema.tables.find((t) => tableId(t) === id)
      if (node && table) {
        const cx = node.position.x + NODE_WIDTH / 2
        const cy = node.position.y + nodeHeight(table) / 2
        setCenter(cx, cy, { zoom: 1.1, duration: 500 })
      }
    }
  }

  function clearSelection() {
    selected = null
    // Clicking away should feel like a full reset: drop isolate mode too so
    // the user returns to the base view without having to also press `I`.
    isolate = false
    history.replaceState(null, '', location.pathname + location.search)
  }

  function toggleLock() {
    locked = !locked
  }
  function toggleIsolate() {
    isolate = !isolate
  }
  function doFitView() {
    fitView({ padding: 0.1, duration: 400 })
  }

  // --- Isolate re-layout ----------------------------------------------------

  // Whenever isolate or selection changes, either compress the graph around
  // the selection or restore the original positions.
  $: refreshIsolate(isolate, selected)

  function refreshIsolate(iso: boolean, sel: string | null) {
    if (iso && sel && schema) {
      if (!savedPositions) {
        savedPositions = snapshotPositions()
      }
      applyIsolatedLayout(sel)
    } else if (savedPositions) {
      restorePositions()
      savedPositions = null
    }
  }

  function snapshotPositions(): Map<string, { x: number; y: number }> {
    const snap = new Map<string, { x: number; y: number }>()
    for (const n of get(nodes)) {
      snap.set(n.id, { x: n.position.x, y: n.position.y })
    }
    return snap
  }

  function applyIsolatedLayout(sel: string) {
    if (!schema) return
    const neighbors = adjacency.get(sel) ?? new Set<string>()
    const focusTables = schema.tables.filter(
      (t) => tableId(t) === sel || neighbors.has(tableId(t)),
    )
    if (!focusTables.some((t) => tableId(t) === sel)) return

    const positions = isolatedLayout(focusTables, sel)

    nodes.update((ns) =>
      ns.map((n) => {
        const p = positions.get(n.id)
        return p ? { ...n, position: p } : n
      }),
    )
    setTimeout(() => fitView({ padding: 0.2, duration: 450 }), 60)
  }

  function restorePositions() {
    if (!savedPositions) return
    const saved = savedPositions
    nodes.update((ns) =>
      ns.map((n) => {
        const p = saved.get(n.id)
        return p ? { ...n, position: p } : n
      }),
    )
    setTimeout(() => fitView({ padding: 0.1, duration: 450 }), 60)
  }

  function onNodeClick(e: CustomEvent<{ node: Node }>) {
    selectTable(e.detail.node.id, false)
  }
  function onPaneClick() {
    clearSelection()
  }

  // --- Layout save -----------------------------------------------------------

  function scheduleSave() {
    if (saveTimer) clearTimeout(saveTimer)
    saveTimer = setTimeout(saveLayout, 400)
  }

  function syncLayoutToSchema() {
    if (!schema) return
    const current = get(nodes)
    for (const t of schema.tables) {
      const n = current.find((node) => node.id === tableId(t))
      if (n) t.layout = { x: n.position.x, y: n.position.y }
    }
  }

  async function saveLayout() {
    // Positions during isolate mode are ephemeral by design — they get
    // restored when isolate is turned off. Don't persist them to the .erd file.
    if (isolate || !schema) return
    syncLayoutToSchema()

    // Static / Pages mode: keep layout in memory until the user downloads.
    if (persistSchema) return

    const payload: Record<string, { x: number; y: number }> = {}
    for (const t of schema.tables) {
      if (t.layout) payload[tableId(t)] = t.layout
    }
    try {
      await fetch('/api/layout', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(payload),
      })
    } catch (e) {
      console.warn('layout save failed', e)
    }
  }

  async function downloadErd() {
    if (!schema || !persistSchema) return
    syncLayoutToSchema()
    try {
      await persistSchema(schema)
    } catch (e) {
      error = `download failed: ${(e as Error).message}`
    }
  }

  // --- Export ----------------------------------------------------------------

  let exporting = false
  async function exportImage(format: 'png' | 'svg') {
    if (exporting) return
    exporting = true
    try {
      const el = document.querySelector('.svelte-flow') as HTMLElement | null
      if (!el) return
      const bg =
        getComputedStyle(document.body).backgroundColor || '#ffffff'
      const opts = { backgroundColor: bg, pixelRatio: 2 }
      const dataUrl =
        format === 'png' ? await toPng(el, opts) : await toSvg(el, opts)
      const a = document.createElement('a')
      const stem =
        (selected ? `${selected}-` : '') +
        (activeViewName || 'schema')
      a.href = dataUrl
      a.download = `${stem}.${format}`
      a.click()
    } catch (e) {
      console.error('export failed', e)
      error = `export failed: ${(e as Error).message}`
    } finally {
      exporting = false
    }
  }

  // --- Keyboard --------------------------------------------------------------

  function onKey(e: KeyboardEvent) {
    const editable =
      e.target instanceof HTMLInputElement ||
      e.target instanceof HTMLTextAreaElement
    if (e.key === '/' && !editable) {
      e.preventDefault()
      searchInput?.focus()
      searchInput?.select()
      return
    }
    if (e.key === 'Escape') {
      if (editable) {
        ;(e.target as HTMLElement).blur()
      } else {
        clearSelection()
      }
      return
    }
    if (editable) return
    if (e.key === 'l' || e.key === 'L') toggleLock()
    else if (e.key === 'i' || e.key === 'I') toggleIsolate()
    else if (e.key === 'f' || e.key === 'F') doFitView()
  }

  onMount(() => {
    window.addEventListener('keydown', onKey)
    load()
  })
  onDestroy(() => window.removeEventListener('keydown', onKey))
</script>

<div class="layout">
  <aside class="sidebar">
    <div class="brand">
      <img class="logo" src={logoUrl} alt="" width="22" height="22" />
      erdlens
      {#if onClose}
        <button class="tool close" title="Close file" on:click={onClose}>✕</button>
      {/if}
    </div>

    <div class="toolbar">
      <button
        class="tool"
        class:active={locked}
        title="Lock table positions (L)"
        on:click={toggleLock}
      >
        {locked ? '🔒' : '🔓'} Lock
      </button>
      <button
        class="tool"
        class:active={isolate}
        title="Show only selected table + neighbors, laid out compactly. Positions revert when off. (I)"
        on:click={toggleIsolate}
        disabled={!selected}
      >
        ◉ Isolate
      </button>
      <button class="tool" title="Fit to view (F)" on:click={doFitView}>
        ⤢ Fit
      </button>
    </div>

    <div class="toolbar">
      <button
        class="tool"
        title="Export current view as PNG"
        on:click={() => exportImage('png')}
        disabled={exporting}
      >
        ⤓ PNG
      </button>
      <button
        class="tool"
        title="Export current view as SVG"
        on:click={() => exportImage('svg')}
        disabled={exporting}
      >
        ⤓ SVG
      </button>
      {#if persistSchema}
        <button
          class="tool"
          title="Download .erd with current layout"
          on:click={downloadErd}
        >
          ⤓ .erd
        </button>
      {/if}
    </div>

    {#if availableSchemas.length > 1}
      <div class="schema-picker">
        <div class="small muted">Schemas</div>
        <div class="schema-list">
          {#each availableSchemas as s (s)}
            <label class="schema-item">
              <input
                type="checkbox"
                checked={enabledSchemas.has(s)}
                on:change={() => toggleSchema(s)}
              />
              <span>{s}</span>
            </label>
          {/each}
        </div>
      </div>
    {/if}

    {#if schema?.views && schema.views.length > 0}
      <div class="view-picker">
        <label class="small muted" for="view-select">View</label>
        <select id="view-select" bind:value={activeViewName} class="select">
          <option value="">All tables</option>
          {#each schema.views as v (v.name)}
            <option value={v.name}>{v.name}</option>
          {/each}
        </select>
      </div>
    {/if}

    <input
      bind:this={searchInput}
      class="search"
      type="search"
      placeholder="Search tables & columns…  /"
      bind:value={search}
    />

    <div class="tables">
      {#if schema}
        <div class="muted small stats">
          {filteredTables.length} / {schema.tables.length} tables · {edgeCount} FKs
          {#if matchedColumns.size > 0}
            · {[...matchedColumns.values()].reduce((n, s) => n + s.size, 0)} column hits
          {/if}
        </div>
        {#each filteredTables as t (tableId(t))}
          <button
            class="table-item"
            class:selected={selected === tableId(t)}
            data-tid={tableId(t)}
            title={t.comment || ''}
            on:click={() => selectTable(tableId(t), true)}
          >
            <div class="row-top">
              <span class="tname">{tableLabel(t)}</span>
              <span class="muted small counts">
                {t.columns.length}c · {(t.foreign_keys ?? []).length}fk
              </span>
            </div>
            {#if matchedColumns.has(tableId(t))}
              {@const hits = matchedColumns.get(tableId(t)) ?? new Set()}
              <div class="col-hits muted small">
                {[...hits].slice(0, 3).join(', ')}
                {#if hits.size > 3}
                  +{hits.size - 3}
                {/if}
              </div>
            {/if}
          </button>
        {/each}
      {:else if error}
        <div class="error">Failed to load: {error}</div>
      {:else}
        <div class="muted">Loading…</div>
      {/if}
    </div>

    <div class="footer muted small">
      {#if selected}
        <span>selected: <code>{selected}</code></span>
      {:else}
        <span
          >Press <kbd>/</kbd> search · <kbd>L</kbd> lock · <kbd>I</kbd> isolate
          · <kbd>F</kbd> fit</span
        >
      {/if}
      {#if error}
        <div class="error small">{error}</div>
      {/if}
    </div>
  </aside>

  <main class="canvas">
    <SvelteFlow
      {nodes}
      {edges}
      {nodeTypes}
      proOptions={{ hideAttribution: true }}
      fitView
      minZoom={0.05}
      maxZoom={2}
      nodesDraggable={!locked}
      nodesConnectable={false}
      elementsSelectable={true}
      on:nodeclick={onNodeClick}
      on:paneclick={onPaneClick}
      on:nodedragstop={scheduleSave}
    >
      <Background />
      <Controls />
      <MiniMap pannable zoomable />
    </SvelteFlow>
  </main>
</div>

<style>
  .layout {
    display: flex;
    height: 100vh;
    width: 100vw;
  }
  .sidebar {
    width: 300px;
    min-width: 300px;
    border-right: 1px solid var(--border);
    display: flex;
    flex-direction: column;
    padding: 12px;
    gap: 8px;
    background: var(--bg-alt);
  }
  .brand {
    font-weight: 700;
    font-size: 15px;
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .brand .close {
    margin-left: auto;
  }
  .logo {
    width: 22px;
    height: 22px;
    flex-shrink: 0;
    display: block;
  }
  .toolbar {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
  }
  .tool {
    background: var(--bg);
    color: var(--fg);
    border: 1px solid var(--border);
    padding: 4px 8px;
    border-radius: 4px;
    font-size: 11px;
    cursor: pointer;
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }
  .tool:hover:not(:disabled) {
    border-color: var(--accent);
  }
  .tool.active {
    background: var(--accent);
    color: white;
    border-color: var(--accent);
  }
  .tool:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
  .schema-picker {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .schema-list {
    display: flex;
    flex-direction: column;
    gap: 2px;
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 4px 6px;
  }
  .schema-item {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    cursor: pointer;
    padding: 2px 2px;
  }
  .schema-item input {
    margin: 0;
    accent-color: var(--accent);
  }
  .view-picker {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .select {
    background: var(--bg);
    color: var(--fg);
    border: 1px solid var(--border);
    padding: 5px 8px;
    border-radius: 4px;
    font-size: 12px;
    outline: none;
  }
  .select:focus {
    border-color: var(--accent);
  }
  .search {
    width: 100%;
    padding: 6px 10px;
    border: 1px solid var(--border);
    border-radius: 4px;
    background: var(--bg);
    color: var(--fg);
    font-size: 13px;
    outline: none;
  }
  .search:focus {
    border-color: var(--accent);
  }
  .stats {
    padding: 4px 8px 6px;
  }
  .tables {
    display: flex;
    flex-direction: column;
    overflow-y: auto;
    gap: 1px;
    flex: 1;
  }
  .table-item {
    display: flex;
    flex-direction: column;
    background: transparent;
    border: none;
    text-align: left;
    padding: 5px 8px;
    border-radius: 4px;
    color: var(--fg);
    cursor: pointer;
    font-size: 12px;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    gap: 2px;
  }
  .table-item:hover {
    background: var(--border);
  }
  .table-item.selected {
    background: var(--accent);
    color: white;
  }
  .row-top {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 6px;
  }
  .tname {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    flex: 1;
  }
  .counts {
    flex-shrink: 0;
  }
  .col-hits {
    font-size: 10px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    padding-left: 2px;
  }
  .table-item.selected .counts,
  .table-item.selected .col-hits {
    color: rgba(255, 255, 255, 0.75);
  }
  .muted {
    color: var(--muted);
  }
  .small {
    font-size: 11px;
  }
  .footer {
    padding: 8px 4px 0;
    border-top: 1px solid var(--border);
    line-height: 1.5;
  }
  .footer code {
    background: var(--bg);
    padding: 1px 4px;
    border-radius: 3px;
    font-size: 11px;
  }
  kbd {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 3px;
    padding: 0 4px;
    font-size: 10px;
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
  }
  .canvas {
    flex: 1;
    height: 100%;
    background: var(--bg);
  }

  /* Smooth node transitions when we re-layout for isolate mode. Suppressed
     during user drag so dragging feels 1:1. */
  :global(.svelte-flow__node) {
    transition: transform 0.35s cubic-bezier(0.25, 0.1, 0.25, 1);
  }
  :global(.svelte-flow__node.dragging),
  :global(.svelte-flow__node.selected.dragging) {
    transition: none;
  }
  .error {
    color: crimson;
    font-size: 12px;
  }
</style>
