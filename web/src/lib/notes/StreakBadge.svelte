<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // Small status-bar pill that surfaces the user's consecutive-day
  // daily-note streak. Read-only — never blocks the editor, never
  // gates a save. Refreshes on note.changed events for any daily-
  // shaped path (debounced) so a fresh daily note bumps the count
  // without a manual reload.
  //
  // Visibility: hidden when current=0 AND longest<2. A user with
  // no daily-note habit shouldn't see an empty "0" badge cluttering
  // the chrome. Once they have any history, the badge stays visible
  // so the streak doesn't pop in/out as today's logged toggles.

  // Parametric source so the same badge surfaces either streak —
  // daily notes (the editor status bar) OR bible reading (the
  // scripture page). Both endpoints return identical shapes so a
  // single fetch+render path works for either; the only per-source
  // difference is which fetch function + which WS path to listen on.
  interface Props {
    source?: 'daily' | 'bible';
  }
  let { source = 'daily' }: Props = $props();

  let current = $state(0);
  let longest = $state(0);
  let lastDate = $state<string | null>(null);
  let todayLogged = $state(false);
  let loaded = $state(false);

  async function refresh() {
    try {
      const r = source === 'bible' ? await api.bibleStreak() : await api.dailyStreak();
      current = r.current;
      longest = r.longest;
      lastDate = r.lastDate ?? null;
      todayLogged = r.todayLogged;
      loaded = true;
    } catch {
      // Silent failure — the badge isn't load-bearing for any
      // user workflow; if /streak is briefly unavailable, hide.
      loaded = false;
    }
  }

  // YYYY-MM-DD anywhere in the path is the cheap heuristic that
  // catches both vault-root dailies (2026-05-11.md) and
  // folder-prefixed ones (daily/2026-05-11.md, journal/2026-05-11.md).
  // Matches the regex shape the backend's jotPathRegex uses.
  const dailyPathRe = /\d{4}-\d{2}-\d{2}\.md$/;
  // The bible-reading log lives at a fixed sidecar path; any
  // state.changed on it means the streak number may have moved.
  const bibleLogPath = '.granit/bible-reading-log.json';

  let debounceTimer: ReturnType<typeof setTimeout> | null = null;
  function scheduleRefresh() {
    if (debounceTimer) clearTimeout(debounceTimer);
    // 800ms — long enough to coalesce a save's own bounce-back and
    // the file-watcher event that follows, short enough that a user
    // saving and then glancing at the status bar sees the new
    // number before they look away.
    debounceTimer = setTimeout(() => {
      debounceTimer = null;
      void refresh();
    }, 800);
  }

  let off: (() => void) | null = null;
  onMount(() => {
    void refresh();
    off = onWsEvent((ev) => {
      if (source === 'bible') {
        if (ev.type !== 'state.changed' || ev.path !== bibleLogPath) return;
      } else {
        if (ev.type !== 'note.changed' && ev.type !== 'note.removed') return;
        if (!ev.path || !dailyPathRe.test(ev.path)) return;
      }
      scheduleRefresh();
    });
  });
  onDestroy(() => {
    if (off) off();
    if (debounceTimer) clearTimeout(debounceTimer);
  });

  // Tooltip surfaces the longer view without taking visual space.
  let title = $derived.by(() => {
    if (!loaded) return '';
    const parts: string[] = [];
    if (current > 0) parts.push(`${current}-day streak`);
    else parts.push('No active streak');
    if (longest > current) parts.push(`longest ${longest}`);
    if (lastDate) parts.push(`last entry ${lastDate}`);
    parts.push(todayLogged ? 'today logged' : 'today pending');
    return parts.join(' · ');
  });

  let visible = $derived(loaded && (current > 0 || longest >= 2));
</script>

{#if visible}
  <span
    class="inline-flex items-center gap-1 px-1.5 py-0.5 rounded text-[11px] tabular-nums {todayLogged
      ? 'text-success bg-success/10'
      : 'text-subtext bg-surface1'}"
    {title}
    aria-label={title}
  >
    <!-- Flame for the active streak; circle for "today pending". The
         icon shape carries the same semantic as the color, so users
         with monochrome themes / colorblindness still see the state. -->
    {#if todayLogged}
      <svg viewBox="0 0 24 24" fill="currentColor" class="w-3 h-3">
        <path d="M12 2s4 4.5 4 8a4 4 0 11-8 0c0-2 1-4 2-5-.5 2 1 3 2 3-1-2 0-4 0-6zm-1 14a3 3 0 116 0c0 2-3 5-3 5s-3-3-3-5z"/>
      </svg>
    {:else}
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" class="w-3 h-3">
        <circle cx="12" cy="12" r="9"/>
      </svg>
    {/if}
    <span>{current}</span>
    {#if longest > current}
      <span class="text-dim">/ {longest}</span>
    {/if}
  </span>
{/if}
