<script lang="ts">
  // PlanMyDayDrawer — preview-and-edit AI scheduling.
  //
  // Replaces the previous fire-and-forget "Plan with AI" toast loop.
  // Flow:
  //   1. User opens the drawer.
  //   2. Hits Run → POST /agents/plan-day-schedule with {dry_run:true}.
  //      Endpoint runs the plan-my-day preset, parses the `## Plan`
  //      section in today's daily note, fuzzy-matches to tasks, but
  //      DOES NOT write scheduledStart yet. Response carries proposals.
  //   3. User reviews the proposals: each row has start/duration pickers
  //      and a keep/skip toggle. Unmatched plan lines get a "create
  //      task" hint section so nothing the model proposed gets lost.
  //   4. Apply → POST /agents/plan-day-apply with the kept subset.
  //      Server writes scheduledStart on each task, drawer closes,
  //      calendar refreshes.
  //
  // Design notes:
  //   - We DON'T subscribe to the agent's WS stream during dry-run.
  //     The plan-my-day preset writes to the daily note; we don't need
  //     the per-step transcript here, just the final proposals. Showing
  //     a phase cycler ("Reading tasks → Drafting plan → Matching")
  //     gives the user something to watch without coupling the drawer
  //     to the agentruntime's event shape.
  //   - Edits are local until Apply. Esc / Cancel discards everything;
  //     the dry-run side-effects (the daily note's `## Plan` section
  //     written by the preset) survive — they're harmless markdown.
  //   - The duration picker is a button group (15/30/45/60/90) instead
  //     of a number input: faster to retarget on touch + the AI
  //     virtually never proposes durations off-grid.

  import { onDestroy } from 'svelte';
  import { goto } from '$app/navigation';
  import { api, type PlanProposal, type Task } from '$lib/api';
  import Drawer from '$lib/components/Drawer.svelte';
  import { toast } from '$lib/components/toast';
  import { classifyAiError } from '$lib/util/aiErrors';

  let {
    open = $bindable(false),
    onApplied
  }: {
    open?: boolean;
    /** Called after a successful apply so the parent can refresh
     *  whatever it's showing (calendar grid, daily note, backlog). */
    onApplied?: () => void | Promise<void>;
  } = $props();

  // Editable local copy of the proposals returned by dry-run. We keep
  // the original PlanProposal alongside (immutable) for the "AI
  // suggested this because…" tooltip, and a mutable view that the
  // pickers bind to.
  type EditableProposal = {
    base: PlanProposal;
    keep: boolean;
    // hh:mm string; bound to <input type="time"> for desktop and a
    // native time picker on mobile. We re-derive the RFC3339 start
    // from this + the original date at apply time.
    startHHMM: string;
    durationMinutes: number;
  };

  let phase = $state<'idle' | 'running' | 'review' | 'applying' | 'error'>('idle');
  let runProgressMessage = $state('Reading your tasks…');
  let progressTimer: ReturnType<typeof setInterval> | null = null;

  // Proposals + unmatched lines from the dry-run.
  let editable = $state<EditableProposal[]>([]);
  let unmatched = $state<string[]>([]);
  let runId = $state<string | null>(null);

  // Quick lookup: tasks already on the grid → so unmatched-list entries
  // can offer to create a new task with the AI's suggested time.
  let creatingFromUnmatched = $state<number | null>(null); // index of busy row

  // Phase cycler — purely cosmetic, but a 5–30s wait with no feedback
  // is the worst UX of the old flow. We rotate three sentences that
  // mirror what the preset does internally.
  const PHASE_MESSAGES = [
    'Reading your tasks…',
    'Drafting today\'s plan…',
    'Matching slots to tasks…'
  ];

  function startPhaseCycler() {
    let i = 0;
    runProgressMessage = PHASE_MESSAGES[i];
    if (progressTimer) clearInterval(progressTimer);
    progressTimer = setInterval(() => {
      i = (i + 1) % PHASE_MESSAGES.length;
      runProgressMessage = PHASE_MESSAGES[i];
    }, 4000);
  }
  function stopPhaseCycler() {
    if (progressTimer) {
      clearInterval(progressTimer);
      progressTimer = null;
    }
  }

  onDestroy(() => stopPhaseCycler());

  // Reset when reopened so a previous review doesn't ghost-render. We
  // don't auto-run on open — give the user a chance to read the intro
  // and confirm before spending an LLM call.
  $effect(() => {
    if (!open) return;
    if (phase === 'applying') return;
    if (phase === 'idle' || phase === 'error') {
      // fresh open
      editable = [];
      unmatched = [];
      runId = null;
    }
  });

  // ─── Helpers ───
  function rfc3339ToHHMM(iso: string): string {
    const d = new Date(iso);
    return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`;
  }

  function hhmmToRfc3339(hhmm: string, sampleIso: string): string {
    // Re-anchor the user's chosen HH:MM onto the original date the AI
    // proposed. This avoids cross-midnight surprises if the user is
    // editing past midnight: the date stays stable.
    const [h, m] = hhmm.split(':').map(Number);
    const d = new Date(sampleIso);
    d.setHours(h, m, 0, 0);
    // Local-time RFC3339 with offset. Date#toISOString would force UTC
    // and shift the wall-clock hour the user picked.
    return localRfc3339(d);
  }

  function localRfc3339(d: Date): string {
    // Mirrors what TaskStore.Schedule expects on the wire — the server
    // parses RFC3339 with offset. Build the tz offset by hand to avoid
    // toISOString's UTC-shift gotcha.
    const pad = (n: number) => String(n).padStart(2, '0');
    const tzMin = -d.getTimezoneOffset();
    const sign = tzMin >= 0 ? '+' : '-';
    const tzh = pad(Math.floor(Math.abs(tzMin) / 60));
    const tzm = pad(Math.abs(tzMin) % 60);
    return (
      `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T` +
      `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}` +
      `${sign}${tzh}:${tzm}`
    );
  }

  function snapToQuarter(hhmm: string): string {
    // 15-min snap. Browsers honour <input step=900> on most platforms,
    // but mobile Safari ignores step on time inputs — we re-snap on
    // change to keep the contract honest.
    const [h, m] = hhmm.split(':').map(Number);
    if (Number.isNaN(h) || Number.isNaN(m)) return hhmm;
    const snapped = Math.round(m / 15) * 15;
    if (snapped === 60) {
      return `${String((h + 1) % 24).padStart(2, '0')}:00`;
    }
    return `${String(h).padStart(2, '0')}:${String(snapped).padStart(2, '0')}`;
  }

  // ─── Dry-run ───
  async function runDryRun() {
    phase = 'running';
    startPhaseCycler();
    try {
      const r = await api.runPlanDaySchedule({ dryRun: true });
      runId = r.runId;
      editable = r.proposals.map((p) => ({
        base: p,
        keep: true,
        startHHMM: rfc3339ToHHMM(p.start),
        durationMinutes: p.durationMinutes
      }));
      unmatched = r.unmatched ?? [];
      phase = 'review';
      if (editable.length === 0 && unmatched.length === 0) {
        toast.warning('AI returned no plan blocks — try again or plan manually');
      }
    } catch (e) {
      const raw = e instanceof Error ? e.message : String(e);
      console.error('[PlanMyDayDrawer] dry-run failed:', raw);
      const hint = classifyAiError(raw);
      toast.error(hint.headline, { action: hint.cta, details: hint.raw });
      phase = 'error';
    } finally {
      stopPhaseCycler();
    }
  }

  // ─── Apply ───
  async function applyKept() {
    const kept = editable.filter((p) => p.keep);
    if (kept.length === 0) {
      toast.info('Nothing to apply — every row is set to skip');
      return;
    }
    phase = 'applying';
    try {
      const r = await api.applyPlanDaySchedule(
        kept.map((p) => ({
          taskId: p.base.taskId,
          start: hhmmToRfc3339(p.startHHMM, p.base.start),
          durationMinutes: p.durationMinutes
        }))
      );
      const n = r.scheduled.length;
      if (r.errors.length > 0) {
        toast.warning(`Applied ${n}, ${r.errors.length} failed`);
      } else {
        toast.success(`Scheduled ${n} task${n === 1 ? '' : 's'}`);
      }
      open = false;
      phase = 'idle';
      await onApplied?.();
    } catch (e) {
      const raw = e instanceof Error ? e.message : String(e);
      console.error('[PlanMyDayDrawer] apply failed:', raw);
      toast.error('Couldn\'t apply schedule', { details: raw });
      phase = 'review'; // back to the editable list, errors aside
    }
  }

  // ─── Unmatched: turn an LLM line into a task ───
  // Best-effort: we don't know what notePath the user wants this in.
  // Default to today's daily note (`api.daily('today')` returns it),
  // shove the line into ## Tasks. The user can re-run plan after.
  async function createTaskFromUnmatched(idx: number, line: string) {
    creatingFromUnmatched = idx;
    try {
      const daily = await api.daily('today');
      const text = line.trim();
      if (!text) {
        toast.warning('Empty line — nothing to create');
        return;
      }
      await api.createTask({
        notePath: daily.path,
        text,
        section: '## Tasks'
      });
      // Drop the line locally so the user sees progress.
      unmatched = unmatched.filter((_, i) => i !== idx);
      toast.success('Task created — re-run Plan to schedule it');
    } catch (e) {
      const raw = e instanceof Error ? e.message : String(e);
      toast.error('Couldn\'t create task', { details: raw });
    } finally {
      creatingFromUnmatched = null;
    }
  }

  // ─── Cancel / discard ───
  function close() {
    if (phase === 'applying') return; // don't yank a write in flight
    open = false;
    phase = 'idle';
    editable = [];
    unmatched = [];
    runId = null;
  }

  // Manual scheduling fallback when the LLM is down. Switches the user
  // to the calendar's plan mode; they can still drag from the backlog.
  function fallbackToManual() {
    open = false;
    phase = 'idle';
    goto('/calendar?view=plan');
  }

  // Duration picker options. 15-min granularity matches the AI's typical
  // proposal cadence; 90 caps the long side because anything longer is
  // usually a sign the user wanted multiple smaller tasks.
  const DURATION_CHOICES = [15, 30, 45, 60, 90];

  // Derived: kept count for the apply CTA.
  let keptCount = $derived(editable.filter((p) => p.keep).length);
</script>

<Drawer bind:open side="right" responsive width="w-full sm:w-[28rem] md:w-[34rem] lg:w-[38rem]">
  <div class="h-full flex flex-col overflow-hidden">
    <!-- Header -->
    <header class="px-4 py-3 border-b border-surface1 flex items-center gap-2 flex-shrink-0">
      <div class="flex-1 min-w-0">
        <h2 class="text-base font-semibold text-text">Plan today with AI</h2>
        <p class="text-[11px] text-dim mt-0.5">
          Reads your tasks, calendar &amp; projects → proposes a schedule. Review before committing.
        </p>
      </div>
      <button
        onclick={close}
        aria-label="close"
        disabled={phase === 'applying'}
        class="w-9 h-9 -mr-2 flex items-center justify-center text-dim hover:text-text hover:bg-surface0 rounded text-xl leading-none disabled:opacity-50"
      >×</button>
    </header>

    <!-- Body -->
    <div class="flex-1 overflow-y-auto">
      {#if phase === 'idle' || phase === 'error'}
        <!-- Pre-run intro -->
        <div class="p-4 space-y-3">
          <div class="rounded p-3 bg-surface0/50 border border-surface1">
            <p class="text-sm text-text">
              Click <strong>Run</strong> to draft today's schedule. The AI looks at:
            </p>
            <ul class="text-xs text-subtext mt-2 ml-4 list-disc space-y-1">
              <li>Open tasks (due today, overdue, P1–P3)</li>
              <li>Calendar events already on today's grid</li>
              <li>Active projects &amp; deadlines</li>
            </ul>
            <p class="text-xs text-dim mt-2 italic">5–30 seconds. Nothing gets scheduled until you hit Apply.</p>
          </div>

          {#if phase === 'error'}
            <div class="rounded p-3 bg-error/10 border border-error/30 text-sm text-text">
              The AI couldn't run — see the toast for details.
              <button
                onclick={fallbackToManual}
                class="block mt-2 text-secondary hover:underline text-xs"
              >Switch to manual scheduling →</button>
            </div>
          {/if}

          <button
            onclick={runDryRun}
            class="w-full px-3 py-2.5 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90"
          >Run plan-my-day</button>
        </div>
      {:else if phase === 'running'}
        <!-- Running -->
        <div class="p-6 flex flex-col items-center text-center gap-3">
          <span class="inline-block w-8 h-8 rounded-full border-3 border-primary border-t-transparent animate-spin"></span>
          <p class="text-sm text-text">{runProgressMessage}</p>
          <p class="text-[11px] text-dim italic max-w-[28ch]">
            The model is drafting your day. We'll show every slot before anything gets scheduled.
          </p>
        </div>
      {:else if phase === 'review' || phase === 'applying'}
        <!-- Review / edit -->
        <div class="p-4 space-y-3">
          {#if editable.length === 0 && unmatched.length === 0}
            <p class="text-sm text-dim italic text-center py-8">
              The AI returned no plan blocks. Try again or
              <button class="text-secondary hover:underline" onclick={fallbackToManual}>plan manually</button>.
            </p>
          {/if}

          {#if editable.length > 0}
            <div class="flex items-baseline gap-2">
              <h3 class="text-[11px] uppercase tracking-wider text-dim font-medium">Proposed slots</h3>
              <span class="text-[11px] text-dim">· {keptCount} of {editable.length} kept</span>
            </div>

            <ul class="space-y-2">
              {#each editable as p, i (p.base.taskId + ':' + i)}
                <li
                  class="rounded border border-surface1 bg-surface0/40 transition-opacity {p.keep ? '' : 'opacity-40'}"
                >
                  <div class="flex items-stretch gap-2 p-2">
                    <!-- Keep / skip toggle. Big tap target on mobile. -->
                    <button
                      type="button"
                      onclick={() => (p.keep = !p.keep)}
                      aria-label={p.keep ? 'skip this slot' : 'keep this slot'}
                      title={p.keep ? 'Click to skip' : 'Click to keep'}
                      class="flex-shrink-0 w-6 h-6 rounded border-2 self-start mt-0.5 flex items-center justify-center
                        {p.keep ? 'bg-primary border-primary text-on-primary' : 'border-surface1 hover:border-primary'}"
                    >
                      {#if p.keep}
                        <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="3" stroke-linecap="round" stroke-linejoin="round">
                          <path d="M5 12l5 5L20 7"/>
                        </svg>
                      {/if}
                    </button>

                    <div class="flex-1 min-w-0 space-y-1.5">
                      <!-- Task title -->
                      <p class="text-sm text-text truncate" title={p.base.taskText}>
                        {p.base.taskText}
                      </p>

                      <!-- Time + duration controls -->
                      <div class="flex items-center gap-2 flex-wrap">
                        <input
                          type="time"
                          bind:value={p.startHHMM}
                          onchange={() => (p.startHHMM = snapToQuarter(p.startHHMM))}
                          step="900"
                          disabled={!p.keep || phase === 'applying'}
                          class="px-2 py-1 bg-mantle border border-surface1 rounded text-xs text-text disabled:opacity-50 focus:outline-none focus:border-primary"
                          aria-label="start time"
                        />
                        <div class="flex bg-mantle border border-surface1 rounded overflow-hidden">
                          {#each DURATION_CHOICES as d}
                            <button
                              type="button"
                              onclick={() => (p.durationMinutes = d)}
                              disabled={!p.keep || phase === 'applying'}
                              class="px-2 py-1 text-[11px] {p.durationMinutes === d ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'} disabled:opacity-50"
                            >{d}m</button>
                          {/each}
                        </div>
                      </div>

                      <!-- Why: the LLM's raw plan line. Tooltip on
                           desktop, inline on mobile (hover doesn't
                           work on touch). -->
                      <p
                        class="text-[10px] text-dim italic font-mono truncate"
                        title="AI suggested this because: {p.base.planLine}"
                      >
                        {p.base.planLine}
                      </p>
                    </div>
                  </div>
                </li>
              {/each}
            </ul>
          {/if}

          {#if unmatched.length > 0}
            <div class="space-y-2 pt-2 border-t border-surface1">
              <h3 class="text-[11px] uppercase tracking-wider text-dim font-medium">
                Lines without a matching task
              </h3>
              <p class="text-[11px] text-dim italic">
                The AI proposed these slots but couldn't find a matching task. Click <strong>+ create</strong> to add one to today's note.
              </p>
              <ul class="space-y-1">
                {#each unmatched as line, i (line + ':' + i)}
                  <li class="flex items-center gap-2 text-xs">
                    <span class="flex-1 min-w-0 truncate text-subtext font-mono">{line}</span>
                    <button
                      onclick={() => createTaskFromUnmatched(i, line)}
                      disabled={creatingFromUnmatched === i}
                      class="px-2 py-1 text-[11px] rounded bg-secondary/15 text-secondary border border-secondary/30 hover:bg-secondary/25 disabled:opacity-50 flex-shrink-0"
                    >
                      {creatingFromUnmatched === i ? '…' : '+ create'}
                    </button>
                  </li>
                {/each}
              </ul>
            </div>
          {/if}
        </div>
      {/if}
    </div>

    <!-- Footer / CTA -->
    {#if phase === 'review' || phase === 'applying'}
      <div class="px-4 py-3 border-t border-surface1 bg-mantle flex items-center gap-2 flex-shrink-0">
        <button
          onclick={close}
          disabled={phase === 'applying'}
          class="px-3 py-2 text-xs text-subtext hover:text-text hover:bg-surface0 rounded disabled:opacity-50"
        >Cancel</button>
        <span class="flex-1"></span>
        <button
          onclick={runDryRun}
          disabled={phase === 'applying'}
          class="px-3 py-2 text-xs text-subtext hover:text-text border border-surface1 rounded hover:border-primary disabled:opacity-50"
          title="Run dry-run again to refresh proposals"
        >Re-run</button>
        <button
          onclick={applyKept}
          disabled={phase === 'applying' || keptCount === 0}
          class="px-3 py-2 text-xs bg-primary text-on-primary rounded font-medium disabled:opacity-50"
        >
          {phase === 'applying' ? 'Applying…' : `Apply ${keptCount} ${keptCount === 1 ? 'slot' : 'slots'}`}
        </button>
      </div>
    {/if}
  </div>
</Drawer>
