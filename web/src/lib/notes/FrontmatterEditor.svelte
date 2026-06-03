<script lang="ts">
  import { todayISO } from '$lib/api';

  let {
    frontmatter,
    onChange
  }: {
    frontmatter: Record<string, unknown>;
    onChange?: (next: Record<string, unknown>) => void | Promise<unknown>;
  } = $props();

  let editingKey = $state<string | null>(null);
  let draft = $state<string>('');
  let saving = $state(false);
  let newKey = $state('');
  let newVal = $state('');
  // Tag-chip input buffer — what the user is typing into the
  // "add a tag" field of a list editor. Separate from `draft` so
  // confirming a chip doesn't lose the chip set.
  let chipBuf = $state<string>('');

  // Quick-pick presets for common frontmatter fields. The "+ add"
  // form shows these as one-click buttons that pre-fill the key
  // (and a sensible default value), so the user doesn't have to
  // type "tags" / "status" / etc. by hand for the 90% case. The
  // FIELD_PRESETS shape is intentionally light — kind drives the
  // editor type, defaultValue seeds the input, and label is the
  // chip caption.
  const FIELD_PRESETS: { key: string; label: string; kind: 'list' | 'string' | 'date' | 'bool'; defaultValue: string }[] = [
    { key: 'tags', label: 'tags', kind: 'list', defaultValue: '' },
    { key: 'status', label: 'status', kind: 'string', defaultValue: 'active' },
    { key: 'type', label: 'type', kind: 'string', defaultValue: '' },
    { key: 'project', label: 'project', kind: 'string', defaultValue: '' },
    { key: 'venture', label: 'venture', kind: 'string', defaultValue: '' },
    { key: 'goal_id', label: 'goal_id', kind: 'string', defaultValue: '' },
    { key: 'date', label: 'date', kind: 'date', defaultValue: todayISO() },
    { key: 'pinned', label: 'pinned', kind: 'bool', defaultValue: 'true' }
  ];

  // One-click status values — common project/note states. Surfaced
  // as a row of chips beneath the value editor when the field name
  // is "status" so the user just clicks instead of typing.
  const STATUS_PRESETS = ['active', 'paused', 'completed', 'archived', 'draft', 'done'];

  function inferKind(v: unknown): 'string' | 'number' | 'bool' | 'list' | 'date' {
    if (typeof v === 'boolean') return 'bool';
    if (typeof v === 'number') return 'number';
    if (Array.isArray(v)) return 'list';
    if (typeof v === 'string') {
      if (/^\d{4}-\d{2}-\d{2}$/.test(v)) return 'date';
    }
    return 'string';
  }

  // Treat list-y keys as lists even when their on-disk form is a
  // single-element string (some legacy notes have `tags: foo` not
  // `tags: [foo]`). Without this, the chip UI never engages on the
  // tags field of older notes — frustrating because tags is the
  // most-edited field.
  function effectiveKind(k: string, v: unknown): ReturnType<typeof inferKind> {
    if (k === 'tags' || k === 'aliases') return 'list';
    return inferKind(v);
  }

  function asChips(v: unknown): string[] {
    if (Array.isArray(v)) return v.map((x) => String(x).trim()).filter(Boolean);
    if (typeof v === 'string' && v.trim()) return [v.trim()];
    return [];
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

  // Click a preset chip → seed the new-field form so the user only
  // has to confirm the value. For lists we leave the value empty so
  // they immediately type tags; for status we pre-pick "active"; for
  // bool we pre-pick "true". Saves a step on the most common edits.
  function applyPreset(p: typeof FIELD_PRESETS[number]) {
    if (Object.prototype.hasOwnProperty.call(frontmatter, p.key)) {
      // Already set — start editing the existing field instead of
      // adding a duplicate. The button in the chip row visually
      // signals this state via the disabled flag.
      startEdit(p.key);
      return;
    }
    newKey = p.key;
    newVal = p.defaultValue;
  }

  // Inline tag-chip helpers. Adding a chip uses the in-flight
  // chipBuf state, removing splices by index. We persist after every
  // mutation so the typing → save path matches what the user
  // expects (no "save" button hunt for chips).
  async function addChip(k: string) {
    const t = chipBuf.trim();
    if (!t) return;
    const cur = asChips(frontmatter[k]);
    if (cur.some((x) => x.toLowerCase() === t.toLowerCase())) {
      chipBuf = '';
      return;
    }
    const next = { ...frontmatter, [k]: [...cur, t] };
    chipBuf = '';
    saving = true;
    try {
      await onChange?.(next);
    } finally {
      saving = false;
    }
  }

  async function removeChip(k: string, idx: number) {
    const cur = asChips(frontmatter[k]);
    const next = { ...frontmatter, [k]: cur.filter((_, i) => i !== idx) };
    saving = true;
    try {
      await onChange?.(next);
    } finally {
      saving = false;
    }
  }

  function onChipKeydown(e: KeyboardEvent, k: string) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      addChip(k);
    } else if (e.key === 'Backspace' && chipBuf === '') {
      // Empty buffer + Backspace → pop the last chip. Standard
      // chip-input UX (Notion / Capacities / Linear).
      const cur = asChips(frontmatter[k]);
      if (cur.length > 0) {
        e.preventDefault();
        removeChip(k, cur.length - 1);
      }
    }
  }

  // One-click status setter — only renders when the active key is
  // exactly "status".
  async function setStatus(k: string, value: string) {
    const next = { ...frontmatter, [k]: value };
    saving = true;
    try {
      await onChange?.(next);
      editingKey = null;
    } finally {
      saving = false;
    }
  }

  let entries = $derived(Object.entries(frontmatter ?? {}));
  let presetsAvailable = $derived(
    FIELD_PRESETS.filter((p) => !Object.prototype.hasOwnProperty.call(frontmatter ?? {}, p.key))
  );
</script>

<div class="space-y-1">
  {#each entries as [k, v] (k)}
    {@const kind = effectiveKind(k, v)}
    {@const isEditing = editingKey === k}
    {@const chips = kind === 'list' ? asChips(v) : []}
    <div class="text-sm group">
      <div class="flex items-baseline gap-2">
        <span class="text-dim text-xs flex-shrink-0">{k}</span>
        <span class="text-[10px] text-dim/60 flex-shrink-0">{kind}</span>
        {#if kind === 'list'}
          <!-- Tag-chip rendering: every list field (tags, aliases,
               anything inferred as a list) shows its members as
               removable chips, with an inline "add" input. The
               legacy comma-list editor is replaced for list kinds
               since chips are the obvious UX. -->
          <div class="flex-1 min-w-0 flex flex-wrap items-center gap-1">
            {#each chips as chip, idx (chip + ':' + idx)}
              <span class="inline-flex items-center gap-1 px-1.5 py-0.5 bg-surface1 text-secondary rounded text-[11px]">
                <span>{chip}</span>
                <button
                  onclick={() => removeChip(k, idx)}
                  disabled={saving}
                  aria-label="remove {chip}"
                  class="text-subtext hover:text-error leading-none disabled:opacity-50"
                >×</button>
              </span>
            {/each}
            <input
              bind:value={chipBuf}
              onkeydown={(e) => onChipKeydown(e, k)}
              onblur={() => addChip(k)}
              placeholder={chips.length === 0 ? 'add a tag…' : '+'}
              class="flex-1 min-w-[6rem] bg-transparent text-xs text-text placeholder-dim focus:outline-none"
            />
          </div>
          <button onclick={() => remove(k)} aria-label="remove" class="text-dim hover:text-error opacity-0 group-hover:opacity-100 flex-shrink-0">×</button>
        {:else if !isEditing}
          <button class="flex-1 text-left text-text truncate hover:text-primary" onclick={() => startEdit(k)}>
            {fmtVal(v) || '—'}
          </button>
          <button onclick={() => remove(k)} aria-label="remove" class="text-dim hover:text-error opacity-0 group-hover:opacity-100">×</button>
        {/if}
      </div>
      {#if isEditing && kind !== 'list'}
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
            <input type="text" bind:value={draft} class="flex-1 px-2 py-1 bg-mantle border border-surface1 rounded text-text text-xs" />
          {/if}
          <button onclick={commit} disabled={saving} class="px-3 py-1.5 sm:px-2 sm:py-1 bg-primary text-on-primary rounded text-xs disabled:opacity-50 min-w-[44px] min-h-[36px] sm:min-w-0 sm:min-h-0">save</button>
          <button onclick={cancel} class="px-3 py-1.5 sm:px-2 sm:py-1 text-dim hover:text-text text-xs min-w-[44px] min-h-[36px] sm:min-w-0 sm:min-h-0">×</button>
        </div>
        {#if k === 'status'}
          <!-- One-click status presets — saves the click + type +
               save sequence to a single tap. -->
          <div class="flex flex-wrap gap-1 mt-1.5">
            {#each STATUS_PRESETS as s}
              <button
                onclick={() => setStatus(k, s)}
                disabled={saving}
                class="px-1.5 py-0.5 text-[11px] rounded {String(v) === s ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'} disabled:opacity-50"
              >{s}</button>
            {/each}
          </div>
        {/if}
      {/if}
    </div>
  {/each}

  {#if presetsAvailable.length > 0}
    <!-- Preset chip row — one-click "+ tags / + status / + project"
         for the most-edited frontmatter fields. Already-present
         keys are filtered out so the row only offers what's
         actually missing on this note. -->
    <div class="flex flex-wrap gap-1 pt-2 mt-2 border-t border-surface1">
      <span class="text-[10px] text-dim self-center">+ add</span>
      {#each presetsAvailable as p}
        <button
          onclick={() => applyPreset(p)}
          disabled={saving}
          class="px-1.5 py-0.5 text-[11px] rounded bg-surface0 text-subtext hover:bg-surface1 hover:text-text disabled:opacity-50"
          title="add {p.key}"
        >{p.label}</button>
      {/each}
    </div>
  {/if}

  <form onsubmit={addNew} class="pt-2 mt-2 border-t border-surface1 space-y-1">
    <div class="flex gap-1">
      <input bind:value={newKey} placeholder="key" class="w-24 px-2 py-1 bg-mantle border border-surface1 rounded text-text text-xs" />
      <input bind:value={newVal} placeholder="value" class="flex-1 px-2 py-1 bg-mantle border border-surface1 rounded text-text text-xs" />
      <button type="submit" disabled={!newKey.trim() || saving} class="px-2 py-1 bg-surface1 text-subtext rounded text-xs disabled:opacity-50">+ add</button>
    </div>
  </form>
</div>
