<script lang="ts">
  import { api, type Goal, type Milestone } from '$lib/api';
  import { toast } from '$lib/components/toast';
  import Drawer from '$lib/components/Drawer.svelte';
  import { inlineMd } from '$lib/util/inlineMd';

  // Detail-and-edit drawer for a single goal. Mirrors ProjectDetail's
  // approach: every field commits via PATCH on blur / explicit toggle so
  // the user never sees a "save" dance for individual properties.
  // Milestones live inside the same drawer (add/edit/toggle/delete).
  let {
    open = $bindable(false),
    goal,
    onUpdated,
    onDeleted
  }: {
    open?: boolean;
    goal: Goal | null;
    onUpdated: () => void | Promise<void>;
    onDeleted: (id: string) => void | Promise<void>;
  } = $props();

  let saving = $state(false);
  let titleBuf = $state('');
  let editingTitle = $state(false);
  let descBuf = $state('');
  let editingDesc = $state(false);
  let notesBuf = $state('');
  let editingNotes = $state(false);

  // Milestones — local input buffers for the "add" form and per-row edits.
  let newMilestoneText = $state('');
  let newMilestoneDue = $state('');
  let editingMilestoneIdx = $state<number | null>(null);
  let editingMilestoneText = $state('');
  let editingMilestoneDue = $state('');

  // Reviews — buffer for "Log review".
  let reviewBuf = $state('');
  let reviewOpen = $state(false);

  const statusOptions: Goal['status'][] = ['active', 'paused', 'completed', 'archived'];
  const colorOptions = ['blue', 'green', 'mauve', 'peach', 'red', 'yellow', 'pink', 'lavender', 'teal', 'sapphire'];
  const categoryOptions = ['career', 'health', 'learning', 'relationships', 'finance', 'creative', 'spiritual', 'other'];

  async function patch(p: Partial<Goal>): Promise<boolean> {
    if (!goal) return false;
    saving = true;
    try {
      await api.patchGoal(goal.id, p);
      await onUpdated();
      return true;
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
      return false;
    } finally {
      saving = false;
    }
  }

  async function commitTitle() {
    editingTitle = false;
    if (goal && titleBuf.trim() && titleBuf !== goal.title) await patch({ title: titleBuf.trim() });
  }
  async function commitDesc() {
    editingDesc = false;
    if (goal && descBuf !== (goal.description ?? '')) await patch({ description: descBuf });
  }
  async function commitNotes() {
    editingNotes = false;
    if (goal && notesBuf !== (goal.notes ?? '')) await patch({ notes: notesBuf });
  }

  async function setStatus(s: Goal['status']) { await patch({ status: s }); }
  async function setTargetDate(v: string) { await patch({ target_date: v }); }
  async function setCategory(v: string) { await patch({ category: v || undefined }); }
  async function setColor(v: string) { await patch({ color: v }); }
  async function setReviewFrequency(v: string) { await patch({ review_frequency: v || undefined }); }
  async function setProject(v: string) { await patch({ project: v.trim() || undefined }); }
  async function setTags(raw: string) {
    const tags = raw.split(',').map((t) => t.trim()).filter(Boolean);
    await patch({ tags });
  }

  async function addMilestone() {
    if (!goal || !newMilestoneText.trim()) return;
    saving = true;
    try {
      await api.addGoalMilestone(goal.id, {
        text: newMilestoneText.trim(),
        due_date: newMilestoneDue || undefined
      });
      newMilestoneText = '';
      newMilestoneDue = '';
      await onUpdated();
    } catch (e) {
      toast.error('add milestone failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      saving = false;
    }
  }

  async function toggleMilestone(idx: number, m: Milestone) {
    if (!goal) return;
    try {
      await api.patchGoalMilestone(goal.id, idx, { done: !m.done });
      await onUpdated();
    } catch (e) {
      toast.error('toggle failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  function startEditMilestone(idx: number, m: Milestone) {
    editingMilestoneIdx = idx;
    editingMilestoneText = m.text;
    editingMilestoneDue = m.due_date ?? '';
  }

  async function commitEditMilestone() {
    if (!goal || editingMilestoneIdx === null) return;
    const idx = editingMilestoneIdx;
    editingMilestoneIdx = null;
    try {
      await api.patchGoalMilestone(goal.id, idx, {
        text: editingMilestoneText,
        due_date: editingMilestoneDue
      });
      await onUpdated();
    } catch (e) {
      toast.error('edit failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function removeMilestone(idx: number) {
    if (!goal) return;
    if (!confirm('Remove this milestone?')) return;
    try {
      await api.deleteGoalMilestone(goal.id, idx);
      await onUpdated();
    } catch (e) {
      toast.error('delete milestone failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  async function logReview() {
    if (!goal || !reviewBuf.trim()) return;
    saving = true;
    try {
      await api.logGoalReview(goal.id, reviewBuf.trim());
      reviewBuf = '';
      reviewOpen = false;
      await onUpdated();
      toast.success('review logged');
    } catch (e) {
      toast.error('log review failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      saving = false;
    }
  }

  async function deleteGoal() {
    if (!goal) return;
    if (!confirm(`Delete goal "${goal.title}"? This is irreversible.`)) return;
    try {
      await api.deleteGoal(goal.id);
      open = false;
      await onDeleted(goal.id);
    } catch (e) {
      toast.error('delete failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  function colorVar(c?: string): string {
    const map: Record<string, string> = {
      red: 'error', yellow: 'warning', orange: 'accent', green: 'success',
      blue: 'secondary', purple: 'primary', cyan: 'info', mauve: 'primary',
      peach: 'accent', teal: 'info', sapphire: 'secondary', pink: 'accent',
      lavender: 'primary', flamingo: 'error'
    };
    return `var(--color-${map[c ?? ''] ?? 'secondary'})`;
  }

  function statusTone(s?: string): string {
    if (s === 'active') return 'primary';
    if (s === 'paused') return 'subtext';
    if (s === 'completed') return 'success';
    if (s === 'archived') return 'dim';
    return 'subtext';
  }

  let progressPct = $derived.by(() => {
    if (!goal) return 0;
    const ms = goal.milestones ?? [];
    if (ms.length === 0) return goal.status === 'completed' ? 100 : 0;
    return Math.round((ms.filter((m) => m.done).length / ms.length) * 100);
  });
</script>

<Drawer bind:open side="right" responsive width="w-full sm:w-[32rem] md:w-[40rem]">
  {#if goal}
    <div class="flex flex-col h-full">
      <header class="px-4 py-3 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
        <span class="w-3 h-3 rounded-full flex-shrink-0" style="background: {colorVar(goal.color)}"></span>
        {#if editingTitle}
          <input
            bind:value={titleBuf}
            onblur={commitTitle}
            onkeydown={(e) => { if (e.key === 'Enter') commitTitle(); else if (e.key === 'Escape') editingTitle = false; }}
            autofocus
            class="text-base font-semibold flex-1 px-1 -mx-1 bg-surface0 border border-primary rounded text-text outline-none"
          />
        {:else}
          <button
            onclick={() => { titleBuf = goal.title; editingTitle = true; }}
            class="text-base font-semibold text-text flex-1 text-left truncate hover:text-primary"
            title="click to rename"
          >{goal.title}</button>
        {/if}
        <select
          value={goal.status ?? 'active'}
          onchange={(e) => setStatus((e.target as HTMLSelectElement).value as Goal['status'])}
          class="text-xs px-2 py-1 bg-surface0 border border-surface1 rounded hover:border-primary"
          style="color: var(--color-{statusTone(goal.status)})"
        >
          {#each statusOptions as s}<option value={s}>{s}</option>{/each}
        </select>
        <button
          onclick={deleteGoal}
          aria-label="delete"
          title="delete goal"
          class="w-9 h-9 flex items-center justify-center text-dim hover:text-error rounded"
        >
          <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
            <path d="M3 6h18M8 6V4a2 2 0 0 1 2-2h4a2 2 0 0 1 2 2v2M19 6l-1 14a2 2 0 0 1-2 2H8a2 2 0 0 1-2-2L5 6"/>
          </svg>
        </button>
        <button onclick={() => (open = false)} aria-label="close" class="text-dim hover:text-text px-2">×</button>
      </header>

      <div class="flex-1 overflow-y-auto p-4 sm:p-6 space-y-6">
        <!-- Progress -->
        <section>
          <div class="flex items-baseline justify-between mb-1.5">
            <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Progress</h3>
            <span class="text-xs text-subtext font-mono">{progressPct}%</span>
          </div>
          <div class="h-2 rounded-full bg-surface0 overflow-hidden">
            <div class="h-full transition-all" style="width: {progressPct}%; background: {colorVar(goal.color)}"></div>
          </div>
        </section>

        <!-- Description -->
        <section>
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-1.5">Description</h3>
          {#if editingDesc}
            <textarea
              bind:value={descBuf}
              onblur={commitDesc}
              onkeydown={(e) => { if (e.key === 'Escape') editingDesc = false; }}
              autofocus
              rows="3"
              class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
            ></textarea>
          {:else}
            <button
              onclick={() => { descBuf = goal.description ?? ''; editingDesc = true; }}
              class="w-full text-left px-3 py-2 text-sm rounded hover:bg-surface0 {goal.description ? 'text-text' : 'text-dim italic'}"
            >{#if goal.description}{@html inlineMd(goal.description)}{:else}click to add a description…{/if}</button>
          {/if}
        </section>

        <!-- Milestones -->
        <section>
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-2">Milestones</h3>
          <ul class="space-y-1.5 mb-3">
            {#each goal.milestones ?? [] as m, i (i)}
              <li class="flex items-start gap-2 text-sm group">
                <button
                  onclick={() => toggleMilestone(i, m)}
                  aria-label={m.done ? 'mark incomplete' : 'mark complete'}
                  class="w-4 h-4 rounded border flex-shrink-0 flex items-center justify-center mt-1 {m.done ? 'bg-success border-success' : 'border-surface2 hover:border-primary'}"
                >
                  {#if m.done}
                    <svg viewBox="0 0 12 12" class="w-3 h-3 text-mantle"><path fill="currentColor" d="M4.5 8.5L2 6l-1 1 3.5 3.5L11 4l-1-1z"/></svg>
                  {/if}
                </button>
                <div class="flex-1 min-w-0">
                  {#if editingMilestoneIdx === i}
                    <div class="space-y-1">
                      <input
                        bind:value={editingMilestoneText}
                        class="w-full px-2 py-1 bg-surface0 border border-primary rounded text-sm text-text outline-none"
                      />
                      <input
                        bind:value={editingMilestoneDue}
                        type="date"
                        class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-xs text-text"
                      />
                      <div class="flex gap-2">
                        <button
                          onclick={commitEditMilestone}
                          class="px-2 py-0.5 text-xs bg-primary text-mantle rounded"
                        >save</button>
                        <button
                          onclick={() => (editingMilestoneIdx = null)}
                          class="px-2 py-0.5 text-xs bg-surface1 text-subtext rounded"
                        >cancel</button>
                      </div>
                    </div>
                  {:else}
                    <button
                      onclick={() => startEditMilestone(i, m)}
                      class="block w-full text-left {m.done ? 'line-through text-dim' : 'text-text'} hover:text-primary"
                    >{m.text}</button>
                    <div class="flex flex-wrap items-center gap-x-3 text-[11px] text-dim">
                      {#if m.due_date}<span>due {m.due_date}</span>{/if}
                      {#if m.completed_at}<span>done {m.completed_at.slice(0, 10)}</span>{/if}
                    </div>
                  {/if}
                </div>
                {#if editingMilestoneIdx !== i}
                  <button
                    onclick={() => removeMilestone(i)}
                    aria-label="remove milestone"
                    class="opacity-0 group-hover:opacity-100 text-dim hover:text-error text-xs transition-opacity"
                  >×</button>
                {/if}
              </li>
            {/each}
          </ul>
          <div class="flex gap-2">
            <input
              bind:value={newMilestoneText}
              onkeydown={(e) => { if (e.key === 'Enter') addMilestone(); }}
              placeholder="new milestone…"
              class="flex-1 px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
            />
            <input
              bind:value={newMilestoneDue}
              type="date"
              class="px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text"
            />
            <button
              onclick={addMilestone}
              disabled={!newMilestoneText.trim() || saving}
              class="px-3 py-1.5 bg-primary text-mantle rounded text-sm disabled:opacity-50"
            >+ add</button>
          </div>
        </section>

        <!-- Reviews -->
        <section>
          <div class="flex items-baseline justify-between mb-2">
            <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Review log</h3>
            <button
              onclick={() => (reviewOpen = !reviewOpen)}
              class="text-xs text-secondary hover:underline"
            >{reviewOpen ? 'cancel' : '+ log review'}</button>
          </div>
          {#if reviewOpen}
            <div class="mb-3 space-y-2">
              <textarea
                bind:value={reviewBuf}
                rows="3"
                placeholder="how is this goal going? what's blocked? what's next?"
                class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
              ></textarea>
              <button
                onclick={logReview}
                disabled={!reviewBuf.trim() || saving}
                class="px-3 py-1.5 bg-primary text-mantle rounded text-sm disabled:opacity-50"
              >{saving ? 'saving…' : 'log review'}</button>
            </div>
          {/if}
          {#if (goal.review_log ?? []).length === 0}
            <p class="text-xs text-dim italic">no reviews logged yet.</p>
          {:else}
            <ul class="space-y-2">
              {#each [...(goal.review_log ?? [])].reverse() as r}
                <li class="px-3 py-2 bg-surface0 rounded text-sm">
                  <div class="flex items-baseline justify-between mb-1">
                    <span class="text-xs text-subtext font-mono">{r.date}</span>
                    <span class="text-[11px] text-dim">{r.progress}%</span>
                  </div>
                  <p class="text-text whitespace-pre-wrap">{r.note}</p>
                </li>
              {/each}
            </ul>
          {/if}
        </section>

        <!-- Notes -->
        <section>
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-1.5">Notes</h3>
          {#if editingNotes}
            <textarea
              bind:value={notesBuf}
              onblur={commitNotes}
              onkeydown={(e) => { if (e.key === 'Escape') editingNotes = false; }}
              autofocus
              rows="4"
              class="w-full px-3 py-2 bg-surface0 border border-primary rounded text-sm text-text outline-none"
            ></textarea>
          {:else}
            <button
              onclick={() => { notesBuf = goal.notes ?? ''; editingNotes = true; }}
              class="w-full text-left px-3 py-2 text-sm rounded hover:bg-surface0 whitespace-pre-wrap {goal.notes ? 'text-text' : 'text-dim italic'}"
            >{goal.notes || 'click to add notes…'}</button>
          {/if}
        </section>

        <!-- Metadata grid -->
        <section class="grid grid-cols-1 sm:grid-cols-2 gap-x-6 gap-y-3 pt-4 border-t border-surface1">
          <div>
            <label for="g-target" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Target date</label>
            <input
              id="g-target"
              type="date"
              value={goal.target_date ?? ''}
              onchange={(e) => setTargetDate((e.target as HTMLInputElement).value)}
              class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
            />
          </div>
          <div>
            <label for="g-cat" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Category</label>
            <select
              id="g-cat"
              value={goal.category ?? ''}
              onchange={(e) => setCategory((e.target as HTMLSelectElement).value)}
              class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
            >
              <option value="">—</option>
              {#each categoryOptions as c}<option value={c}>{c}</option>{/each}
            </select>
          </div>
          <div>
            <label for="g-rev" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Review frequency</label>
            <select
              id="g-rev"
              value={goal.review_frequency ?? ''}
              onchange={(e) => setReviewFrequency((e.target as HTMLSelectElement).value)}
              class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
            >
              <option value="">— none —</option>
              <option value="weekly">weekly</option>
              <option value="monthly">monthly</option>
              <option value="quarterly">quarterly</option>
            </select>
          </div>
          <div>
            <label for="g-proj" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Project</label>
            <input
              id="g-proj"
              value={goal.project ?? ''}
              onblur={(e) => setProject((e.target as HTMLInputElement).value)}
              placeholder="link to a project name"
              class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
            />
          </div>
          <div class="sm:col-span-2">
            <label for="g-tags" class="text-[11px] uppercase tracking-wider text-dim block mb-1">Tags</label>
            <input
              id="g-tags"
              value={(goal.tags ?? []).join(', ')}
              onblur={(e) => setTags((e.target as HTMLInputElement).value)}
              placeholder="comma, separated"
              class="w-full px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
            />
          </div>
          <div class="sm:col-span-2">
            <span class="text-[11px] uppercase tracking-wider text-dim block mb-1">Color</span>
            <div class="flex gap-1.5 flex-wrap">
              {#each colorOptions as c}
                <button
                  onclick={() => setColor(c)}
                  aria-label="color {c}"
                  class="w-6 h-6 rounded-full border-2 {goal.color === c ? 'border-text' : 'border-surface1'}"
                  style="background: {colorVar(c)}"
                ></button>
              {/each}
            </div>
          </div>
        </section>

        <footer class="text-[11px] text-dim pt-2 border-t border-surface1 flex justify-between">
          <span>created {(goal.created_at ?? '').slice(0, 10) || '—'}</span>
          <span>updated {(goal.updated_at ?? '').slice(0, 10) || '—'}</span>
        </footer>
      </div>
    </div>
  {/if}
</Drawer>
