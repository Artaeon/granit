<!--
  TasksQuickAddBar — single-line task entry row that sits above the
  list / kanban / today views. Three controls share the bar:

    1. Quick-add input — parses granit's task-line syntax
       ("buy milk !2 due:tomorrow #errand") and creates the task in
       today's daily note. Keeps hands on the keyboard.
    2. ✨ Plan day — fires the focus-plan AI with the user's stated
       focus-hours budget.
    3. Ask tasks — opens a chat-style Q&A over the visible task list.

  Owns its own input state + busy flag + parse pipeline. aiFocusHours
  is bindable because TasksPlanMyDay (rendered separately in the
  parent) reads the same value — keeping it parent-side avoids
  ping-pong sync between two components.
-->
<script lang="ts">
  import { api } from '$lib/api';
  import { parseTaskInput, smartDate } from '$lib/util/taskParse';
  import { toast } from '$lib/components/toast';
  import { errorMessage } from '$lib/util/errorMessage';
  import type { FocusPlanStore } from './aiAgentStore';
  import type { TasksDataController } from './tasksData.svelte';

  type Props = {
    focusPlan: FocusPlanStore;
    dataCtl: TasksDataController;
    /** filterCtl.filtered.length — used to disable Ask-tasks when the
     *  current view is empty. */
    filteredCount: number;
    aiFocusHours: number;
    onAdded: () => void | Promise<void>;
    onStartAsk: () => void;
  };

  let {
    focusPlan,
    dataCtl,
    filteredCount,
    aiFocusHours = $bindable(),
    onAdded,
    onStartAsk
  }: Props = $props();

  let quickAdd = $state('');
  let quickAddBusy = $state(false);

  async function submitQuickAdd() {
    const raw = quickAdd.trim();
    if (!raw || quickAddBusy) return;
    quickAddBusy = true;
    try {
      // Pre-parse to extract priority / due / tags. parseTaskInput
      // accepts ISO dates; smartDate translates "tomorrow" / "fri"
      // / "next mon" so the user can type natural language.
      const parsed = parseTaskInput(raw);
      // If the user typed `due:<word>` (non-ISO), retry via smartDate.
      // The pre-parse leaves the original raw visible, so we slice
      // it ourselves by re-matching.
      let dueDate = parsed.dueDate;
      if (!dueDate) {
        const m = raw.match(/(?:^|\s)due:([\w-]+)(?=\s|$)/);
        if (m) {
          const sd = smartDate(m[1]);
          if (sd) {
            dueDate = sd;
            // Strip the original `due:<word>` from the text we
            // pre-parsed (parseTaskInput only stripped ISO matches).
            parsed.text = parsed.text.replace(/(?:^|\s)due:[\w-]+(?=\s|$)/, ' ').trim().replace(/\s+/g, ' ');
          }
        }
      }
      if (!parsed.text) {
        toast.error('Empty task — type a description first.');
        return;
      }
      // Create in today's daily note. If today's daily doesn't exist
      // yet, granit's GET /api/v1/daily/today auto-creates it from
      // the template.
      const daily = await api.daily('today');
      await api.createTask({
        notePath: daily.path,
        text: parsed.text,
        priority: parsed.priority || undefined,
        dueDate: dueDate || undefined,
        tags: parsed.tags.length > 0 ? parsed.tags : undefined
      });
      toast.success(`Added: ${parsed.text}`);
      quickAdd = '';
      await onAdded();
    } catch (e) {
      toast.error('Failed to add task: ' + (errorMessage(e)));
    } finally {
      quickAddBusy = false;
    }
  }
</script>

<div class="px-3 py-2 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
  <span class="text-xl text-primary leading-none flex-shrink-0" aria-hidden="true">＋</span>
  <input
    bind:value={quickAdd}
    onkeydown={(e) => { if (e.key === 'Enter') { e.preventDefault(); void submitQuickAdd(); } }}
    placeholder="Quick-add a task — e.g. fix login bug !1 due:tomorrow #frontend"
    aria-label="Quick-add task"
    disabled={quickAddBusy}
    class="flex-1 min-w-0 px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary disabled:opacity-60"
  />
  <button
    onclick={() => void submitQuickAdd()}
    disabled={!quickAdd.trim() || quickAddBusy}
    class="px-3 py-2 bg-primary text-on-primary rounded text-sm disabled:opacity-50 flex-shrink-0"
  >{quickAddBusy ? '…' : 'Add'}</button>
  <!-- Focus-hours input + Plan-my-day trigger. The hours value
       feeds the AI's budget so it doesn't propose 8h of work
       when the user has 2h available. Persisted in
       localStorage by the parent so the user only sets it once.
       Step is 0.5; clamps 0.5-12h. -->
  <label class="hidden sm:inline-flex items-center gap-1 text-xs text-dim flex-shrink-0" title="Focus hours available today — feeds the Plan-my-day budget">
    <input
      type="number"
      min="0.5"
      max="12"
      step="0.5"
      bind:value={aiFocusHours}
      class="w-12 px-1 py-1 bg-surface0 border border-surface1 rounded text-text text-xs tabular-nums text-center focus:outline-none focus:border-primary"
      aria-label="Focus hours available today"
    />
    <span>h</span>
  </label>
  <button
    onclick={() => void focusPlan.run(dataCtl.tasks, aiFocusHours)}
    disabled={$focusPlan.busy || dataCtl.tasks.filter((t) => !t.done).length === 0}
    title="AI builds a sequenced day-plan budgeted to your focus hours"
    class="hidden sm:inline-flex px-3 py-2 text-sm bg-surface1 border border-surface2 text-primary rounded hover:border-primary disabled:opacity-50 flex-shrink-0 items-center gap-1.5"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <path d="M12 3l1.5 4.5L18 9l-4.5 1.5L12 15l-1.5-4.5L6 9l4.5-1.5L12 3z"/>
    </svg>
    <span>{$focusPlan.busy ? 'planning…' : 'Plan day'}</span>
  </button>
  <!-- Ask Tasks — opens a Q&A panel above the list. The model
       answers from the loaded task set as context. No mutations
       — pure read surface for "which P1 has no due date?" /
       "what's blocked?" / "summarize today's commitments" -->
  <button
    onclick={onStartAsk}
    disabled={filteredCount === 0}
    title="Ask AI a question about your current task view"
    class="inline-flex px-3 py-2 text-sm bg-surface1 border border-surface2 text-primary rounded hover:border-primary disabled:opacity-50 flex-shrink-0 items-center gap-1.5"
  >
    <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
      <circle cx="12" cy="12" r="9"/>
      <path d="M9.5 9a2.5 2.5 0 0 1 5 0c0 1.5-2.5 2-2.5 3.5"/>
      <path d="M12 17h0"/>
    </svg>
    <span>Ask tasks</span>
  </button>
</div>
