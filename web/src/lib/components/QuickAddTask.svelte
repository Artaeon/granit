<script lang="ts">
  import { api } from '$lib/api';
  import { parseTaskInput } from '$lib/util/taskParse';
  import { toast } from '$lib/components/toast';

  let {
    notePath,
    section = '## Tasks',
    onCreated
  }: {
    notePath: string;
    section?: string;
    onCreated?: () => void;
  } = $props();

  let raw = $state('');
  let busy = $state(false);
  let inputEl: HTMLInputElement | undefined = $state();

  let parsed = $derived(parseTaskInput(raw));

  function priorityClass(p: number): string {
    if (p === 1) return 'bg-error/20 text-error border-error/30';
    if (p === 2) return 'bg-warning/20 text-warning border-warning/30';
    if (p === 3) return 'bg-info/20 text-info border-info/30';
    return '';
  }

  async function submit(e?: Event) {
    e?.preventDefault();
    const t = parsed.text.trim();
    if (!t) return;
    busy = true;
    try {
      await api.createTask({
        notePath,
        text: t,
        priority: parsed.priority || undefined,
        dueDate: parsed.dueDate || undefined,
        tags: parsed.tags.length ? parsed.tags : undefined,
        section
      });
      raw = '';
      toast.success('task added');
      onCreated?.();
      inputEl?.focus();
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      toast.error(`add failed: ${msg}`);
    } finally {
      busy = false;
    }
  }

  // Keyboard hint visibility
  let showHint = $state(false);
  let hasMarkers = $derived(parsed.priority > 0 || parsed.dueDate !== '' || parsed.tags.length > 0);
</script>

<form onsubmit={submit} class="bg-surface0 border border-surface1 rounded-lg overflow-hidden focus-within:border-primary/60 transition-colors">
  <div class="flex items-center gap-2 px-3 py-2.5">
    <span class="text-dim text-lg leading-none flex-shrink-0">＋</span>
    <input
      bind:this={inputEl}
      bind:value={raw}
      onfocus={() => (showHint = true)}
      onblur={() => (showHint = false)}
      placeholder="Add a task —  !1 high  ·  due:2026-05-15  ·  #tag"
      class="flex-1 bg-transparent text-base sm:text-sm text-text placeholder-dim focus:outline-none"
      disabled={busy}
    />
    {#if raw.trim()}
      <button
        type="submit"
        disabled={busy || !parsed.text}
        class="px-3 py-1 bg-primary text-mantle rounded text-sm font-medium disabled:opacity-50 flex-shrink-0"
      >
        {busy ? '…' : 'add'}
        <kbd class="hidden sm:inline ml-1 text-[10px] opacity-70">↵</kbd>
      </button>
    {/if}
  </div>

  {#if hasMarkers}
    <div class="flex flex-wrap items-center gap-1.5 px-3 pb-2 pt-1 border-t border-surface1/50 text-xs">
      <span class="text-dim text-[10px] uppercase tracking-wider">parsed</span>
      {#if parsed.priority > 0}
        <span class="px-2 py-0.5 rounded border {priorityClass(parsed.priority)}">P{parsed.priority}</span>
      {/if}
      {#if parsed.dueDate}
        <span class="px-2 py-0.5 rounded bg-surface1 text-secondary">📅 {parsed.dueDate}</span>
      {/if}
      {#each parsed.tags as t}
        <span class="px-2 py-0.5 rounded bg-surface1 text-accent">#{t}</span>
      {/each}
    </div>
  {:else if showHint && !raw.trim()}
    <div class="px-3 pb-2 pt-1 text-[11px] text-dim leading-relaxed border-t border-surface1/50">
      <span class="text-subtext">Smart syntax:</span>
      <code class="text-error">!1</code>/<code class="text-warning">!2</code>/<code class="text-info">!3</code> priority ·
      <code class="text-secondary">due:2026-05-15</code> ·
      <code class="text-accent">#tag</code>
    </div>
  {/if}
</form>
