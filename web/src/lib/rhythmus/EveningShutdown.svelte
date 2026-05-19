<script lang="ts">
  // The evening shutdown card. Replaces the pillar list on the
  // Heute-Karte once local time crosses the configured evening
  // threshold (default 20:30). Other modules in the app stay
  // reachable — this is *soft* gating, not Sabbath-level hiding.
  //
  // The four questions are taken from the user's brainstorm and
  // map to four short textareas. Saving each writes back into the
  // daily note frontmatter (rhythmus_shutdown.*) so a future Review
  // surface or the next morning's check-in can read what the user
  // committed to.
  //
  // Why the four questions:
  //   1. "Was heute geschafft?" — names the win so the brain can
  //      let go of the open loops.
  //   2. "Was ist morgen wichtig?" — pre-commits tomorrow's MIT so
  //      the morning check-in lands on a real anchor instead of a
  //      blank field.
  //   3. "Was lasse ich bewusst liegen?" — permission to drop, not
  //      just defer. Reduces the next-day cognitive load.
  //   4. "Handy weg?" — a single yes/no that closes the loop. No
  //      streak; just the bookmark on today.
  //
  // The `evening` pillar's `done` flag is what marks the day
  // complete; that flips when the user hits "Tag schließen".

  type Shutdown = {
    achieved: string;
    tomorrow: string;
    letGo: string;
    phoneAway: boolean;
  };

  type Props = {
    shutdown: Shutdown;
    eveningDone: boolean;
    onShutdownChange: (next: Shutdown) => void;
    onCloseDay: () => void;
  };

  let { shutdown, eveningDone, onShutdownChange, onCloseDay }: Props = $props();

  let local = $state<Shutdown>({ achieved: '', tomorrow: '', letGo: '', phoneAway: false });
  $effect(() => {
    local = { ...shutdown };
  });

  function patch(field: keyof Shutdown, value: string | boolean) {
    local = { ...local, [field]: value } as Shutdown;
  }
</script>

<section class="bg-mantle border border-mauve/40 rounded-lg p-5 space-y-4">
  <header class="space-y-1">
    <h2 class="text-lg font-semibold text-text">Abend-Shutdown</h2>
    <p class="text-sm text-dim">
      Arbeit ist für heute zu. Vier Zeilen, dann Handy weg. Schlaf ist jetzt Training.
    </p>
  </header>

  <div>
    <label for="ev-achieved" class="block text-xs uppercase tracking-wider text-dim mb-1">
      Was hast du heute geschafft?
    </label>
    <textarea
      id="ev-achieved"
      rows="2"
      bind:value={local.achieved}
      onblur={() => onShutdownChange(local)}
      placeholder="das hier zählt heute."
      class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-y"
    ></textarea>
  </div>

  <div>
    <label for="ev-tomorrow" class="block text-xs uppercase tracking-wider text-dim mb-1">
      Was ist morgen wichtig?
    </label>
    <textarea
      id="ev-tomorrow"
      rows="2"
      bind:value={local.tomorrow}
      onblur={() => onShutdownChange(local)}
      placeholder="eine konkrete Aufgabe, kein Stichwort."
      class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-y"
    ></textarea>
  </div>

  <div>
    <label for="ev-letgo" class="block text-xs uppercase tracking-wider text-dim mb-1">
      Was lässt du bewusst liegen?
    </label>
    <textarea
      id="ev-letgo"
      rows="2"
      bind:value={local.letGo}
      onblur={() => onShutdownChange(local)}
      placeholder={'nicht „auf morgen verschieben". Loslassen.'}
      class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary resize-y"
    ></textarea>
  </div>

  <div class="flex items-center justify-between pt-2 border-t border-surface1">
    <label class="flex items-center gap-2 text-sm text-subtext cursor-pointer">
      <input
        type="checkbox"
        checked={local.phoneAway}
        onchange={(e) => {
          const next = (e.target as HTMLInputElement).checked;
          patch('phoneAway', next);
          onShutdownChange({ ...local, phoneAway: next });
        }}
        class="accent-primary"
      />
      Handy weg?
    </label>
    <button
      type="button"
      onclick={onCloseDay}
      disabled={eveningDone}
      class="px-4 py-2 rounded text-sm font-medium transition-colors
        {eveningDone
          ? 'bg-success/20 text-success border border-success cursor-default'
          : 'bg-primary text-on-primary hover:opacity-90'}"
    >
      {eveningDone ? 'Tag geschlossen' : 'Tag schließen'}
    </button>
  </div>
</section>
