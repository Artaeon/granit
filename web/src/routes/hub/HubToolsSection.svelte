<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type HubTool, type HubCommand } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import { focusOnMount } from '$lib/util/focusOnMount';

  // HubToolsSection renders the second half of the Hub: a curated
  // setup-command catalogue. Each tool card carries an ordered list
  // of commands the user copies into a terminal — "how do I install
  // / configure / use this program on a fresh machine".
  //
  // Keeps its own state (tools list + dialog form) so the parent
  // page-level component stays focused on the link launcher. The
  // tools section subscribes to its dedicated hub.tools.changed
  // WS event so a multi-tab edit refreshes without re-fetching
  // the (potentially long) link list above.

  let tools = $state<HubTool[]>([]);
  let loading = $state(false);
  let q = $state('');

  // Dialog state. editing === null → create; an HubTool → edit.
  let dialogOpen = $state(false);
  let editing = $state<HubTool | null>(null);

  // Form buffers — bound to inputs so cancel discards cleanly.
  let fName = $state('');
  let fDescription = $state('');
  let fIcon = $state('');
  let fColor = $state('blue');
  let fTagsInput = $state('');
  let fCommands = $state<HubCommand[]>([]);
  let saving = $state(false);

  // Drag-to-reorder state for tool cards. Same pattern as the
  // link launcher — native HTML5 drag/drop, scoped to the tools
  // grid only so a tool can't be dragged into the link list.
  let dragId = $state<string | null>(null);
  let dragOverId = $state<string | null>(null);

  // Available card colours. Maps to a Tailwind border + tint class
  // applied to the card. Kept short so the user picks decisively
  // instead of cycling through a long palette.
  const COLORS = ['blue', 'green', 'purple', 'orange', 'pink', 'red', 'yellow', 'gray'] as const;
  type Color = typeof COLORS[number];

  function colorClass(c: string | undefined): string {
    switch (c as Color) {
      case 'green':  return 'border-l-green-500';
      case 'purple': return 'border-l-purple-500';
      case 'orange': return 'border-l-orange-500';
      case 'pink':   return 'border-l-pink-500';
      case 'red':    return 'border-l-red-500';
      case 'yellow': return 'border-l-yellow-500';
      case 'gray':   return 'border-l-gray-500';
      case 'blue':
      default:       return 'border-l-blue-500';
    }
  }

  async function load() {
    loading = true;
    try {
      const r = await api.listHubTools();
      tools = r.tools;
    } catch (e) {
      toast.error('failed to load tools: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'hub.tools.changed') load();
    });
  });

  // Fuzzy-ish filter — substring match across name, description,
  // command labels/lines, and tags. Cheap O(N·M), but the
  // catalogue is small (typical < 50 tools, < 10 commands each)
  // so no need for a proper index.
  let visible = $derived.by(() => {
    const term = q.trim().toLowerCase();
    if (!term) return tools;
    return tools.filter((t) => {
      if (t.name.toLowerCase().includes(term)) return true;
      if ((t.description ?? '').toLowerCase().includes(term)) return true;
      if ((t.tags ?? []).some((tag) => tag.toLowerCase().includes(term))) return true;
      if ((t.commands ?? []).some((c) =>
        c.label.toLowerCase().includes(term) ||
        c.command.toLowerCase().includes(term)
      )) return true;
      return false;
    });
  });

  function openCreate() {
    editing = null;
    fName = '';
    fDescription = '';
    fIcon = '';
    fColor = 'blue';
    fTagsInput = '';
    fCommands = [{ label: '', command: '' }];
    dialogOpen = true;
  }

  function openEdit(t: HubTool) {
    editing = t;
    fName = t.name;
    fDescription = t.description ?? '';
    fIcon = t.icon ?? '';
    fColor = (t.color ?? 'blue') as string;
    fTagsInput = (t.tags ?? []).join(', ');
    // Deep clone so cancel doesn't mutate the on-screen list.
    fCommands = (t.commands ?? []).map((c) => ({ ...c }));
    if (fCommands.length === 0) fCommands = [{ label: '', command: '' }];
    dialogOpen = true;
  }

  function addCommandRow() {
    fCommands = [...fCommands, { label: '', command: '' }];
  }
  function removeCommandRow(i: number) {
    fCommands = fCommands.filter((_, idx) => idx !== i);
    if (fCommands.length === 0) fCommands = [{ label: '', command: '' }];
  }
  function moveCommand(i: number, dir: -1 | 1) {
    const j = i + dir;
    if (j < 0 || j >= fCommands.length) return;
    const next = [...fCommands];
    [next[i], next[j]] = [next[j], next[i]];
    fCommands = next;
  }

  async function save() {
    if (!fName.trim()) {
      toast.warning('name is required');
      return;
    }
    saving = true;
    const tags = fTagsInput
      .split(/[,\n]/)
      .map((t) => t.trim())
      .filter((t) => t.length > 0);
    const payload: Partial<HubTool> = {
      name: fName.trim(),
      description: fDescription.trim() || undefined,
      icon: fIcon.trim() || undefined,
      color: fColor || undefined,
      tags: tags.length ? tags : undefined,
      commands: fCommands
        .map((c) => ({
          label: c.label.trim(),
          command: c.command.trim(),
          notes: c.notes?.trim() || undefined
        }))
        .filter((c) => c.label || c.command)
    };
    try {
      if (editing) {
        await api.patchHubTool(editing.id, payload);
        toast.success('updated');
      } else {
        await api.createHubTool(payload);
        toast.success('tool added');
      }
      dialogOpen = false;
      await load();
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      saving = false;
    }
  }

  async function remove(t: HubTool) {
    if (!confirm(`Remove "${t.name}" from the tools catalogue?`)) return;
    try {
      await api.deleteHubTool(t.id);
      await load();
    } catch (e) {
      toast.error('delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function copyCommand(cmd: string) {
    try {
      await navigator.clipboard.writeText(cmd);
      toast.success('copied');
    } catch {
      toast.error('copy failed (clipboard blocked?)');
    }
  }

  let seeding = $state(false);
  async function seedStarter() {
    seeding = true;
    try {
      const r = await api.seedHubTools();
      if (r.added > 0) {
        toast.success(`added ${r.added} starter tool${r.added === 1 ? '' : 's'}`);
      } else {
        toast.info('starter set already loaded');
      }
      await load();
    } catch (e) {
      toast.error('seed failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      seeding = false;
    }
  }

  function fallbackIcon(t: HubTool): string {
    if (t.icon?.trim()) return t.icon.trim();
    return (t.name.trim().charAt(0) || '·').toUpperCase();
  }

  // Drag handlers — match the link launcher pattern.
  function onDragStart(id: string, ev: DragEvent) {
    dragId = id;
    if (ev.dataTransfer) {
      ev.dataTransfer.effectAllowed = 'move';
      try { ev.dataTransfer.setData('text/plain', id); } catch {}
    }
  }
  function onDragOver(id: string, ev: DragEvent) {
    if (!dragId || dragId === id) return;
    ev.preventDefault();
    if (ev.dataTransfer) ev.dataTransfer.dropEffect = 'move';
    dragOverId = id;
  }
  function onDragLeave(id: string) {
    if (dragOverId === id) dragOverId = null;
  }
  async function onDrop(targetId: string, ev: DragEvent) {
    ev.preventDefault();
    const from = dragId;
    dragId = null;
    dragOverId = null;
    if (!from || from === targetId) return;
    const ids = visible.map((t) => t.id);
    const fromIdx = ids.indexOf(from);
    const toIdx = ids.indexOf(targetId);
    if (fromIdx < 0 || toIdx < 0) return;
    const [moved] = ids.splice(fromIdx, 1);
    ids.splice(toIdx, 0, moved);
    try {
      await api.reorderHubTools(ids);
      await load();
    } catch (e) {
      toast.error('reorder failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  function onDragEnd() {
    dragId = null;
    dragOverId = null;
  }
</script>

<div class="space-y-3 mb-4">
  <div class="flex items-center gap-2">
    <input
      bind:value={q}
      placeholder="search tool name, command, tag…"
      class="flex-1 px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
    />
    {#if tools.length > 0}
      <button
        type="button"
        onclick={seedStarter}
        disabled={seeding}
        class="px-3 py-2 bg-surface0 border border-surface1 text-subtext rounded text-sm hover:border-primary hover:text-text flex-shrink-0 disabled:opacity-50"
        title="Append the curated starter set (git, Node, Docker, shell)"
      >{seeding ? 'seeding…' : '✨ Starter set'}</button>
    {/if}
    <button
      onclick={openCreate}
      class="px-3 py-2 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90 flex-shrink-0"
    >+ Add tool</button>
  </div>
</div>

{#if loading && tools.length === 0}
  <div class="text-sm text-dim">loading…</div>
{:else if tools.length === 0}
  <div class="bg-surface0 border border-surface1 rounded-lg p-6 text-center">
    <p class="text-sm text-text mb-2">No tools yet.</p>
    <p class="text-xs text-dim mb-4 max-w-md mx-auto">
      Build a catalogue of setup commands for the programs you use —
      "install via brew", "clone dotfiles", "kubectl context switch". One
      click copies each command to your clipboard.
    </p>
    <div class="flex items-center justify-center gap-2 flex-wrap">
      <button
        onclick={openCreate}
        class="px-4 py-2 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90"
      >+ Add your first tool</button>
      <button
        type="button"
        onclick={seedStarter}
        disabled={seeding}
        class="px-4 py-2 bg-surface1 border border-surface1 text-subtext rounded text-sm hover:border-primary hover:text-text disabled:opacity-50"
        title="Seed git, Node+pnpm, Docker, and shell snippets — edit freely afterward"
      >{seeding ? 'seeding…' : '✨ Load starter set'}</button>
    </div>
  </div>
{:else if visible.length === 0}
  <div class="text-sm text-dim italic">No matches.</div>
{:else}
  <ul class="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-3 gap-3">
    {#each visible as t (t.id)}
      {@const isDragSource = dragId === t.id}
      {@const isDragTarget = dragOverId === t.id && dragId !== t.id}
      <li
        draggable="true"
        ondragstart={(e) => onDragStart(t.id, e)}
        ondragover={(e) => onDragOver(t.id, e)}
        ondragleave={() => onDragLeave(t.id)}
        ondrop={(e) => onDrop(t.id, e)}
        ondragend={onDragEnd}
        class="bg-surface0 border border-surface1 rounded-lg overflow-hidden group transition-colors border-l-4 {colorClass(t.color)}
          {isDragSource ? 'opacity-40' : ''}
          {isDragTarget ? 'ring-1 ring-primary border-primary' : 'hover:border-primary'}"
      >
        <header class="px-3 py-2 flex items-start gap-2.5">
          <div class="w-9 h-9 flex-shrink-0 rounded bg-surface1 flex items-center justify-center text-sm font-medium text-text">
            {fallbackIcon(t)}
          </div>
          <div class="flex-1 min-w-0">
            <h3 class="text-sm font-medium text-text truncate">{t.name}</h3>
            {#if t.description}
              <p class="text-[11px] text-subtext line-clamp-2">{t.description}</p>
            {/if}
            {#if t.tags && t.tags.length > 0}
              <div class="flex flex-wrap gap-1 mt-1">
                {#each t.tags as tag}
                  <span class="text-[10px] text-dim bg-surface1 rounded px-1.5 py-0.5">{tag}</span>
                {/each}
              </div>
            {/if}
          </div>
          <div class="flex flex-col gap-0.5 flex-shrink-0 opacity-0 group-hover:opacity-100 transition-opacity">
            <span
              class="text-dim/60 select-none w-5 h-5 flex items-center justify-center cursor-grab active:cursor-grabbing"
              title="drag to reorder"
              aria-hidden="true"
            >⋮⋮</span>
            <button
              onclick={() => openEdit(t)}
              title="edit"
              aria-label="edit"
              class="text-dim hover:text-text text-xs leading-none w-5 h-5"
            >✎</button>
            <button
              onclick={() => remove(t)}
              title="delete"
              aria-label="delete"
              class="text-dim hover:text-error text-xs leading-none w-5 h-5"
            >×</button>
          </div>
        </header>
        {#if t.commands && t.commands.length > 0}
          <ul class="border-t border-surface1">
            {#each t.commands as c, i (i)}
              <li class="px-3 py-2 border-b border-surface1 last:border-b-0 flex items-baseline gap-2">
                <div class="flex-1 min-w-0">
                  {#if c.label}
                    <div class="text-[11px] text-subtext">{c.label}</div>
                  {/if}
                  {#if c.command}
                    <code class="text-[11px] text-text font-mono break-all">{c.command}</code>
                  {/if}
                  {#if c.notes}
                    <p class="text-[10px] text-dim mt-0.5 italic">{c.notes}</p>
                  {/if}
                </div>
                {#if c.command}
                  <button
                    type="button"
                    onclick={() => copyCommand(c.command)}
                    title="copy command"
                    aria-label="copy command"
                    class="text-dim hover:text-primary text-xs flex-shrink-0"
                  >⧉</button>
                {/if}
              </li>
            {/each}
          </ul>
        {:else}
          <div class="px-3 py-2 text-[11px] text-dim italic border-t border-surface1">
            No commands yet — edit to add some.
          </div>
        {/if}
      </li>
    {/each}
  </ul>
{/if}

<!-- Add / edit dialog -->
{#if dialogOpen}
  <div
    class="fixed inset-0 z-50 flex items-start justify-center pt-8 px-4 bg-black/60"
    onclick={() => (dialogOpen = false)}
    role="presentation"
  >
    <!-- role="dialog" + click-stop live on a wrapping div so
         svelte-check accepts the dialog semantics (a form is
         interactive, not a dialog container). The form just
         submits — Enter on any input triggers save(). -->
    <div
      class="w-full max-w-2xl bg-base border border-surface1 rounded-lg shadow-xl max-h-[90dvh] flex flex-col"
      role="dialog"
      aria-modal="true"
      aria-label={editing ? 'Edit tool' : 'Add tool'}
      tabindex="-1"
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
    >
    <form
      onsubmit={(e) => { e.preventDefault(); save(); }}
      class="flex flex-col flex-1 min-h-0"
    >
      <header class="px-3 py-2 border-b border-surface1 flex items-baseline gap-2">
        <h2 class="text-sm font-semibold text-text flex-1">
          {editing ? 'Edit tool' : 'Add tool'}
        </h2>
        <button
          type="button"
          onclick={() => (dialogOpen = false)}
          aria-label="close"
          class="text-dim hover:text-text text-lg leading-none"
        >×</button>
      </header>

      <div class="p-4 space-y-3 overflow-y-auto">
        <div class="grid grid-cols-[1fr_auto] gap-2 items-end">
          <div>
            <label for="tool-name" class="block text-xs uppercase tracking-wider text-dim mb-1">Name</label>
            <input
              id="tool-name"
              bind:value={fName}
              required
              use:focusOnMount
              placeholder="e.g. neovim"
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
            />
          </div>
          <div>
            <label for="tool-icon" class="block text-xs uppercase tracking-wider text-dim mb-1">Icon</label>
            <input
              id="tool-icon"
              bind:value={fIcon}
              placeholder="📝"
              maxlength="4"
              class="w-16 px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary text-center"
            />
          </div>
        </div>

        <div>
          <label for="tool-desc" class="block text-xs uppercase tracking-wider text-dim mb-1">Description <span class="text-dim/70 normal-case">(optional)</span></label>
          <input
            id="tool-desc"
            bind:value={fDescription}
            placeholder="One-line summary"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
          />
        </div>

        <div>
          <!-- Not a real form-label: the row is toggle buttons, not
               a single input. Span keeps the visual without lying
               about the relationship. -->
          <span class="block text-xs uppercase tracking-wider text-dim mb-1">Color</span>
          <div class="flex flex-wrap gap-1.5">
            {#each COLORS as c}
              <button
                type="button"
                onclick={() => (fColor = c)}
                class="px-2.5 py-1 text-[11px] rounded border {fColor === c ? 'border-primary text-text bg-surface0' : 'border-surface1 text-subtext hover:border-primary'}"
              >{c}</button>
            {/each}
          </div>
        </div>

        <div>
          <label for="tool-tags" class="block text-xs uppercase tracking-wider text-dim mb-1">Tags <span class="text-dim/70 normal-case">(comma-separated)</span></label>
          <input
            id="tool-tags"
            bind:value={fTagsInput}
            placeholder="editor, terminal, devops…"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
          />
        </div>

        <div>
          <div class="flex items-center justify-between mb-1">
            <span class="block text-xs uppercase tracking-wider text-dim">Commands</span>
            <button
              type="button"
              onclick={addCommandRow}
              class="text-xs text-primary hover:underline"
            >+ Add command</button>
          </div>
          <div class="space-y-2">
            {#each fCommands as c, i (i)}
              <div class="border border-surface1 rounded p-2 space-y-1.5">
                <div class="flex items-center gap-1">
                  <input
                    bind:value={c.label}
                    placeholder="Label (e.g. install via brew)"
                    class="flex-1 px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text focus:outline-none focus:border-primary"
                  />
                  <button
                    type="button"
                    onclick={() => moveCommand(i, -1)}
                    title="move up"
                    aria-label="move up"
                    disabled={i === 0}
                    class="text-dim hover:text-text text-xs w-5 h-5 disabled:opacity-30"
                  >↑</button>
                  <button
                    type="button"
                    onclick={() => moveCommand(i, 1)}
                    title="move down"
                    aria-label="move down"
                    disabled={i === fCommands.length - 1}
                    class="text-dim hover:text-text text-xs w-5 h-5 disabled:opacity-30"
                  >↓</button>
                  <button
                    type="button"
                    onclick={() => removeCommandRow(i)}
                    title="remove"
                    aria-label="remove"
                    class="text-dim hover:text-error text-xs w-5 h-5"
                  >×</button>
                </div>
                <textarea
                  bind:value={c.command}
                  rows="1"
                  placeholder="brew install neovim"
                  class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text font-mono focus:outline-none focus:border-primary resize-y"
                ></textarea>
                <input
                  bind:value={c.notes}
                  placeholder="Notes (optional)"
                  class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-[11px] text-subtext focus:outline-none focus:border-primary"
                />
              </div>
            {/each}
          </div>
        </div>
      </div>

      <footer class="px-4 py-3 border-t border-surface1 flex items-center gap-2 justify-end">
        {#if editing}
          <button
            type="button"
            onclick={() => { remove(editing!); dialogOpen = false; }}
            class="px-3 py-1.5 text-sm text-error hover:bg-surface0 rounded mr-auto"
          >Delete</button>
        {/if}
        <button
          type="button"
          onclick={() => (dialogOpen = false)}
          class="px-3 py-1.5 text-sm text-subtext hover:bg-surface0 rounded"
        >Cancel</button>
        <button
          type="submit"
          disabled={saving || !fName.trim()}
          class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50"
        >{saving ? 'saving…' : editing ? 'Save' : 'Add'}</button>
      </footer>
    </form>
    </div>
  </div>
{/if}
