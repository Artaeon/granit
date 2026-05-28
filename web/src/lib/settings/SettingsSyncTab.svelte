<script lang="ts">
  // Sync tab — push reminders, git autocommit, Stoicera intranet
  // integration, devices, git auto-sync status. Each block is a
  // section. The per-category reminder preferences (calendar / tasks /
  // deadlines / quiet hours) live behind "Show reminder rules" so
  // the default view fits.
  import type { Device } from '$lib/api';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import SettingsSection from './SettingsSection.svelte';
  import SettingsRow from './SettingsRow.svelte';
  import ConfirmButton from './ConfirmButton.svelte';

  type SyncStatus = {
    enabled: boolean;
    interval?: string;
    lastPull?: string;
    lastPush?: string;
    pulls?: number;
    pushes?: number;
    lastErr?: string;
  };
  type PushStatus = { supported: boolean; permission: NotificationPermission; subscribed: boolean; paused?: boolean };
  type Prefs = {
    calendar: { enabled: boolean };
    tasks: { enabled: boolean; due_today_time: string };
    deadlines: { enabled: boolean; days_before: number[]; at_time: string };
    quiet_hours: { enabled: boolean; start: string; end: string };
    default_event_reminder: number;
  };
  type Stoicera = { enabled: boolean; venture_name: string; token_masked: string; has_token: boolean };
  type Autocommit = { enabled: boolean; isGitRepo: boolean };

  type Props = {
    sync: SyncStatus | null;
    syncBusy: boolean;
    pushStatus: PushStatus;
    pushBusy: boolean;
    prefs: Prefs;
    autocommit: Autocommit;
    autocommitSaving: boolean;
    stoiceraSettings: Stoicera;
    stoiceraSaving: boolean;
    stoiceraVentureBuf: string;
    stoiceraTokenRevealed: string | null;
    devices: Device[];
    revokeBusy: string | null;
    syncNow: () => Promise<void>;
    enablePush: () => Promise<void>;
    pausePush: () => Promise<void>;
    resumePush: () => Promise<void>;
    disablePush: () => Promise<void>;
    testPush: () => Promise<void>;
    savePrefs: () => void;
    toggleDeadlineOffset: (off: number) => void;
    toggleAutocommit: (enabled: boolean) => Promise<void>;
    toggleStoicera: (enabled: boolean) => Promise<void>;
    commitStoiceraVenture: () => Promise<void>;
    regenerateStoiceraToken: () => Promise<void>;
    revealStoiceraToken: () => Promise<void>;
    copyStoiceraToken: () => Promise<void>;
    setStoiceraTokenRevealed: (v: string | null) => void;
    revokeDevice: (id: string) => Promise<void>;
    fmtTime: (s?: string) => string;
    fmtRelative: (iso: string) => string;
  };

  let {
    sync,
    syncBusy,
    pushStatus,
    pushBusy,
    prefs = $bindable(),
    autocommit,
    autocommitSaving,
    stoiceraSettings,
    stoiceraSaving,
    stoiceraVentureBuf = $bindable(),
    stoiceraTokenRevealed,
    devices,
    revokeBusy,
    syncNow,
    enablePush,
    pausePush,
    resumePush,
    disablePush,
    testPush,
    savePrefs,
    toggleDeadlineOffset,
    toggleAutocommit,
    toggleStoicera,
    commitStoiceraVenture,
    regenerateStoiceraToken,
    revealStoiceraToken,
    copyStoiceraToken,
    setStoiceraTokenRevealed,
    revokeDevice,
    fmtTime,
    fmtRelative
  }: Props = $props();
</script>

<!-- Reminders (push) -->
<SettingsSection
  title="Reminders"
  status={pushBusy ? 'working…' : undefined}
  advancedLabel="Show reminder rules"
>
  {#snippet children()}
    {#if !pushStatus.supported}
      <p class="text-sm text-dim py-1">
        Push notifications aren't supported in this browser. On iOS this works only in an installed PWA on iOS 16.4+.
      </p>
    {:else if pushStatus.permission === 'denied'}
      <p class="text-sm text-warning py-1">
        Notifications are blocked at the browser level. Enable them in your browser's site settings, then return here.
      </p>
    {:else if !pushStatus.subscribed}
      <p class="text-sm text-dim mb-2 leading-snug">
        Reminds you about upcoming events even when the tab is closed. Set "Remind me N min before" on any event to fire a push.
      </p>
      <button
        type="button"
        onclick={() => void enablePush()}
        disabled={pushBusy}
        class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50"
      >Enable mobile reminders</button>
    {:else}
      {#if pushStatus.paused}
        <p class="text-sm text-warning mb-2 leading-snug">
          Notifications paused on this device. Subscription is still active — resume any time without re-granting permission.
        </p>
      {:else}
        <p class="text-sm text-success mb-2 leading-snug">
          Subscribed on this device. Set a reminder on any event in the calendar to receive a push.
        </p>
      {/if}
      <div class="flex flex-wrap items-center gap-1.5">
        {#if pushStatus.paused}
          <button
            type="button"
            onclick={() => void resumePush()}
            disabled={pushBusy}
            class="px-2.5 py-1 bg-primary text-on-primary rounded text-xs font-medium disabled:opacity-50"
          >Resume</button>
        {:else}
          <button
            type="button"
            onclick={() => void pausePush()}
            disabled={pushBusy}
            class="px-2.5 py-1 bg-surface0 text-warning rounded text-xs border border-surface1 hover:bg-surface1 disabled:opacity-50"
          >Pause</button>
        {/if}
        <button
          type="button"
          onclick={() => void testPush()}
          disabled={pushBusy || pushStatus.paused}
          class="px-2.5 py-1 bg-surface0 text-subtext rounded text-xs border border-surface1 hover:bg-surface1 disabled:opacity-50"
        >Send test</button>
        <span class="flex-1"></span>
        <ConfirmButton
          label="Unsubscribe"
          confirmLabel="Unsubscribe?"
          danger
          disabled={pushBusy}
          title="Permanently remove this device's subscription"
          onconfirm={disablePush}
        />
      </div>
    {/if}
  {/snippet}

  {#snippet advanced()}
    {#if !pushStatus.subscribed}
      <p class="text-[11px] text-dim italic">Subscribe first to configure per-category rules.</p>
    {:else}
      <h3 class="text-[10px] uppercase tracking-wider text-dim font-semibold mb-2">What to remind me about</h3>
      <div class="space-y-3">
        <!-- Calendar events -->
        <div class="flex items-start gap-2.5">
          <input
            type="checkbox"
            bind:checked={prefs.calendar.enabled}
            onchange={() => savePrefs()}
            class="mt-1 w-4 h-4 accent-primary"
          />
          <div class="flex-1 min-w-0">
            <div class="text-sm text-text">Calendar events</div>
            <div class="text-[11px] text-dim leading-snug">Fires at the configured "remind me N min before" on each event.</div>
            <label class="mt-1.5 flex items-center gap-2 text-[11px] text-dim">
              Default offset
              <select
                bind:value={prefs.default_event_reminder}
                onchange={() => savePrefs()}
                class="bg-mantle border border-surface1 rounded px-2 py-1 text-text text-xs"
              >
                <option value={0}>off</option>
                <option value={5}>5 minutes before</option>
                <option value={15}>15 minutes before</option>
                <option value={30}>30 minutes before</option>
                <option value={60}>1 hour before</option>
                <option value={1440}>1 day before</option>
              </select>
            </label>
          </div>
        </div>

        <!-- Tasks due today -->
        <div class="flex items-start gap-2.5">
          <input
            type="checkbox"
            bind:checked={prefs.tasks.enabled}
            onchange={() => savePrefs()}
            class="mt-1 w-4 h-4 accent-primary"
          />
          <div class="flex-1 min-w-0">
            <div class="text-sm text-text">Tasks due today</div>
            <div class="text-[11px] text-dim leading-snug">One morning summary listing tasks whose due date is today.</div>
            <label class="mt-1.5 flex items-center gap-2 text-[11px] text-dim">
              Reminder time
              <input
                type="time"
                bind:value={prefs.tasks.due_today_time}
                onchange={() => savePrefs()}
                class="bg-mantle border border-surface1 rounded px-2 py-1 text-text text-xs font-mono tabular-nums"
              />
            </label>
          </div>
        </div>

        <!-- Deadlines -->
        <div class="flex items-start gap-2.5">
          <input
            type="checkbox"
            bind:checked={prefs.deadlines.enabled}
            onchange={() => savePrefs()}
            class="mt-1 w-4 h-4 accent-primary"
          />
          <div class="flex-1 min-w-0">
            <div class="text-sm text-text">Deadlines</div>
            <div class="text-[11px] text-dim leading-snug">Fire at each chosen offset before a deadline (one push per offset).</div>
            <div class="mt-1.5 flex items-center gap-1 flex-wrap">
              {#each [14, 7, 3, 1, 0] as off (off)}
                {@const active = prefs.deadlines.days_before.includes(off)}
                <button
                  type="button"
                  onclick={() => { toggleDeadlineOffset(off); savePrefs(); }}
                  class="px-2 py-0.5 text-[11px] rounded border transition-colors
                    {active ? 'bg-surface1 border-primary text-primary' : 'bg-surface0 border-surface1 text-dim hover:border-primary'}"
                >{off === 0 ? 'day-of' : `${off}d`}</button>
              {/each}
            </div>
            <label class="mt-1.5 flex items-center gap-2 text-[11px] text-dim">
              Reminder time
              <input
                type="time"
                bind:value={prefs.deadlines.at_time}
                onchange={() => savePrefs()}
                class="bg-mantle border border-surface1 rounded px-2 py-1 text-text text-xs font-mono tabular-nums"
              />
            </label>
          </div>
        </div>

        <!-- Quiet hours -->
        <div class="flex items-start gap-2.5 pt-2 border-t border-surface1">
          <input
            type="checkbox"
            bind:checked={prefs.quiet_hours.enabled}
            onchange={() => savePrefs()}
            class="mt-1 w-4 h-4 accent-primary"
          />
          <div class="flex-1 min-w-0">
            <div class="text-sm text-text">Quiet hours</div>
            <div class="text-[11px] text-dim leading-snug">No pushes between these times — across all categories.</div>
            <div class="mt-1.5 flex items-center gap-2 text-[11px] text-dim">
              From
              <input
                type="time"
                bind:value={prefs.quiet_hours.start}
                onchange={() => savePrefs()}
                class="bg-mantle border border-surface1 rounded px-2 py-1 text-text text-xs font-mono tabular-nums"
              />
              to
              <input
                type="time"
                bind:value={prefs.quiet_hours.end}
                onchange={() => savePrefs()}
                class="bg-mantle border border-surface1 rounded px-2 py-1 text-text text-xs font-mono tabular-nums"
              />
            </div>
          </div>
        </div>
      </div>
    {/if}
  {/snippet}
</SettingsSection>

<!-- Git auto-sync (status / sync now). Pull → commit → push on the
     interval the daemon was started with. -->
<SettingsSection title="Git auto-sync">
  {#snippet children()}
    {#if !sync}
      <Skeleton class="h-4 w-1/2" />
    {:else if !sync.enabled}
      <div class="text-sm text-dim leading-snug space-y-1.5 py-1">
        <p>
          Off. To enable periodic <code class="text-xs">git pull</code> + commit/push, add
          <code class="text-xs">--sync --sync-interval 60s</code> to the granit command in your <code class="text-xs">docker-compose.yml</code>, then <code class="text-xs">docker compose up -d</code>.
        </p>
        <details>
          <summary class="text-[11px] text-dim hover:text-text cursor-pointer">Manual one-off sync (no daemon)</summary>
          <pre class="text-xs font-mono px-2 py-1.5 mt-1 bg-mantle border border-surface1 rounded overflow-x-auto"><code>cd /srv/granit-vault
git add -A
git commit -m "manual sync $(date +%F)"
git push</code></pre>
        </details>
      </div>
    {:else}
      <div class="flex items-baseline gap-2 mb-1.5">
        <button
          type="button"
          onclick={syncNow}
          disabled={syncBusy}
          class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90 disabled:opacity-50"
        >{syncBusy ? '…' : 'Sync now'}</button>
        <span class="text-[11px] text-dim">interval <span class="text-subtext">{sync.interval ?? '—'}</span></span>
      </div>
      <dl class="text-[12px] space-y-0.5">
        <div class="flex gap-3">
          <dt class="text-dim w-20 flex-shrink-0">Last pull</dt>
          <dd class="text-text">{fmtTime(sync.lastPull)} ({sync.pulls ?? 0} total)</dd>
        </div>
        <div class="flex gap-3">
          <dt class="text-dim w-20 flex-shrink-0">Last push</dt>
          <dd class="text-text">{fmtTime(sync.lastPush)} ({sync.pushes ?? 0} total)</dd>
        </div>
        {#if sync.lastErr}
          <div class="flex gap-3">
            <dt class="text-dim w-20 flex-shrink-0">Error</dt>
            <dd class="text-error">{sync.lastErr}</dd>
          </div>
        {/if}
      </dl>
    {/if}
  {/snippet}
</SettingsSection>

<!-- Autocommit — debounced git-commit-on-save. -->
<SettingsSection title="Git autocommit" status={autocommitSaving ? 'saving…' : undefined}>
  {#snippet children()}
    <label class="flex items-start gap-2.5 cursor-pointer py-1">
      <input
        type="checkbox"
        checked={autocommit.enabled}
        onchange={(e) => void toggleAutocommit((e.target as HTMLInputElement).checked)}
        disabled={autocommitSaving}
        class="w-4 h-4 mt-0.5 accent-primary cursor-pointer"
      />
      <div class="flex-1 min-w-0">
        <div class="text-sm text-text">Auto-commit changes to git</div>
        <div class="text-[11px] text-dim leading-snug">
          {#if autocommit.isGitRepo}
            Coalesced commit ~30s after the last save. Single tidy commit per work session.
          {:else}
            <span class="text-warning">Vault is not a git repository — toggle does nothing until you run <code class="text-[10px]">git init</code> in the vault.</span>
          {/if}
        </div>
      </div>
    </label>
  {/snippet}
</SettingsSection>

<!-- Stoicera intranet integration. Off by default. -->
<SettingsSection
  title="Stoicera intranet"
  status={stoiceraSaving ? 'saving…' : undefined}
  advancedLabel="Show token + venture config"
>
  {#snippet children()}
    <p class="text-[12px] text-dim mb-1.5 leading-snug">
      Read-only API at <code class="text-[10px]">/api/v1/integrations/stoicera/*</code> for the stoicera-intranet app. Off until you name a venture and enable.
    </p>
    <label class="flex items-start gap-2.5 cursor-pointer py-1">
      <input
        type="checkbox"
        checked={stoiceraSettings.enabled}
        onchange={(e) => void toggleStoicera((e.target as HTMLInputElement).checked)}
        class="w-4 h-4 mt-0.5 accent-primary cursor-pointer"
      />
      <div class="flex-1 min-w-0">
        <div class="text-sm text-text">Enable integration</div>
        <div class="text-[11px] text-dim leading-snug">
          When off, all integration endpoints return 404 — the existence of the feature is hidden behind a reverse proxy.
        </div>
      </div>
    </label>
  {/snippet}

  {#snippet advanced()}
    <div class="space-y-2">
      <div>
        <label class="text-[11px] uppercase tracking-wider text-dim block mb-1" for="stoicera-venture">Venture name</label>
        <input
          id="stoicera-venture"
          type="text"
          bind:value={stoiceraVentureBuf}
          onblur={() => void commitStoiceraVenture()}
          onkeydown={(e) => { if (e.key === 'Enter') void commitStoiceraVenture(); }}
          placeholder="Stoicera"
          disabled={!stoiceraSettings.enabled}
          class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm font-mono disabled:opacity-50"
        />
        <div class="text-[11px] text-dim mt-1 leading-snug">
          Only projects + goals whose <code>venture:</code> field matches this string (case-insensitive) surface through the integration.
        </div>
      </div>
      {#if stoiceraSettings.has_token}
        <div>
          <div class="text-[11px] uppercase tracking-wider text-dim mb-1">Integration token</div>
          <div class="flex flex-wrap items-center gap-1.5">
            <code class="text-xs font-mono px-2 py-1 bg-mantle border border-surface1 rounded text-text flex-1 min-w-0 break-all">{stoiceraTokenRevealed ?? stoiceraSettings.token_masked}</code>
            {#if stoiceraTokenRevealed === null}
              <button type="button" onclick={() => void revealStoiceraToken()} class="text-[11px] px-2 py-1 bg-surface0 border border-surface1 rounded text-subtext hover:border-primary">Show</button>
            {:else}
              <button type="button" onclick={() => setStoiceraTokenRevealed(null)} class="text-[11px] px-2 py-1 bg-surface0 border border-surface1 rounded text-subtext hover:border-primary">Hide</button>
            {/if}
            <button type="button" onclick={() => void copyStoiceraToken()} class="text-[11px] px-2 py-1 bg-surface0 border border-surface1 rounded text-subtext hover:border-primary">Copy</button>
            <ConfirmButton
              label="Regenerate"
              confirmLabel="Regenerate?"
              danger
              title="Invalidate the current token and issue a new one"
              onconfirm={regenerateStoiceraToken}
            />
          </div>
          <div class="text-[11px] text-dim mt-1 leading-snug">
            Configure the stoicera-intranet app to send <code>Authorization: Bearer &lt;token&gt;</code>.
          </div>
        </div>
      {/if}
    </div>
  {/snippet}
</SettingsSection>

<!-- Devices — every browser/laptop with an active session. -->
<SettingsSection title="Devices" status={`${devices.length} active`}>
  {#snippet children()}
    {#if devices.length === 0}
      <p class="text-sm text-dim italic py-1">No active sessions.</p>
    {:else}
      <ul class="divide-y divide-surface1">
        {#each devices as d (d.id)}
          <li class="py-2 flex items-center gap-2.5">
            <div class="w-7 h-7 rounded bg-surface1 flex items-center justify-center text-subtext flex-shrink-0">
              {#if d.label === 'iOS' || d.label === 'Android'}
                <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2"><rect x="6" y="2" width="12" height="20" rx="2"/><path d="M11 18h2"/></svg>
              {:else}
                <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="3" y="5" width="18" height="11" rx="1"/><path d="M2 20h20"/></svg>
              {/if}
            </div>
            <div class="flex-1 min-w-0">
              <div class="flex items-baseline gap-2 flex-wrap">
                <span class="text-sm text-text font-medium">{d.label || 'Unknown'}</span>
                {#if d.current}
                  <span class="text-[10px] uppercase px-1.5 py-0.5 rounded bg-surface0 text-success">this device</span>
                {/if}
                <code class="text-[10px] text-dim font-mono">{d.id}</code>
              </div>
              <div class="text-[11px] text-dim">
                active {fmtRelative(d.lastUsed)} · created {fmtRelative(d.createdAt)}
              </div>
            </div>
            {#if !d.current}
              <ConfirmButton
                label={revokeBusy === d.id ? 'revoking…' : 'Revoke'}
                confirmLabel="Revoke?"
                danger
                disabled={revokeBusy === d.id}
                title="Sign out this device"
                onconfirm={() => revokeDevice(d.id)}
              />
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
  {/snippet}
</SettingsSection>
