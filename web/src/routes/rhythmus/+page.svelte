<script lang="ts">
  // /rhythmus — settings UI for the Heute-Karte's quiet machinery.
  //
  // What lives here:
  //   - per-pillar label override (rename "Körper" to "Sport" etc.)
  //   - per-pillar minimum text per day mode (Normal / Chaotic /
  //     Emergency)
  //   - "hide in emergency" toggle per pillar
  //   - evening shutdown threshold (HH:MM)
  //   - eat-nag start time (HH:MM)
  //   - reset to defaults
  //
  // What doesn't:
  //   - the pillar KEYS themselves are locked. The discipline rests
  //     on "there are five"; renaming the labels personalises without
  //     loosening the rhythm.
  //   - day-state edits (current mode / MIT / etc.) — those belong
  //     on the Heute-Karte, not in settings.
  //
  // Persistence: per-device localStorage via $lib/rhythmus/minima.
  // No server call — what counts as "enough" is intensely personal
  // and there's nothing here a second device needs to learn before
  // its owner can start using the app.

  import PageHeader from '$lib/components/PageHeader.svelte';
  import { toast } from '$lib/components/toast';
  import { DEFAULT_PILLARS, PILLAR_ORDER, type PillarKey } from '$lib/rhythmus/pillars';
  import {
    rhythmusConfig,
    DEFAULT_CONFIG,
    type RhythmusConfig,
    type Reminder
  } from '$lib/rhythmus/minima';
  import type { DayMode } from '$lib/rhythmus/dayState';

  let cfg = $state<RhythmusConfig>(structuredClone($rhythmusConfig));

  // Re-sync the local form copy whenever the store changes from
  // another tab. Doing this in a $effect lets a settings edit on
  // tab A propagate to an open settings tab B without a manual
  // reload.
  let storeSnapshot = $derived($rhythmusConfig);
  $effect(() => {
    void storeSnapshot;
    // Only adopt the snapshot if the user isn't mid-edit on a field.
    // Simple heuristic: if the snapshot differs from our last write,
    // adopt it. We track the last write via a flag below.
    if (!justWroteRef.value) cfg = structuredClone(storeSnapshot);
  });

  // Tiny ref so the effect above can tell "I just wrote this" from
  // "something else changed and I should adopt".
  const justWroteRef = { value: false };

  function persist() {
    justWroteRef.value = true;
    rhythmusConfig.set(cfg);
    queueMicrotask(() => {
      justWroteRef.value = false;
    });
  }

  function setLabel(key: PillarKey, label: string) {
    const labels = { ...cfg.labels };
    if (label.trim()) labels[key] = label.trim();
    else delete labels[key];
    cfg = { ...cfg, labels };
    persist();
  }

  function setMinimum(key: PillarKey, mode: DayMode, text: string) {
    const minima = { ...cfg.minima };
    minima[key] = { ...minima[key], [mode]: text };
    cfg = { ...cfg, minima };
    persist();
  }

  function setHideInEmergency(key: PillarKey, hide: boolean) {
    const minima = { ...cfg.minima };
    minima[key] = { ...minima[key], hideInEmergency: hide };
    cfg = { ...cfg, minima };
    persist();
  }

  function setTime(field: 'eveningStartsAt' | 'eatNagAfter', value: string) {
    // Naive HH:MM sanity check — anything that doesn't match a
    // plausible time falls back to the existing value, so a
    // half-typed input doesn't blow away the user's setting mid-edit.
    if (!/^\d{1,2}:\d{2}$/.test(value)) return;
    cfg = { ...cfg, [field]: value };
    persist();
  }

  function resetToDefaults() {
    if (!confirm('Alle Rhythmus-Einstellungen auf Default zurücksetzen?')) return;
    cfg = structuredClone(DEFAULT_CONFIG);
    persist();
    toast.success('Rhythmus auf Defaults zurückgesetzt');
  }

  // Reminder helpers. Each edit replaces the whole reminders array
  // (immutable update keeps Svelte's reactivity tracking happy and
  // avoids mid-tick mutation when the ticker reads $rhythmusConfig).
  function updateReminder(id: string, patch: Partial<Reminder>): void {
    cfg = {
      ...cfg,
      reminders: cfg.reminders.map((r) => (r.id === id ? { ...r, ...patch } : r))
    };
    persist();
  }

  function setReminderTime(id: string, value: string): void {
    if (!/^\d{1,2}:\d{2}$/.test(value)) return;
    updateReminder(id, { time: value });
  }

  // Notification-permission UI. Browsers expose a tri-state
  // ('granted' | 'denied' | 'default'); we only ever ASK from
  // 'default' — a denied permission means the user has already
  // declined and asking again would just be ignored.
  let notifPermission = $state<NotificationPermission>('default');
  $effect(() => {
    if (typeof Notification === 'undefined') return;
    notifPermission = Notification.permission;
  });
  async function requestNotificationPermission() {
    if (typeof Notification === 'undefined') {
      toast.info('Dein Browser unterstützt keine Notifications.');
      return;
    }
    try {
      const next = await Notification.requestPermission();
      notifPermission = next;
      if (next === 'granted') toast.success('Notifications aktiviert');
      else if (next === 'denied') toast.info('Notifications abgelehnt — bleibt bei In-App-Toasts.');
    } catch {
      toast.error('Permission-Request fehlgeschlagen');
    }
  }

  const MODE_LABELS: Record<DayMode, string> = {
    normal:    'Normal',
    chaotic:   'Chaotisch',
    emergency: 'Notfall'
  };
  const MODE_HINTS: Record<DayMode, string> = {
    normal:    'guter Tag — alle 5 Säulen voll',
    chaotic:   'es zerfällt — reduzierte Erwartungen',
    emergency: 'gar nichts geht — Survival-Liste'
  };
</script>

<svelte:head>
  <title>Rhythmus · granit</title>
</svelte:head>

<div class="h-full overflow-y-auto">
  <div class="max-w-3xl mx-auto p-6 sm:p-10 space-y-8">
    <PageHeader
      title="Rhythmus"
      subtitle="Was zählt heute mindestens — pro Säule, pro Modus"
    />

    <!-- Times row. Two small inputs side-by-side; persisted on
         blur via the on:input handler that validates the HH:MM
         shape. -->
    <section class="bg-mantle border border-surface1 rounded-lg p-5 space-y-4">
      <h2 class="text-sm font-medium text-text">Zeiten</h2>
      <div class="grid grid-cols-1 sm:grid-cols-2 gap-4">
        <label class="block">
          <span class="text-xs uppercase tracking-wider text-dim">Abendmodus ab</span>
          <input
            type="time"
            value={cfg.eveningStartsAt}
            oninput={(e) => setTime('eveningStartsAt', (e.target as HTMLInputElement).value)}
            class="mt-1 px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text font-mono w-32 focus:outline-none focus:border-primary"
          />
          <p class="text-[11px] text-dim mt-1">
            Heute-Karte wechselt ab dieser Zeit zum Shutdown-Flow.
          </p>
        </label>
        <label class="block">
          <span class="text-xs uppercase tracking-wider text-dim">„Hast du gegessen?" ab</span>
          <input
            type="time"
            value={cfg.eatNagAfter}
            oninput={(e) => setTime('eatNagAfter', (e.target as HTMLInputElement).value)}
            class="mt-1 px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text font-mono w-32 focus:outline-none focus:border-primary"
          />
          <p class="text-[11px] text-dim mt-1">
            Vor dieser Zeit fragt die App nicht — frühe Aufsteher sehen die Brot-Frage erst, wenn der Morgen meaningfully losgegangen ist.
          </p>
        </label>
      </div>
    </section>

    <!-- Reminders. Five (default) time-of-day pings; each fires a
         toast plus a browser notification when permission is granted.
         Dedup is per-day per-id so a reload doesn't re-fire. -->
    <section class="bg-mantle border border-surface1 rounded-lg p-5 space-y-4">
      <header class="flex items-baseline gap-3 flex-wrap">
        <h2 class="text-sm font-medium text-text flex-1">Reminder</h2>
        {#if notifPermission === 'granted'}
          <span class="text-[11px] text-success">Browser-Notifications aktiv</span>
        {:else if notifPermission === 'denied'}
          <span class="text-[11px] text-dim">Browser-Notifications abgelehnt — nur In-App-Toasts</span>
        {:else}
          <button
            type="button"
            onclick={requestNotificationPermission}
            class="text-[11px] px-2 py-1 rounded bg-surface1 border border-surface2 text-primary hover:border-primary"
          >Browser-Notifications erlauben</button>
        {/if}
      </header>
      <p class="text-[11px] text-dim">
        Fenster: ab der Zeit + 30 Min Gnadenfrist. Wer das Fenster verpasst, sieht den Reminder erst wieder am nächsten Tag.
      </p>
      <ul class="space-y-2">
        {#each cfg.reminders as r (r.id)}
          <li class="grid grid-cols-[auto_5rem_1fr] items-center gap-3">
            <label class="flex items-center gap-2 cursor-pointer">
              <input
                type="checkbox"
                checked={r.enabled}
                onchange={(e) => updateReminder(r.id, { enabled: (e.target as HTMLInputElement).checked })}
                class="accent-primary"
                aria-label="Reminder aktiv"
              />
            </label>
            <input
              type="time"
              value={r.time}
              oninput={(e) => setReminderTime(r.id, (e.target as HTMLInputElement).value)}
              disabled={!r.enabled}
              class="px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text font-mono focus:outline-none focus:border-primary disabled:opacity-50"
            />
            <input
              type="text"
              value={r.label}
              oninput={(e) => updateReminder(r.id, { label: (e.target as HTMLInputElement).value })}
              disabled={!r.enabled}
              class="px-2 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary disabled:opacity-50"
            />
          </li>
        {/each}
      </ul>
    </section>

    <!-- One section per pillar. Inside: label override + 3 minima
         + emergency-hide toggle. The 3 modes are stacked vertically
         (not in a table) because the strings are sentence-length,
         not single words — a table would clip them. -->
    {#each PILLAR_ORDER as key (key)}
      {@const def = DEFAULT_PILLARS[key]}
      {@const labelValue = cfg.labels[key] ?? ''}
      {@const minima = cfg.minima[key]}
      <section class="bg-mantle border border-surface1 rounded-lg p-5 space-y-4">
        <header class="flex items-center gap-3">
          <span class="text-2xl" aria-hidden="true">{def.icon}</span>
          <div class="flex-1 min-w-0">
            <h2 class="text-sm font-medium text-text">{def.label}</h2>
            <p class="text-[11px] text-dim">Säule {key}</p>
          </div>
        </header>

        <label class="block">
          <span class="text-xs uppercase tracking-wider text-dim">Label</span>
          <input
            type="text"
            value={labelValue}
            oninput={(e) => setLabel(key, (e.target as HTMLInputElement).value)}
            placeholder={def.label}
            class="mt-1 w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
          />
          <p class="text-[11px] text-dim mt-1">
            Leer lassen für Default („{def.label}").
          </p>
        </label>

        <div class="space-y-3">
          <p class="text-xs uppercase tracking-wider text-dim">Minimum pro Modus</p>
          {#each ['normal', 'chaotic', 'emergency'] as mode (mode)}
            {@const dm = mode as DayMode}
            <label class="block">
              <span class="text-[11px] text-subtext">
                <span class="font-medium">{MODE_LABELS[dm]}</span>
                <span class="text-dim ml-1.5">— {MODE_HINTS[dm]}</span>
              </span>
              <input
                type="text"
                value={minima[dm]}
                oninput={(e) => setMinimum(key, dm, (e.target as HTMLInputElement).value)}
                class="mt-1 w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary"
              />
            </label>
          {/each}
        </div>

        <label class="flex items-center gap-2 text-xs text-subtext cursor-pointer">
          <input
            type="checkbox"
            checked={!!minima.hideInEmergency}
            onchange={(e) => setHideInEmergency(key, (e.target as HTMLInputElement).checked)}
            class="accent-primary"
          />
          Im Notfall-Modus ganz ausblenden
        </label>
      </section>
    {/each}

    <div class="pt-2">
      <button
        type="button"
        onclick={resetToDefaults}
        class="text-xs px-3 py-1.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-error hover:text-error transition-colors"
      >
        ↺ Alle Rhythmus-Einstellungen auf Default
      </button>
    </div>

    <p class="text-[11px] text-dim italic pt-4 border-t border-surface1">
      Per-Device — Einstellungen liegen in <code>localStorage</code>, nicht in deinem Vault. Die fünf Säulen-Schlüssel sind fest; alles andere ist deins.
    </p>
  </div>
</div>
