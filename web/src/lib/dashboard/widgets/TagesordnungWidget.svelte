<script lang="ts">
  // Tagesordnung — daily anchor of 16 Leitbegriffe.
  //
  // Phase 1 of the alignment system. Pure presentation: reads from
  // the central PRINCIPLES constant and renders a quiet grid that
  // sits on the dashboard as a "stand of stones" — always there,
  // doesn't change, doesn't track anything. Lived testing first;
  // later phases (Check-in, Review, task/project tagging) will pull
  // from the same constants without breaking this surface.
  //
  // Visual intent per spec: ruhig, minimal, würdig — no icons, no
  // colour accents, no streaks, no scores. The user reads the list,
  // the list grounds the day, that's the whole job.

  import { PRINCIPLES, PRINCIPLES_KURZ } from '$lib/principles/principles';

  // The widget API matches the rest of the dashboard's contract —
  // every widget receives the vault root path so it can build links
  // back into the file tree if it needs to. This one doesn't, but
  // accepting the prop keeps the registry's typing uniform.
  interface Props {
    vaultPath?: string;
  }
  let { vaultPath: _vaultPath = '' }: Props = $props();
</script>

<section class="bg-mantle border border-surface1 rounded-lg p-4 sm:p-5">
  <header class="flex items-baseline gap-2 mb-3">
    <h2 class="text-sm font-medium text-text">Tagesordnung</h2>
    <span class="text-[11px] text-dim">Innere Ordnung — täglicher Anker</span>
  </header>

  <!-- 16 Leitbegriffe in a compact grid. Column counts chosen so the
       list divides evenly at every breakpoint: 1×16 on phone, 2×8 on
       tablet/laptop, 4×4 on a wide monitor. No orphan rows, no
       half-empty trailing columns. -->
  <ul class="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-x-5 gap-y-2.5">
    {#each PRINCIPLES as p (p.id)}
      <li class="leading-snug">
        <div class="text-sm font-medium text-text">{p.name}</div>
        <div class="text-xs text-dim">{p.short}</div>
      </li>
    {/each}
  </ul>

  <!-- Kurzform of the Leitsatz — the verb refrain. Quiet, italic, no
       border above; a signature line, not a divider. Hidden on the
       narrowest phones where vertical space is the scarce axis. -->
  <p class="hidden sm:block mt-4 text-[11px] text-dim italic">
    {PRINCIPLES_KURZ}
  </p>
</section>
