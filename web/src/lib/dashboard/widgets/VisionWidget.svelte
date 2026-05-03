<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Vision } from '$lib/api';
  import { onWsEvent } from '$lib/ws';

  // VisionWidget is the persistent dashboard strip that re-anchors
  // focus every morning. Renders compact (mission as serif italic +
  // values as chips + season focus + day counter) and links to
  // /vision for editing. Empty state surfaces a single CTA — no
  // form inline since vision is a write-rarely concept that wants
  // a dedicated page.

  let v = $state<Vision | null>(null);

  async function load() {
    try {
      v = await api.getVision();
    } catch {
      v = null;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/vision.json') load();
    });
  });

  let isEmpty = $derived(
    !v || ((!v.mission || v.mission === '') && !v.season_focus && (!v.values || v.values.length === 0))
  );
</script>

<section class="bg-surface0 border border-surface1 rounded-lg p-4 hover:border-primary/40 transition-colors">
  {#if isEmpty}
    <!-- Empty state: single sentence + single CTA. The dashboard
         widget shouldn't be a teaching surface; the /vision page
         carries that copy. -->
    <div class="flex items-center gap-3">
      <span class="text-2xl">🧭</span>
      <div class="flex-1 min-w-0">
        <p class="text-sm text-text font-medium">No vision set yet</p>
        <p class="text-xs text-dim">The layer above goals — mission, values, season focus.</p>
      </div>
      <a href="/vision" class="text-xs px-3 py-1.5 bg-primary text-on-primary rounded font-medium hover:opacity-90 flex-shrink-0">
        Set vision →
      </a>
    </div>
  {:else if v}
    <a href="/vision" class="block group">
      <div class="flex items-baseline gap-2 mb-2">
        <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Vision</h2>
        <span class="flex-1"></span>
        {#if v.season_day && v.season_total}
          <span class="text-[11px] text-dim">Day {v.season_day} / {v.season_total}</span>
        {/if}
        <span class="text-xs text-dim group-hover:text-primary transition-colors">edit →</span>
      </div>

      {#if v.mission}
        <p class="text-base text-text leading-snug font-serif italic mb-3">
          "{v.mission}"
        </p>
      {/if}

      {#if v.season_focus}
        <p class="text-sm text-text mb-3">
          <span class="text-dim text-xs uppercase tracking-wider mr-2">This season</span>
          <span class="font-medium">{v.season_focus}</span>
        </p>
        {#if v.season_total}
          {@const pct = Math.min(100, Math.round(((v.season_day ?? 0) / v.season_total) * 100))}
          <div class="h-1 mb-3 bg-mantle rounded-full overflow-hidden">
            <div class="h-full bg-primary transition-all" style="width: {pct}%"></div>
          </div>
        {/if}
      {/if}

      {#if v.values && v.values.length > 0}
        <ul class="flex flex-wrap gap-1.5">
          {#each v.values as val}
            <li class="text-[11px] px-2 py-0.5 bg-surface1 text-subtext rounded-full">{val}</li>
          {/each}
        </ul>
      {/if}
    </a>
  {/if}
</section>
