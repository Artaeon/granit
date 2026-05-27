<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { inlineMd } from '$lib/util/inlineMd';
  import { onLocalMidnight } from '$lib/util/midnightTick';

  // TodayFocusWidget surfaces the user's "what would make today a win"
  // commitment from the daily note. We look for a `## Plan` (or
  // `## Daily Plan`) section. If we find one, we extract the goal and
  // the first checklist; if we don't, we render a CTA pointing at the
  // morning routine. Cheap parser — no Markdown AST, just line-by-line.

  let daily = $state<Note | null>(null);
  let loading = $state(false);

  async function load() {
    loading = true;
    try {
      daily = await api.daily('today');
    } catch {
      daily = null;
    } finally {
      loading = false;
    }
  }
  let stopMidnight: (() => void) | null = null;
  onMount(() => {
    load();
    // Switch to the new day's daily note at local midnight so the
    // focus statement refreshes with the user's new morning plan
    // (or empties out for a freshly-rolled day).
    stopMidnight = onLocalMidnight(() => { void load(); });
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' && daily && ev.path === daily.path) load();
    });
  });
  onDestroy(() => { if (stopMidnight) stopMidnight(); });

  // Pull lines from the first matching `## Plan` / `## Daily Plan` /
  // `## Today's Plan` heading until the next `## ` heading. Returns the
  // raw section body (no leading heading) or '' if the section isn't
  // present.
  function extractPlanSection(body: string): string {
    const lines = body.split('\n');
    let start = -1;
    for (let i = 0; i < lines.length; i++) {
      const m = lines[i].match(/^##\s+(.*)$/);
      if (!m) continue;
      const h = m[1].trim().toLowerCase();
      if (h === 'plan' || h === 'daily plan' || h === "today's plan") {
        start = i + 1;
        break;
      }
    }
    if (start === -1) return '';
    const out: string[] = [];
    for (let i = start; i < lines.length; i++) {
      if (/^##\s+/.test(lines[i])) break;
      out.push(lines[i]);
    }
    return out.join('\n').trim();
  }

  // From the section text pull a one-line "goal" — first non-empty,
  // non-list line. We skip task lines (`- [ ]`) since those are picked
  // tasks, not the focus statement.
  function extractGoal(section: string): string {
    if (!section) return '';
    for (const raw of section.split('\n')) {
      const line = raw.trim();
      if (!line) continue;
      if (/^- \[/.test(line)) continue; // task checkbox
      if (/^>/.test(line)) continue;     // blockquote (scripture)
      // Strip "Goal:" / "**Goal:**" prefixes the morning save adds.
      return line.replace(/^\*?\*?goal:?\*?\*?\s*/i, '').replace(/^[-*]\s*/, '').trim();
    }
    return '';
  }

  // First few task lines so the widget echoes the commitments, not the
  // entire daily note.
  function extractTasks(section: string, limit = 3): string[] {
    if (!section) return [];
    const out: string[] = [];
    for (const raw of section.split('\n')) {
      const m = raw.match(/^- \[([ x])\]\s+(.+)$/);
      if (!m) continue;
      out.push((m[1] === 'x' ? '✓ ' : '') + m[2].trim());
      if (out.length >= limit) break;
    }
    return out;
  }

  let section = $derived(daily?.body ? extractPlanSection(daily.body) : '');
  let goal = $derived(section ? extractGoal(section) : '');
  let tasks = $derived(section ? extractTasks(section) : []);
</script>

<!-- Today's focus reads as a banner statement, not a list item.
     When a goal is set, the line gets larger text-lg/xl + medium
     weight + tight leading so it lands like a daily mantra. Tasks
     beneath are smaller chevron-bulleted lines — a reminder of the
     committed scope without competing with the goal itself. Empty
     state surfaces a single CTA so the user has a one-tap path
     back to /morning. -->
<section class="bg-surface0 border border-surface1 rounded-lg p-3 sm:p-4 flex flex-col h-full">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-[10px] uppercase tracking-wider text-dim font-medium">Today's focus</h2>
    {#if daily && section}
      <a href="/notes/{encodeURIComponent(daily.path)}" class="text-xs text-secondary hover:underline">edit →</a>
    {:else}
      <a href="/morning" class="text-xs text-secondary hover:underline">plan →</a>
    {/if}
  </div>

  {#if loading}
    <div class="text-sm text-dim">loading…</div>
  {:else if section}
    {#if goal}
      <!-- The goal line is the visual heart of the widget. text-lg
           jumps from the default text-base so the eye lands here
           first; leading-snug stops a multi-word goal from spreading
           too wide vertically. text-text (not subtext) so it reads
           as primary content. -->
      <p class="text-lg sm:text-xl text-text font-medium leading-snug mb-2.5">{@html inlineMd(goal)}</p>
    {/if}
    {#if tasks.length > 0}
      <!-- Tasks beneath. text-subtext + smaller size so they recede
           behind the goal. Chevron bullet ('›') reads as "next /
           continuing" — same glyph the editor breadcrumb uses. -->
      <ul class="space-y-1 text-sm text-subtext">
        {#each tasks as t}
          <li class="flex items-baseline gap-1.5 truncate">
            <span class="text-dim flex-shrink-0" aria-hidden="true">›</span>
            <span class="truncate">{@html inlineMd(t)}</span>
          </li>
        {/each}
      </ul>
    {:else if !goal}
      <p class="text-sm text-dim italic">Plan section is empty. <a href="/morning" class="text-secondary hover:underline">Plan today →</a></p>
    {/if}
  {:else}
    <!-- No daily-plan section at all. Centre the CTA so the empty
         state reads as deliberate breathing room, not visual junk. -->
    <div class="flex-1 flex flex-col items-center justify-center text-center py-2 gap-2">
      <p class="text-sm text-dim">No focus set for today.</p>
      <a href="/morning" class="px-3 py-1.5 text-xs bg-primary text-on-primary rounded font-medium hover:opacity-90">
        Set a focus →
      </a>
    </div>
  {/if}
</section>
