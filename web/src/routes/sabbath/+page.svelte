<script lang="ts">
  import { onMount } from 'svelte';
  import { sabbath, sabbathSchedule, sabbathMinutesRemaining, DAY_LABELS } from '$lib/stores/sabbath';
  import { goto } from '$app/navigation';

  // /sabbath — a contemplative landing surface. Two roles:
  //   1. While sabbath is active: show what the user has paused, time
  //      remaining, and a short verse + prayer prompt to anchor the
  //      day. The user lands here from the ribbon's tap-target so the
  //      mode has a *place*, not just a hidden-state.
  //   2. While sabbath is NOT active: explain what the toggle does,
  //      let the user configure the auto-schedule, and offer a "begin
  //      now" button. This is the page the sidebar Sabbath button
  //      could deep-link to in future.

  let now = $state(Date.now());
  // Tick each minute so the time-remaining label refreshes without a
  // full page reload. The interval is cleared in the return cleanup.
  onMount(() => {
    const id = setInterval(() => { now = Date.now(); }, 60_000);
    return () => clearInterval(id);
  });

  // Read inside a $derived so the computation re-runs when `now`
  // ticks — sabbathMinutesRemaining isn't reactive on its own.
  let minutesRemaining = $derived.by(() => {
    void now; // depend on tick
    return sabbathMinutesRemaining();
  });

  function fmtRemaining(mins: number): string {
    if (mins <= 0) return 'less than a minute';
    if (mins < 60) return `${mins} minute${mins === 1 ? '' : 's'}`;
    const h = Math.floor(mins / 60);
    const m = mins % 60;
    if (m === 0) return `${h} hour${h === 1 ? '' : 's'}`;
    return `${h}h ${m}m`;
  }

  // Day-of-week label for the schedule picker. -1 reserves the "off"
  // slot; the form binds to the DOW select via a string.
  let scheduleDay = $derived($sabbathSchedule.dayOfWeek);
  let scheduleEnabled = $derived($sabbathSchedule.enabled);

  function setSchedule(enabled: boolean, dow: number) {
    sabbathSchedule.set({ enabled, dayOfWeek: dow });
  }

  // Picked verses for the welcome card. The selection rotates with
  // day-of-month so the user gets variety without true randomness
  // (same date → same verse). Six is enough that consecutive
  // sabbaths land on different ones.
  const SABBATH_VERSES: { ref: string; text: string }[] = [
    {
      ref: 'Mark 2:27',
      text: 'And he said unto them, The sabbath was made for man, and not man for the sabbath.'
    },
    {
      ref: 'Psalm 46:10',
      text: 'Be still, and know that I am God: I will be exalted among the heathen, I will be exalted in the earth.'
    },
    {
      ref: 'Matthew 11:28',
      text: 'Come unto me, all ye that labour and are heavy laden, and I will give you rest.'
    },
    {
      ref: 'Exodus 20:8-10',
      text: 'Remember the sabbath day, to keep it holy. Six days shalt thou labour, and do all thy work: but the seventh day is the sabbath of the Lord thy God: in it thou shalt not do any work.'
    },
    {
      ref: 'Hebrews 4:9-10',
      text: 'There remaineth therefore a rest to the people of God. For he that is entered into his rest, he also hath ceased from his own works, as God did from his.'
    },
    {
      ref: 'Isaiah 30:15',
      text: 'In returning and rest shall ye be saved; in quietness and in confidence shall be your strength.'
    }
  ];
  let verse = $derived.by(() => {
    void now;
    const d = new Date();
    const idx = (d.getDate() + d.getMonth()) % SABBATH_VERSES.length;
    return SABBATH_VERSES[idx];
  });
</script>

<div class="h-full overflow-y-auto bg-mantle">
  <div class="max-w-2xl mx-auto px-5 py-10 sm:py-16">
    {#if $sabbath}
      <!-- ACTIVE STATE — the user is in sabbath. Lead with the
           verse, then time remaining, then a prayer prompt. No
           dashboard chrome; the page itself is the rest. -->
      <header class="text-center mb-10">
        <div class="inline-flex items-center justify-center w-16 h-16 rounded-full bg-surface0 text-success mb-4">
          <svg viewBox="0 0 24 24" class="w-8 h-8" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round">
            <path d="M12 2l1.5 4.5L18 8l-4.5 1.5L12 14l-1.5-4.5L6 8l4.5-1.5L12 2zM12 14v8M9 22h6"/>
          </svg>
        </div>
        <h1 class="text-3xl font-light text-text">Sabbath</h1>
        <p class="text-sm text-dim mt-2">a day made for you, not the other way around</p>
      </header>

      <!-- Verse — large, centered, generous breathing room. -->
      <blockquote class="text-center my-12 px-4">
        <p class="text-lg sm:text-xl text-text leading-relaxed font-serif italic">
          {verse.text}
        </p>
        <footer class="mt-3 text-sm text-secondary">— {verse.ref}</footer>
      </blockquote>

      <!-- Time-remaining + work-modules-paused summary. -->
      <section class="bg-surface0 border border-surface1 rounded-lg p-5 my-8 text-center">
        <p class="text-sm text-subtext">
          Rest until midnight — <span class="text-text font-medium">{fmtRemaining(minutesRemaining)}</span> remaining.
        </p>
        <p class="text-xs text-dim mt-2">
          Tasks, finance, projects, agents, and other work modules are paused. Scripture, prayer, and notes remain.
        </p>
      </section>

      <!-- Prayer prompt — a single soft cue. The user can journal
           by clicking through to /prayer or /jots; the prompt
           itself is not a form field, just a place to settle. -->
      <section class="my-10 px-2">
        <h2 class="text-xs uppercase tracking-wider text-dim mb-3">A prompt for the day</h2>
        <p class="text-base text-subtext leading-relaxed">
          What is one thing this week that I can release into rest — a worry,
          a striving, an outcome I do not control? Carry it gently into prayer,
          and let it stay there.
        </p>
        <div class="mt-5 flex flex-wrap gap-2 text-sm">
          <a href="/prayer" class="px-3 py-1.5 rounded bg-surface1 text-primary border border-surface2 hover:bg-surface2">→ /prayer</a>
          <a href="/jots" class="px-3 py-1.5 rounded bg-surface0 text-subtext border border-surface1 hover:bg-surface1">→ /jots</a>
          <a href="/scripture" class="px-3 py-1.5 rounded bg-surface0 text-subtext border border-surface1 hover:bg-surface1">→ /scripture</a>
        </div>
      </section>

      <!-- Exit. Tucked at the bottom — leaving sabbath should feel
           deliberate, not a button competing with the verse. -->
      <div class="mt-16 text-center">
        <button
          type="button"
          onclick={() => sabbath.disable()}
          class="text-xs text-dim hover:text-text underline"
        >exit sabbath mode</button>
      </div>
    {:else}
      <!-- IDLE STATE — sabbath is off. Explain + offer to begin or
           configure the auto-schedule. -->
      <header class="mb-8">
        <h1 class="text-2xl font-semibold text-text">Sabbath mode</h1>
        <p class="text-sm text-dim mt-2 max-w-prose">
          A weekly day of rest. While sabbath is on, work modules
          (tasks, finance, deadlines, agents, …) hide from the rail
          and AI calls are gated server-side. Scripture, prayer,
          and notes stay. Mark 2:27.
        </p>
      </header>

      <button
        type="button"
        onclick={() => { sabbath.enable(); }}
        class="w-full sm:w-auto px-5 py-2.5 rounded bg-primary text-on-primary font-medium hover:opacity-90"
      >Begin sabbath now</button>

      <section class="mt-10 bg-surface0 border border-surface1 rounded-lg p-5">
        <header class="flex items-baseline gap-2 mb-4">
          <h2 class="text-base font-medium text-text">Auto-schedule</h2>
          <span class="text-xs text-dim">turn on automatically every week</span>
        </header>

        <div class="space-y-3">
          <label class="flex items-center gap-2 text-sm text-subtext">
            <input
              type="checkbox"
              checked={scheduleEnabled}
              onchange={(e) => setSchedule((e.target as HTMLInputElement).checked, scheduleDay < 0 ? 0 : scheduleDay)}
              class="rounded"
            />
            <span>enable weekly schedule</span>
          </label>

          <label class="flex items-center gap-2 text-sm text-subtext {scheduleEnabled ? '' : 'opacity-50'}">
            <span class="w-12">day:</span>
            <select
              disabled={!scheduleEnabled}
              value={scheduleDay}
              onchange={(e) => setSchedule(scheduleEnabled, Number((e.target as HTMLSelectElement).value))}
              class="px-2 py-1 bg-mantle border border-surface1 rounded text-text"
            >
              {#each DAY_LABELS as label, i}
                <option value={i}>{label}</option>
              {/each}
            </select>
          </label>
          <p class="text-xs text-dim">
            On the chosen day, the app launches into sabbath mode automatically. Auto-clears at midnight, same as the manual toggle.
          </p>
        </div>
      </section>
    {/if}
  </div>
</div>
