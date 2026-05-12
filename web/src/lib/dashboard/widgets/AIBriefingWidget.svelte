<script lang="ts">
  import { api , todayISO } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import MarkdownRenderer from '$lib/notes/MarkdownRenderer.svelte';
  import { loadStored, saveStored } from '$lib/util/storage';

  type Cache = { date: string; markdown: string };

  // AIBriefingWidget — fires the daily-briefing AI call when the
  // user clicks "Generate". Shows the markdown response inline
  // with a "Save to today" button that appends it to today's
  // daily note. Cached client-side per-day so a refresh doesn't
  // re-bill the LLM.
  //
  // Hidden by default — the user has to enable daily_briefing in
  // Settings → AI features. The widget itself shows a hint
  // pointing at settings when disabled.

  const STORAGE_KEY = 'granit.ai.briefing.cache';

  let markdown = $state('');
  let busy = $state(false);
  let error = $state('');
  let savedToToday = $state(false);
  let abort: AbortController | null = null;

  // Cache: load on mount if today's briefing was already generated.
  let today = todayISO();
  $effect(() => {
    const cached = loadStored<Partial<Cache> | null>(STORAGE_KEY, null);
    if (cached && cached.date === today && cached.markdown) {
      markdown = cached.markdown;
    }
  });

  async function generate() {
    busy = true;
    error = '';
    savedToToday = false;
    abort = new AbortController();
    try {
      const r = await api.aiDailyBriefing(abort.signal);
      markdown = r.markdown;
      saveStored<Cache>(STORAGE_KEY, { date: today, markdown });
    } catch (err) {
      if (err instanceof DOMException && err.name === 'AbortError') {
        // User cancelled — leave any prior cached markdown alone
        // and fall back to the empty/cached state without showing
        // a scary error banner.
        error = '';
      } else {
        error = errorMessage(err);
      }
    } finally {
      busy = false;
      abort = null;
    }
  }
  function cancel() { abort?.abort(); }

  async function saveToToday() {
    if (!markdown) return;
    try {
      const daily = await api.daily('today');
      const stamp = new Date().toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit', hour12: false });
      const block = `\n\n## AI briefing · ${stamp}\n\n${markdown}\n`;
      await api.putNote(daily.path, {
        frontmatter: daily.frontmatter ?? {},
        body: (daily.body ?? '') + block
      });
      savedToToday = true;
      toast.success('Saved to today\'s daily');
    } catch (err) {
      toast.error('Save failed: ' + (errorMessage(err)));
    }
  }

  function regenerate() {
    markdown = '';
    saveStored<Cache>(STORAGE_KEY, undefined);
    void generate();
  }
</script>

<div class="bg-surface0 border border-surface1 rounded-lg p-3">
  <header class="flex items-baseline gap-2 mb-3">
    <span class="text-base">✨</span>
    <h3 class="text-sm font-medium text-text">Daily briefing</h3>
    <span class="flex-1"></span>
    {#if markdown && !busy}
      <button
        onclick={regenerate}
        class="text-[11px] text-dim hover:text-primary"
        title="Generate a fresh briefing"
      >regenerate</button>
    {/if}
  </header>

  {#if busy}
    <div class="flex items-center gap-3">
      <div class="text-sm text-dim italic flex-1">Composing…</div>
      <button
        onclick={cancel}
        class="px-2 py-1 text-xs text-warning hover:underline"
        title="Cancel the in-flight briefing"
      >cancel</button>
    </div>
  {:else if error}
    <div class="text-xs text-warning mb-2">
      {error}
    </div>
    {#if /disabled in AI preferences/i.test(error)}
      <a href="/settings" class="text-xs text-secondary hover:underline">Enable in Settings → AI features →</a>
    {:else}
      <button
        onclick={generate}
        class="px-3 py-1.5 text-xs bg-primary text-on-primary rounded"
      >Try again</button>
    {/if}
  {:else if !markdown}
    <p class="text-xs text-dim mb-3">
      One-click summary of today: events, urgent tasks, the next deadline, plus a short framing line.
    </p>
    <button
      onclick={generate}
      class="px-3 py-1.5 text-sm bg-primary text-on-primary rounded font-medium"
    >Generate today's briefing</button>
  {:else}
    <div class="prose prose-sm max-w-none mb-3">
      <MarkdownRenderer body={markdown} />
    </div>
    <div class="flex items-center gap-2">
      <button
        onclick={saveToToday}
        disabled={savedToToday}
        class="px-3 py-1.5 text-xs bg-surface1 text-secondary rounded hover:bg-surface2 disabled:opacity-50"
      >{savedToToday ? '✓ Saved to today' : 'Save to today\'s daily'}</button>
    </div>
  {/if}
</div>
