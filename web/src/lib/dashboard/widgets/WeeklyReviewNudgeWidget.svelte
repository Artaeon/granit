<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // WeeklyReviewNudgeWidget — silently watches Reviews/ and nags
  // the user when their last weekly review is older than 7 days
  // (or missing entirely). Closes the companion loop to
  // OneThingWidget: that one surfaces the commitment *from* a
  // review, this one nudges the user to *write* the next one.
  //
  // Reads the same Reviews/ folder as OneThing — single cheap
  // listNotes call, limit 1, sorted newest-first by modTime. No
  // body fetch needed; we only care about the timestamp.
  //
  // Hides itself when a fresh review (≤ 7 days) exists so the
  // dashboard isn't littered with green "all good" tiles. The
  // user only sees this widget when there's something to do —
  // which is exactly when a nudge is useful.

  let daysSince = $state<number | null>(null);
  let hasReview = $state(false);
  let loaded = $state(false);

  async function load() {
    try {
      const list = await api.listNotes({ folder: 'Reviews', limit: 1 });
      const note = list.notes[0];
      if (!note) {
        hasReview = false;
        daysSince = null;
        loaded = true;
        return;
      }
      const modMs = new Date(note.modTime).getTime();
      if (Number.isNaN(modMs)) {
        hasReview = false;
        daysSince = null;
        loaded = true;
        return;
      }
      const today = new Date();
      today.setHours(0, 0, 0, 0);
      const modDay = new Date(modMs);
      modDay.setHours(0, 0, 0, 0);
      daysSince = Math.max(0, Math.round((today.getTime() - modDay.getTime()) / 86_400_000));
      hasReview = true;
      loaded = true;
    } catch {
      // A missing Reviews/ folder is "no reviews yet", not an
      // error. Fall through to the no-review CTA.
      hasReview = false;
      daysSince = null;
      loaded = true;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'note.changed' && ev.path?.startsWith('Reviews/')) load();
      if (ev.type === 'note.removed' && ev.path?.startsWith('Reviews/')) load();
    });
  });

  // Threshold matches the weekly cadence: ≤7 days = fresh, hide.
  // Anything older surfaces the nudge so the user catches drift
  // before a second week slips by silently.
  const isStale = $derived(!hasReview || (daysSince !== null && daysSince > 7));
</script>

{#if loaded && isStale}
  <div class="bg-surface0 border border-surface1 rounded-lg p-4 hover:border-primary/40 transition-colors">
    <header class="flex items-baseline gap-2 mb-3">
      <h3 class="text-sm font-medium text-text">Weekly review</h3>
      <span class="flex-1"></span>
      {#if hasReview && daysSince !== null}
        <span class="text-[11px] text-warning font-mono">{daysSince}d ago</span>
      {:else}
        <span class="text-[11px] text-warning font-mono">none yet</span>
      {/if}
    </header>

    <p class="text-sm text-text leading-snug mb-3">
      {#if !hasReview}
        You haven't run a weekly review yet — start one to set this week's focus.
      {:else if daysSince !== null}
        It's been {daysSince} day{daysSince === 1 ? '' : 's'} since your last review.
      {/if}
    </p>

    <a
      href="/review"
      class="inline-flex items-center gap-1 text-xs px-3 py-1.5 bg-primary text-on-primary rounded font-medium hover:opacity-90"
    >
      Start review →
    </a>
  </div>
{/if}
