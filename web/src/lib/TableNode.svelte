<script lang="ts">
  import { Handle, Position } from '@xyflow/svelte'
  import type { Table } from './types'

  export let data: {
    table: Table
    highlighted?: boolean
    dimmed?: boolean
    matched?: Set<string>
    related?: Set<string>
  }

  $: pkSet = new Set(data.table.primary_key ?? [])
  $: fkColumns = new Set((data.table.foreign_keys ?? []).flatMap((fk) => fk.columns))
  $: matched = data.matched ?? new Set<string>()
  $: related = data.related ?? new Set<string>()
</script>

<div
  class="table-node"
  class:highlighted={data.highlighted}
  class:dimmed={data.dimmed}
  class:has-match={matched.size > 0}
  title={data.table.comment || ''}
>
  <div class="header">{data.table.name}</div>
  <div class="body">
    {#each data.table.columns as col}
      <div
        class="row"
        class:matched={matched.has(col.name)}
        class:related={related.has(col.name)}
        title={col.comment || ''}
      >
        <!-- Column-level handles: an edge from another table can terminate on
             this exact row via targetHandle/sourceHandle = column name. -->
        <Handle
          type="target"
          position={Position.Left}
          id={col.name}
          class="row-handle"
        />
        <span class="col-name">
          {#if pkSet.has(col.name)}<span class="badge pk">PK</span>{/if}
          {#if fkColumns.has(col.name)}<span class="badge fk">FK</span>{/if}
          {col.name}
        </span>
        <span class="col-type">{col.type}</span>
        <Handle
          type="source"
          position={Position.Right}
          id={col.name}
          class="row-handle"
        />
      </div>
    {/each}
  </div>
</div>

<style>
  .table-node {
    background: var(--bg);
    border: 1px solid var(--border);
    border-radius: 6px;
    width: 260px;
    font-size: 12px;
    box-shadow: 0 1px 2px rgba(0, 0, 0, 0.04);
    overflow: hidden;
    transition: opacity 0.15s ease, box-shadow 0.15s ease, border-color 0.15s ease;
  }
  .table-node.highlighted {
    border-color: var(--accent);
    box-shadow: 0 0 0 2px var(--accent), 0 4px 12px rgba(0, 0, 0, 0.1);
  }
  .table-node.has-match:not(.highlighted) {
    border-color: var(--fk);
  }
  .table-node.dimmed {
    opacity: 0.35;
    filter: saturate(0.4);
  }
  .header {
    padding: 8px 10px;
    font-weight: 600;
    background: var(--bg-alt);
    border-bottom: 1px solid var(--border);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 12px;
  }
  .highlighted .header {
    background: var(--accent);
    color: white;
  }
  .body {
    display: flex;
    flex-direction: column;
  }
  .row {
    position: relative;
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
    padding: 3px 10px;
    border-bottom: 1px solid var(--border);
    transition: background 0.12s ease;
  }
  .row:last-child {
    border-bottom: none;
  }
  .row.matched {
    background: rgba(96, 165, 250, 0.22);
  }
  .row.related {
    background: rgba(59, 130, 246, 0.14);
    box-shadow: inset 3px 0 0 var(--accent);
  }
  .row.related .col-name {
    color: var(--accent);
    font-weight: 600;
  }
  .row.related.matched {
    background: rgba(96, 165, 250, 0.28);
  }
  .col-name {
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 12px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .col-type {
    color: var(--muted);
    font-family: ui-monospace, SFMono-Regular, Menlo, monospace;
    font-size: 11px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 100px;
  }
  .badge {
    display: inline-block;
    font-size: 9px;
    padding: 1px 4px;
    border-radius: 3px;
    margin-right: 4px;
    font-weight: 700;
    vertical-align: middle;
    color: #000;
  }
  .badge.pk {
    background: var(--pk);
  }
  .badge.fk {
    background: var(--fk);
  }

  /* Global: hide handle dots but keep them functional as edge anchors. */
  :global(.svelte-flow__handle.row-handle) {
    width: 8px;
    height: 8px;
    min-width: 8px;
    min-height: 8px;
    opacity: 0;
    background: var(--accent);
    border: none;
  }
</style>
