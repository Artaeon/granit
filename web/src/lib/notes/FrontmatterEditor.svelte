<script lang="ts">
  let {
    frontmatter,
    onChange
  }: {
    frontmatter: Record<string, unknown>;
    onChange?: (next: Record<string, unknown>) => void | Promise<void>;
  } = $props();

  let editingKey = $state<string | null>(null);
  let draft = $state<string>('');
  let saving = $state(false);
  let newKey = $state('');
  let newVal = $state('');

  function inferKind(v: unknown): 'string' | 'number' | 'bool' | 'list' | 'date' {
    if (typeof v === 'boolean') return 'bool';
    if (typeof v === 'number') return 'number';
    if (Array.isArray(v)) return 'list';
    if (typeof v === 'string') {
      if (/^\d{4}-\d{2}-\d{2}$/.test(v)) return 'date';
    }
    return 'string';
  }

  function fmtVal(v: unknown): string {
    if (v == null) return '';
    if (Array.isArray(v)) return v.join(', ');
    if (typeof v === 'object') return JSON.stringify(v);
    return String(v);
  }

  function parseValue(raw: string, kind: 'string' | 'number' | 'bool' | 'list' | 'date'): unknown {
    const t = raw.trim();
    if (kind === 'list') {
      return t === '' ? [] : t.split(',').map((s) => s.trim()).filter(Boolean);
    }
    if (kind === 'number') {
      const n = Number(t);
      return Number.isFinite(n) ? n : t;
    }
    if (kind === 'bool') return t === 'true' || t === '1' || t === 'yes';
    return t;
  }

  function startEdit(k: string) {
    editingKey = k;
    draft = fmtVal(frontmatter[k]);
  }
  function cancel() { editingKey = null; }

  async function commit() {
    if (editingKey == null) return;
    const k = editingKey;
    const kind = inferKind(frontmatter[k]);
    const next = { ...frontmatter, [k]: parseValue(draft, kind) };
    saving = true;
    try {
      await onChange?.(next);
      editingKey = null;
    } finally {
      saving = false;
    }
  }

  async function remove(k: string) {
    if (!confirm(`Remove property "${k}"?`)) return;
    const next = { ...frontmatter };
    delete next[k];
    await onChange?.(next);
  }

  async function addNew(e: Event) {
    e.preventDefault();
    const k = newKey.trim();
    if (!k) return;
    const kind = inferKind(newVal);
    const next = { ...frontmatter, [k]: parseValue(newVal, kind) };
    saving = true;
    try {
      await onChange?.(next);
      newKey = '';
      newVal = '';
    } finally {
      saving = false;
    }
  }

  let entries = $derived(Object.entries(frontmatter ?? {}));
</script>

<div class="space-y-1">
  {#each entries as [k, v] (k)}
    {@const kind = inferKind(v)}
    {@const isEditing = editingKey === k}
    <div class="text-sm">
      <div class="flex items-baseline gap-2">
        <span class="text-dim text-xs flex-shrink-0">{k}</span>
        <span class="text-[10px] text-dim/60 flex-shrink-0">{kind}</span>
        {#if !isEditing}
          <button class="flex-1 text-left text-text truncate hover:text-primary" onclick={() => startEdit(k)}>
            {fmtVal(v) || '—'}
          </button>
          <button onclick={() => remove(k)} aria-label="remove" class="text-dim hover:text-error opacity-0 group-hover:opacity-100">×</button>
        {/if}
      </div>
      {#if isEditing}
        <div class="flex gap-1 mt-1">
          {#if kind === 'bool'}
            <select bind:value={draft} class="flex-1 px-2 py-1 bg-mantle border border-surface1 rounded text-text text-xs">
              <option value="true">true</option>
              <option value="false">false</option>
            </select>
          {:else if kind === 'date'}
            <input type="date" bind:value={draft} class="flex-1 px-2 py-1 bg-mantle border border-surface1 rounded text-text text-xs" />
          {:else if kind === 'number'}
            <input type="number" bind:value={draft} class="flex-1 px-2 py-1 bg-mantle border border-surface1 rounded text-text text-xs" />
          {:else}
            <input type="text" bind:value={draft} class="flex-1 px-2 py-1 bg-mantle border border-surface1 rounded text-text text-xs" placeholder={kind === 'list' ? 'a, b, c' : ''} />
          {/if}
          <button onclick={commit} disabled={saving} class="px-2 py-1 bg-primary text-on-primary rounded text-xs disabled:opacity-50">save</button>
          <button onclick={cancel} class="px-2 py-1 text-dim hover:text-text text-xs">×</button>
        </div>
      {/if}
    </div>
  {/each}

  <form onsubmit={addNew} class="pt-2 mt-2 border-t border-surface1 space-y-1">
    <div class="flex gap-1">
      <input bind:value={newKey} placeholder="key" class="w-24 px-2 py-1 bg-mantle border border-surface1 rounded text-text text-xs" />
      <input bind:value={newVal} placeholder="value" class="flex-1 px-2 py-1 bg-mantle border border-surface1 rounded text-text text-xs" />
      <button type="submit" disabled={!newKey.trim() || saving} class="px-2 py-1 bg-surface1 text-subtext rounded text-xs disabled:opacity-50">+ add</button>
    </div>
  </form>
</div>
