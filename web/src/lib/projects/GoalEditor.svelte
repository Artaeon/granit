<script lang="ts">
  import type { ProjectGoal, ProjectMilestone } from '$lib/api';

  let {
    goals,
    onChange
  }: {
    goals: ProjectGoal[];
    onChange: (next: ProjectGoal[]) => void | Promise<void>;
  } = $props();

  let newGoalText = $state('');
  let newMilestoneFor = $state<number | null>(null);
  let newMilestoneText = $state('');

  function clone(gs: ProjectGoal[]): ProjectGoal[] {
    return gs.map((g) => ({ ...g, milestones: g.milestones ? g.milestones.map((m) => ({ ...m })) : [] }));
  }

  async function addGoal() {
    const t = newGoalText.trim();
    if (!t) return;
    newGoalText = '';
    const next = clone(goals);
    next.push({ title: t, done: false, milestones: [] });
    await onChange(next);
  }

  async function toggleGoal(idx: number) {
    const next = clone(goals);
    next[idx].done = !next[idx].done;
    // If a goal has milestones, completing the goal completes all of them too;
    // un-completing the goal leaves milestones as-is.
    if (next[idx].done && next[idx].milestones) {
      next[idx].milestones = next[idx].milestones!.map((m) => ({ ...m, done: true }));
    }
    await onChange(next);
  }

  async function renameGoal(idx: number, title: string) {
    if (title.trim() === goals[idx].title) return;
    const next = clone(goals);
    next[idx].title = title.trim();
    await onChange(next);
  }

  async function removeGoal(idx: number) {
    if (!confirm(`Remove goal "${goals[idx].title}"?`)) return;
    const next = clone(goals);
    next.splice(idx, 1);
    await onChange(next);
  }

  async function moveGoal(idx: number, dir: -1 | 1) {
    const target = idx + dir;
    if (target < 0 || target >= goals.length) return;
    const next = clone(goals);
    [next[idx], next[target]] = [next[target], next[idx]];
    await onChange(next);
  }

  async function addMilestone(goalIdx: number) {
    const t = newMilestoneText.trim();
    if (!t) return;
    newMilestoneText = '';
    newMilestoneFor = null;
    const next = clone(goals);
    next[goalIdx].milestones = next[goalIdx].milestones ?? [];
    next[goalIdx].milestones!.push({ text: t, done: false });
    await onChange(next);
  }

  async function toggleMilestone(goalIdx: number, mIdx: number) {
    const next = clone(goals);
    const ms = next[goalIdx].milestones!;
    ms[mIdx].done = !ms[mIdx].done;
    // Auto-complete the goal when all its milestones are done.
    if (ms.length > 0 && ms.every((m) => m.done)) {
      next[goalIdx].done = true;
    } else {
      next[goalIdx].done = false;
    }
    await onChange(next);
  }

  async function renameMilestone(goalIdx: number, mIdx: number, text: string) {
    if (text.trim() === goals[goalIdx].milestones![mIdx].text) return;
    const next = clone(goals);
    next[goalIdx].milestones![mIdx].text = text.trim();
    await onChange(next);
  }

  async function removeMilestone(goalIdx: number, mIdx: number) {
    const next = clone(goals);
    next[goalIdx].milestones!.splice(mIdx, 1);
    await onChange(next);
  }

  function goalProgress(g: ProjectGoal): { done: number; total: number; pct: number } {
    const ms = g.milestones ?? [];
    if (ms.length === 0) return { done: g.done ? 1 : 0, total: 1, pct: g.done ? 100 : 0 };
    const done = ms.filter((m: ProjectMilestone) => m.done).length;
    return { done, total: ms.length, pct: Math.round((done / ms.length) * 100) };
  }
</script>

<div class="space-y-2">
  {#each goals as g, gi (gi)}
    {@const prog = goalProgress(g)}
    <div class="border border-surface1 rounded bg-surface0/50">
      <div class="flex items-center gap-2 px-3 py-2">
        <button
          onclick={() => toggleGoal(gi)}
          aria-label={g.done ? 'mark not done' : 'mark done'}
          class="w-4 h-4 rounded border flex items-center justify-center flex-shrink-0
            {g.done ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
        >
          {#if g.done}
            <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
          {/if}
        </button>
        <input
          value={g.title}
          onblur={(e) => renameGoal(gi, (e.target as HTMLInputElement).value)}
          onkeydown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
          class="flex-1 bg-transparent text-sm text-text outline-none px-1 -mx-1 hover:bg-surface0 focus:bg-surface0 rounded {g.done ? 'line-through opacity-60' : ''}"
        />
        <span class="text-[10px] text-dim font-mono flex-shrink-0">{prog.done}/{prog.total}</span>
        <div class="hidden sm:flex gap-0.5 opacity-0 group-hover:opacity-100">
          <button onclick={() => moveGoal(gi, -1)} aria-label="up" class="text-dim hover:text-text px-1 text-xs">↑</button>
          <button onclick={() => moveGoal(gi, 1)} aria-label="down" class="text-dim hover:text-text px-1 text-xs">↓</button>
        </div>
        <button
          onclick={() => removeGoal(gi)}
          aria-label="remove goal"
          class="text-dim hover:text-error px-1 text-xs"
        >×</button>
      </div>

      {#if (g.milestones ?? []).length > 0 || newMilestoneFor === gi}
        <div class="ml-7 mr-3 mb-2 space-y-px border-l border-surface1 pl-3">
          {#each g.milestones ?? [] as m, mi (mi)}
            <div class="flex items-center gap-2 py-1">
              <button
                onclick={() => toggleMilestone(gi, mi)}
                aria-label={m.done ? 'mark not done' : 'mark done'}
                class="w-3.5 h-3.5 rounded-sm border flex items-center justify-center flex-shrink-0
                  {m.done ? 'bg-secondary border-secondary' : 'border-surface2 hover:border-secondary'}"
              >
                {#if m.done}
                  <svg viewBox="0 0 12 12" class="w-2.5 h-2.5 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                {/if}
              </button>
              <input
                value={m.text}
                onblur={(e) => renameMilestone(gi, mi, (e.target as HTMLInputElement).value)}
                onkeydown={(e) => { if (e.key === 'Enter') (e.target as HTMLInputElement).blur(); }}
                class="flex-1 bg-transparent text-xs text-subtext outline-none px-1 -mx-1 hover:bg-surface0 focus:bg-surface0 rounded {m.done ? 'line-through opacity-60' : ''}"
              />
              <button onclick={() => removeMilestone(gi, mi)} aria-label="remove" class="text-dim hover:text-error px-1 text-xs">×</button>
            </div>
          {/each}
          {#if newMilestoneFor === gi}
            <div class="flex items-center gap-2 py-1">
              <span class="w-3.5 h-3.5 flex-shrink-0"></span>
              <input
                bind:value={newMilestoneText}
                onkeydown={(e) => { if (e.key === 'Enter') addMilestone(gi); else if (e.key === 'Escape') { newMilestoneFor = null; newMilestoneText = ''; } }}
                onblur={() => { if (!newMilestoneText.trim()) { newMilestoneFor = null; } else addMilestone(gi); }}
                autofocus
                placeholder="new milestone…"
                class="flex-1 bg-mantle border border-primary rounded text-xs text-text px-1.5 py-0.5 outline-none"
              />
            </div>
          {/if}
        </div>
      {/if}

      <button
        onclick={() => { newMilestoneFor = gi; newMilestoneText = ''; }}
        class="ml-7 mb-2 text-[11px] text-dim hover:text-text"
      >+ milestone</button>
    </div>
  {/each}

  <form
    onsubmit={(e) => { e.preventDefault(); addGoal(); }}
    class="flex gap-2 pt-1"
  >
    <input
      bind:value={newGoalText}
      placeholder="add a goal…"
      class="flex-1 px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
    />
    <button
      type="submit"
      disabled={!newGoalText.trim()}
      class="px-3 py-2 bg-primary text-mantle rounded text-sm font-medium disabled:opacity-50"
    >+ goal</button>
  </form>
</div>
