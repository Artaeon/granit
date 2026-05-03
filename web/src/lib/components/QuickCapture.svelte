<script lang="ts">
  import { api } from '$lib/api';

  let { notePath, section = '## Tasks', onCreated }: { notePath: string; section?: string; onCreated?: () => void } = $props();

  let text = $state('');
  let priority = $state(0);
  let dueDate = $state('');
  let busy = $state(false);
  let error = $state('');

  async function submit(e: Event) {
    e.preventDefault();
    if (!text.trim()) return;
    busy = true;
    error = '';
    try {
      await api.createTask({
        notePath,
        text: text.trim(),
        priority: priority || undefined,
        dueDate: dueDate || undefined,
        section
      });
      text = '';
      priority = 0;
      dueDate = '';
      onCreated?.();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }
</script>

<form onsubmit={submit} class="bg-surface0 border border-surface1 rounded p-3 space-y-2">
  <input
    bind:value={text}
    placeholder="add a task to today's note…"
    class="w-full bg-transparent text-base sm:text-sm text-text placeholder-dim focus:outline-none"
    disabled={busy}
  />
  <div class="flex flex-wrap items-center gap-2 text-xs text-dim">
    <select bind:value={priority} class="bg-mantle border border-surface1 rounded px-2 py-1.5 text-text text-sm">
      <option value={0}>no priority</option>
      <option value={1}>!1 high</option>
      <option value={2}>!2 med</option>
      <option value={3}>!3 low</option>
    </select>
    <input
      type="date"
      bind:value={dueDate}
      class="bg-mantle border border-surface1 rounded px-2 py-1.5 text-text text-sm"
    />
    <span class="flex-1 min-w-0"></span>
    {#if error}<span class="text-error truncate">{error}</span>{/if}
    <button
      type="submit"
      disabled={busy || !text.trim()}
      class="px-4 py-1.5 bg-primary text-on-primary rounded font-medium text-sm disabled:opacity-50"
    >
      {busy ? '…' : 'add'}
    </button>
  </div>
</form>
