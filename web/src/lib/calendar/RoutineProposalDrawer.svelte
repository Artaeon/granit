<!--
  RoutineProposalDrawer — preview surface for the Daily Routine AI.

  Mirrors CalendarAgent.svelte's right-side drawer layout but for a
  single streaming proposal rather than a conversation. Sections:

    - Header: title + close + (when a proposal is ready) Apply button
    - Inline status: thinking / streaming / error / partial-failure
    - Rationale paragraph
    - Daily plan preview (full markdown body in a <pre> — full diff
      vs. the existing block is a future polish)
    - Event ops list — one row per op with a colored badge (CREATE /
      UPDATE / DELETE), a one-line summary, and a checkbox to opt-out
    - Footer: Discard + Apply selected (N/M)

  Pure render — every state mutation goes through the controller's
  methods. Lazy toast import keeps the initial bundle slim.
-->
<script lang="ts">
  import type { RoutineAIController } from './calendarRoutineAI.svelte';
  import type { RoutineEventOp } from '$lib/api';

  interface Props {
    open: boolean;
    ctl: RoutineAIController;
    onClose: () => void;
  }
  let { open, ctl, onClose }: Props = $props();

  // Compact one-line summary for an event op. We don't try to render
  // every detail — the row's badge already signals create/update/delete
  // and the rationale paragraph covers the why.
  function summariseOp(op: RoutineEventOp): string {
    switch (op.op) {
      case 'create': {
        const ev = op.event;
        if (!ev) return 'create — (no payload)';
        const time =
          ev.startTime && ev.endTime
            ? ` · ${ev.startTime}–${ev.endTime}`
            : ev.startTime
            ? ` · ${ev.startTime}`
            : '';
        return `${ev.title}${time}`;
      }
      case 'update': {
        const id = op.eventId ?? '?';
        const fields = Object.keys(op.patch ?? {}).join(', ') || '(no fields)';
        return `${id} → ${fields}`;
      }
      case 'delete':
        return `${op.eventId ?? '?'}`;
      default:
        return JSON.stringify(op).slice(0, 80);
    }
  }

  // Badge colour token per op. Matches the calendar's existing semantic
  // palette — primary for additive, secondary for in-place edits, error
  // for removals so the destructive op stands out without being shouty.
  function badgeClass(opKind: string): string {
    switch (opKind) {
      case 'create':
        return 'bg-primary/15 text-primary border-primary/40';
      case 'update':
        return 'bg-secondary/15 text-secondary border-secondary/40';
      case 'delete':
        return 'bg-error/15 text-error border-error/40';
      default:
        return 'bg-surface1 text-dim border-surface2';
    }
  }

  function onKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      onClose();
    }
  }
</script>

{#if open}
  <div
    class="fixed inset-0 z-50 bg-black/40"
    role="dialog"
    aria-modal="true"
    aria-label="Daily routine proposal"
    onclick={(e) => {
      if (e.target === e.currentTarget) onClose();
    }}
    onkeydown={onKey}
    tabindex="-1"
  >
    <aside
      class="absolute right-0 top-0 bottom-0 w-full sm:w-[28rem] bg-base border-l border-surface1 shadow-2xl flex flex-col"
      style="padding-right: var(--ai-pinned-w, 0px);"
    >
      <header
        class="px-3 py-2 border-b border-surface1 flex items-center gap-2 flex-shrink-0"
      >
        <h2 class="text-base font-medium text-text flex-1 inline-flex items-center gap-2">
          <svg
            viewBox="0 0 24 24"
            class="w-4 h-4"
            fill="none"
            stroke="currentColor"
            stroke-width="2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M12 3v3m0 12v3M3 12h3m12 0h3" />
            <circle cx="12" cy="12" r="4" />
          </svg>
          Daily Routine — proposal
        </h2>
        <span class="text-[11px] text-dim font-mono">{ctl.date}</span>
        <button
          type="button"
          onclick={onClose}
          class="text-sm text-dim hover:text-text px-1"
          aria-label="close"
        >×</button>
      </header>

      <div class="flex-1 min-h-0 overflow-y-auto px-3 py-3 space-y-3">
        {#if ctl.busy && !ctl.proposal}
          <p class="text-xs text-dim italic">Drafting your day… this usually takes 5–15s.</p>
        {/if}

        {#if ctl.error}
          <p
            class="text-[11px] px-2 py-1.5 rounded bg-warning/10 text-warning border border-warning/30"
          >{ctl.error}</p>
        {/if}

        {#if ctl.proposal}
          <section>
            <h3 class="text-[10px] uppercase tracking-wide text-dim mb-1">Rationale</h3>
            <p class="text-xs text-text leading-snug">{ctl.proposal.rationale}</p>
          </section>

          <section>
            <h3 class="text-[10px] uppercase tracking-wide text-dim mb-1">Proposed daily plan</h3>
            <pre
              class="text-[11px] whitespace-pre-wrap bg-surface0 border border-surface1 rounded p-2 max-h-64 overflow-auto text-text font-mono leading-snug"
            >{ctl.proposal.dailyPlan}</pre>
          </section>

          {#if ctl.proposal.eventOps.length > 0}
            <section>
              <h3 class="text-[10px] uppercase tracking-wide text-dim mb-1">
                Event changes ({ctl.selectedCount}/{ctl.totalOps})
              </h3>
              <ul class="space-y-1.5">
                {#each ctl.proposal.eventOps as op, i (i + '::' + op.op + '::' + (op.eventId ?? op.event?.title ?? ''))}
                  {@const off = ctl.rejected.has(i)}
                  <li
                    class="border rounded p-2 text-xs flex items-start gap-2 transition-opacity {off
                      ? 'opacity-50 border-surface1 bg-surface0'
                      : 'border-surface1 bg-surface0 hover:border-primary'}"
                  >
                    <input
                      type="checkbox"
                      checked={!off}
                      onchange={() => ctl.toggleOp(i)}
                      aria-label="include this op"
                      class="mt-0.5 w-3.5 h-3.5 accent-primary flex-shrink-0"
                    />
                    <span
                      class="text-[9px] font-mono px-1 py-0.5 rounded uppercase tracking-wide border flex-shrink-0 {badgeClass(op.op)}"
                    >{op.op}</span>
                    <span class="text-text leading-snug break-words">{summariseOp(op)}</span>
                  </li>
                {/each}
              </ul>
            </section>
          {:else}
            <p class="text-[11px] text-dim italic">No event changes proposed — daily plan only.</p>
          {/if}
        {:else if !ctl.busy && !ctl.error}
          <p class="text-xs text-dim italic">No proposal yet.</p>
        {/if}
      </div>

      <footer class="px-3 py-2 border-t border-surface1 flex items-center gap-2 flex-shrink-0">
        {#if ctl.busy}
          <button
            type="button"
            onclick={() => ctl.cancel()}
            class="text-xs text-warning hover:underline"
          >Cancel</button>
        {/if}
        <span class="flex-1"></span>
        <button
          type="button"
          onclick={() => {
            ctl.discard();
            onClose();
          }}
          disabled={ctl.applying}
          class="text-xs px-2 py-1 rounded text-dim hover:text-text disabled:opacity-40"
        >Discard</button>
        <button
          type="button"
          onclick={() => void ctl.apply()}
          disabled={!ctl.proposal || ctl.applying || ctl.busy}
          class="text-xs px-3 py-1 rounded bg-primary text-on-primary hover:opacity-90 disabled:opacity-40"
        >{ctl.applying ? 'applying…' : `Apply selected (${ctl.selectedCount}/${ctl.totalOps})`}</button>
      </footer>
    </aside>
  </div>
{/if}
