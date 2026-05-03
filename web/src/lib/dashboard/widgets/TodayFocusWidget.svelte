<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Note } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { inlineMd } from '$lib/util/inlineMd';

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
  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' && daily && ev.path === daily.path) load();
    });
  });

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

<section class="bg-surface0 border border-surface1 rounded-lg p-4 flex flex-col">
  <div class="flex items-baseline justify-between mb-2">
    <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Today's focus</h2>
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
      <p class="text-base text-text font-medium leading-snug mb-2">{@html inlineMd(goal)}</p>
    {/if}
    {#if tasks.length > 0}
      <ul class="space-y-0.5 text-sm text-subtext">
        {#each tasks as t}
          <li class="truncate">· {@html inlineMd(t)}</li>
        {/each}
      </ul>
    {:else if !goal}
      <p class="text-sm text-dim italic">Plan section is empty. <a href="/morning" class="text-secondary hover:underline">Plan today →</a></p>
    {/if}
  {:else}
    <p class="text-sm text-dim mb-3">No focus set for today.</p>
    <a href="/morning" class="self-start px-3 py-1.5 text-xs bg-primary text-on-primary rounded font-medium hover:opacity-90">
      Set a focus →
    </a>
  {/if}
</section>
