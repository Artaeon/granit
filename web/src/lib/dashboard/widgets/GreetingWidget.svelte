<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { sabbath } from '$lib/stores/sabbath';

  // Greeting — first thing the user reads on the dashboard. We aim
  // for "warm, grounded, calm". A live clock that updates per minute
  // (no second-hand twitch). A time-of-day greeting that picks a few
  // distinct phrasings rather than the original four blocks. A
  // sabbath-aware variant that names the day's purpose so the
  // greeting fits the mode the rest of the rail is in.

  let { vaultPath = '' }: { vaultPath?: string } = $props();

  let now = $state(new Date());
  let tick: ReturnType<typeof setInterval> | null = null;
  onMount(() => {
    tick = setInterval(() => { now = new Date(); }, 60_000);
  });
  onDestroy(() => { if (tick) clearInterval(tick); });

  let dateLong = $derived(now.toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' }));
  let timeStr = $derived(now.toLocaleTimeString(undefined, { hour: '2-digit', minute: '2-digit' }));

  // Greet by hour bucket. Slightly varied within each bucket so the
  // dashboard doesn't read the same sentence every day. Picks a
  // stable variant per calendar date so reloading doesn't reshuffle.
  function greetFor(d: Date, sab: boolean): string {
    if (sab) {
      const restGreets = [
        'Sabbath rest',
        'A day of rest',
        'Be still',
        'Sabbath peace'
      ];
      return restGreets[(d.getDate() + d.getMonth()) % restGreets.length];
    }
    const h = d.getHours();
    const date = d.getDate();
    if (h < 5) return 'Working late';
    if (h < 12) {
      const morning = ['Good morning', 'Morning', 'A new day'];
      return morning[date % morning.length];
    }
    if (h < 14) {
      const noon = ['Good afternoon', 'Midday', 'Halfway through'];
      return noon[date % noon.length];
    }
    if (h < 18) {
      const aft = ['Good afternoon', 'Afternoon focus', 'The day rolls on'];
      return aft[date % aft.length];
    }
    if (h < 22) {
      const eve = ['Good evening', 'Evening', 'Wind it down'];
      return eve[date % eve.length];
    }
    return 'Late evening';
  }

  let greet = $derived(greetFor(now, $sabbath));

  // Vault path display — strip leading ~/ and trailing trailing slash
  // for a tighter visual.
  let vaultLabel = $derived(vaultPath.replace(/^~\//, '').replace(/\/$/, ''));
</script>

<div class="flex items-baseline gap-3 flex-wrap">
  <h1 class="text-2xl sm:text-3xl font-semibold text-text leading-none">{greet}.</h1>
  <span class="flex-1"></span>
  <!-- Live clock + date in a calm right-aligned cluster. The clock
       updates per minute so the dashboard reads "current" without a
       per-second twitch that would compete with the title for
       attention. -->
  <div class="text-right flex-shrink-0">
    <div class="text-base sm:text-lg font-mono tabular-nums text-subtext leading-tight">{timeStr}</div>
    <div class="text-xs text-dim">{dateLong}</div>
  </div>
</div>
{#if $sabbath}
  <p class="mt-2 text-xs text-success/90">
    🕊️ work modules paused for the day · <a href="/sabbath" class="underline hover:text-success">/sabbath</a>
  </p>
{:else if vaultLabel}
  <p class="mt-2 text-xs text-dim truncate">vault: {vaultLabel}</p>
{/if}
