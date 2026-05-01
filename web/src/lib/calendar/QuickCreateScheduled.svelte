<script lang="ts">
  import { api, todayISO } from '$lib/api';

  let {
    open = $bindable(false),
    date,
    hour = 9,
    minute = 0,
    defaultNotePath,
    onCreated
  }: {
    open?: boolean;
    date: Date;
    hour?: number;
    minute?: number;
    defaultNotePath?: string;
    onCreated?: () => void;
  } = $props();

  let text = $state('');
  // Initialized empty; $effect below seeds it from the prop on each open.
  let notePath = $state('');
  let duration = $state(60);
  let priority = $state(0);
  let busy = $state(false);
  let error = $state('');

  $effect(() => {
    if (open) {
      text = '';
      notePath = defaultNotePath ?? `${todayISO()}.md`;
      duration = 60;
      priority = 0;
      error = '';
    }
  });

  let timeStr = $derived(
    `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`
  );
  let dateStr = $derived(date.toLocaleDateString(undefined, { weekday: 'short', month: 'short', day: 'numeric' }));

  async function submit(e: Event) {
    e.preventDefault();
    if (!text.trim()) return;
    busy = true;
    error = '';
    try {
      const start = new Date(date);
      start.setHours(hour, minute, 0, 0);
      await api.createTask({
        notePath,
        text: text.trim(),
        priority: priority || undefined,
        section: '## Tasks',
        scheduledStart: start.toISOString(),
        durationMinutes: duration
      });
      open = false;
      onCreated?.();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  function close() { open = false; }
  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') close();
  }
</script>

{#if open}
  <div
    class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4"
    onclick={close}
    onkeydown={onKey}
    role="dialog"
    tabindex="-1"
  >
    <div
      role="presentation"
      onclick={(e) => e.stopPropagation()}
      onkeydown={(e) => e.stopPropagation()}
      class="w-full max-w-md"
    >
    <form
      onsubmit={submit}
      class="bg-mantle border border-surface1 rounded-lg p-5 space-y-3"
    >
      <h2 class="text-base font-semibold text-text">New scheduled task</h2>
      <div class="text-sm text-dim">{dateStr} at {timeStr}</div>
      <input
        bind:value={text}
        placeholder="title…"
        class="w-full px-3 py-2.5 bg-surface0 border border-surface1 rounded text-base sm:text-sm text-text focus:outline-none focus:border-primary"
      />
      <div class="grid grid-cols-2 gap-2">
        <label class="text-xs text-dim">
          duration (min)
          <input type="number" min="5" step="5" bind:value={duration} class="mt-1 w-full px-2 py-2 bg-surface0 border border-surface1 rounded text-text text-base sm:text-sm" />
        </label>
        <label class="text-xs text-dim">
          priority
          <select bind:value={priority} class="mt-1 w-full px-2 py-2 bg-surface0 border border-surface1 rounded text-text text-base sm:text-sm">
            <option value={0}>none</option>
            <option value={1}>!1 high</option>
            <option value={2}>!2 med</option>
            <option value={3}>!3 low</option>
          </select>
        </label>
      </div>
      <label class="block text-xs text-dim">
        attach to note (path)
        <input bind:value={notePath} class="mt-1 w-full px-2 py-2 bg-surface0 border border-surface1 rounded text-text font-mono text-xs" />
      </label>
      {#if error}<div class="text-sm text-error">{error}</div>{/if}
      <div class="flex gap-2 justify-end">
        <button type="button" onclick={close} class="px-3 py-1.5 text-sm text-subtext hover:text-text">cancel</button>
        <button type="submit" disabled={busy || !text.trim()} class="px-3 py-1.5 text-sm bg-primary text-mantle rounded disabled:opacity-50">
          {busy ? 'creating…' : 'create'}
        </button>
      </div>
    </form>
    </div>
  </div>
{/if}
