<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api } from '$lib/api';
  import { onWsEvent, wsConnected } from '$lib/ws';
  import { theme, themeIcon, themeLabel, type Theme } from '$lib/stores/theme';
  import PageHeader from '$lib/components/PageHeader.svelte';
  import Skeleton from '$lib/components/Skeleton.svelte';

  type SyncStatus = {
    enabled: boolean;
    interval?: string;
    lastPull?: string;
    lastPush?: string;
    pulls?: number;
    pushes?: number;
    lastErr?: string;
  };

  let vault = $state<{ root: string; notes: number } | null>(null);
  let sync = $state<SyncStatus | null>(null);
  let authStatus = $state<{ hasPassword: boolean; sessionCount?: number; setupAt?: string } | null>(null);
  let devices = $state<import('$lib/api').Device[]>([]);
  let revokeBusy = $state<string | null>(null);

  // Change-password panel state — hidden until the user opens it.
  let pwOpen = $state(false);
  let pwOld = $state('');
  let pwNew = $state('');
  let pwConfirm = $state('');
  let pwBusy = $state(false);
  let pwError = $state('');
  let pwSuccess = $state('');

  async function load() {
    if (!$auth) return;
    try {
      const [v, s, a, d] = await Promise.all([
        api.vault(),
        api.req<SyncStatus>('/sync'),
        api.authStatus(),
        api.listDevices().catch(() => ({ devices: [] }))
      ]);
      vault = v;
      sync = s;
      authStatus = a;
      devices = d.devices;
    } catch {}
  }

  async function revokeDevice(id: string) {
    revokeBusy = id;
    try {
      await api.revokeDevice(id);
      devices = devices.filter((d) => d.id !== id);
    } catch (e) {
      pwError = e instanceof Error ? e.message : String(e);
    } finally {
      revokeBusy = null;
    }
  }

  function fmtRelative(iso: string): string {
    const d = new Date(iso);
    if (isNaN(d.getTime())) return iso;
    const mins = Math.round((Date.now() - d.getTime()) / 60_000);
    if (mins < 1) return 'just now';
    if (mins < 60) return `${mins} min ago`;
    if (mins < 24 * 60) return `${Math.round(mins / 60)}h ago`;
    const days = Math.round(mins / (24 * 60));
    if (days < 7) return `${days}d ago`;
    return d.toLocaleDateString();
  }

  async function changePassword(e: Event) {
    e.preventDefault();
    pwError = ''; pwSuccess = '';
    if (pwNew.length < 6) { pwError = 'new password must be at least 6 characters'; return; }
    if (pwNew !== pwConfirm) { pwError = 'passwords do not match'; return; }
    pwBusy = true;
    try {
      await api.authChangePassword(pwOld, pwNew);
      pwSuccess = 'password changed — all sessions revoked, please sign in again';
      pwOld = ''; pwNew = ''; pwConfirm = '';
      // Server invalidates the current token on password change. Clear
      // local auth so the user routes back to login.
      setTimeout(() => { auth.clear(); }, 1500);
    } catch (e) {
      pwError = e instanceof Error ? e.message : String(e);
    } finally {
      pwBusy = false;
    }
  }

  async function revokeAllSessions() {
    if (!confirm('Sign out everywhere? You will need to log in again on every device.')) return;
    try {
      await api.authRevokeAll();
      auth.clear();
    } catch (e) {
      pwError = e instanceof Error ? e.message : String(e);
    }
  }
  onMount(() => {
    load();
    return onWsEvent(() => load());
  });

  const themeOptions: Theme[] = ['system', 'light', 'dark'];

  // Keyboard shortcuts list (mirrors what's actually wired)
  const shortcuts: { keys: string; what: string }[] = [
    { keys: '⌘ K  /  Ctrl+K', what: 'Open command palette / search' },
    { keys: '⌘ S  /  Ctrl+S', what: 'Save the current note' },
    { keys: '⌘ F  /  Ctrl+F', what: 'Find in editor' },
    { keys: '⌘ Z  /  Ctrl+Z', what: 'Undo' },
    { keys: '↵',              what: 'Submit (in any form)' },
    { keys: 'Esc',            what: 'Close modal / palette' }
  ];

  function fmtTime(s?: string): string {
    if (!s || s.startsWith('0001-')) return '—';
    return new Date(s).toLocaleString();
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-3xl mx-auto">
    <PageHeader title="Settings" subtitle="Theme, keyboard shortcuts, vault info, sync status" />

    <!-- Theme -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Theme</h2>
      <div class="grid grid-cols-3 gap-2">
        {#each themeOptions as t}
          {@const active = $theme === t}
          <button
            onclick={() => theme.set(t)}
            class="px-3 py-3 rounded-lg border-2 flex flex-col items-center gap-1
              {active ? 'border-primary bg-primary/10 text-primary' : 'border-surface1 bg-mantle text-subtext hover:border-primary/40'}"
          >
            <span class="text-2xl">{themeIcon(t)}</span>
            <span class="text-xs font-medium">{themeLabel(t)}</span>
          </button>
        {/each}
      </div>
      <p class="text-xs text-dim mt-2 leading-relaxed">
        System follows your OS setting and updates live. Stored in <code class="text-[10px]">localStorage</code>.
      </p>
    </section>

    <!-- Security -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Security</h2>
      {#if !authStatus}
        <Skeleton class="h-4 w-1/2" />
      {:else}
        <dl class="space-y-1.5 text-sm mb-3">
          <div class="flex gap-3">
            <dt class="text-dim w-28 flex-shrink-0">Password</dt>
            <dd class="text-text">{authStatus.hasPassword ? 'set' : 'not set — paste-token mode'}</dd>
          </div>
          {#if authStatus.setupAt}
            <div class="flex gap-3">
              <dt class="text-dim w-28 flex-shrink-0">Created</dt>
              <dd class="text-text">{new Date(authStatus.setupAt).toLocaleDateString()}</dd>
            </div>
          {/if}
          <div class="flex gap-3">
            <dt class="text-dim w-28 flex-shrink-0">Active sessions</dt>
            <dd class="text-text">{authStatus.sessionCount ?? 0}</dd>
          </div>
        </dl>

        {#if authStatus.hasPassword}
          {#if !pwOpen}
            <div class="flex gap-2 flex-wrap">
              <button onclick={() => (pwOpen = true)} class="px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded hover:border-primary text-text">
                Change password
              </button>
              <button onclick={revokeAllSessions} class="px-3 py-1.5 text-xs text-error hover:bg-error/10 rounded border border-error/30">
                Sign out everywhere
              </button>
            </div>
          {:else}
            <form onsubmit={changePassword} class="space-y-2 mt-2">
              <input type="password" bind:value={pwOld} placeholder="current password" autocomplete="current-password" required class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-sm text-text" />
              <input type="password" bind:value={pwNew} placeholder="new password" autocomplete="new-password" required class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-sm text-text" />
              <input type="password" bind:value={pwConfirm} placeholder="confirm new password" autocomplete="new-password" required class="w-full px-3 py-2 bg-mantle border border-surface1 rounded text-sm text-text" />
              {#if pwError}<div class="text-sm text-error">{pwError}</div>{/if}
              {#if pwSuccess}<div class="text-sm text-success">{pwSuccess}</div>{/if}
              <div class="flex gap-2">
                <button type="submit" disabled={pwBusy} class="px-3 py-1.5 text-xs bg-primary text-mantle rounded disabled:opacity-50">
                  {pwBusy ? 'changing…' : 'Change password'}
                </button>
                <button type="button" onclick={() => { pwOpen = false; pwError = ''; pwOld = pwNew = pwConfirm = ''; }} class="px-3 py-1.5 text-xs text-dim hover:text-text">cancel</button>
              </div>
            </form>
          {/if}
        {:else}
          <p class="text-xs text-dim italic">Sign in once via the home page to set a password.</p>
        {/if}
      {/if}
    </section>

    <!-- Devices — every browser/laptop with an active session. The
         current device is highlighted. Each row can be revoked, which
         signs that device out without touching the password. -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <div class="flex items-baseline justify-between mb-3">
        <h2 class="text-xs uppercase tracking-wider text-dim font-medium">Devices</h2>
        <span class="text-xs text-dim">{devices.length} active</span>
      </div>
      {#if devices.length === 0}
        <p class="text-sm text-dim italic">No active sessions. (You can't see this — you're logged in.)</p>
      {:else}
        <ul class="divide-y divide-surface1">
          {#each devices as d (d.id)}
            <li class="py-2.5 flex items-center gap-3">
              <!-- Per-device-type icon. The label was set by the browser
                   on login (Web/iOS/macOS/Linux/Windows) — best-effort
                   identifier the user can recognize. -->
              <div class="w-8 h-8 rounded bg-surface1 flex items-center justify-center text-subtext flex-shrink-0">
                {#if d.label === 'iOS' || d.label === 'Android'}
                  <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2"><rect x="6" y="2" width="12" height="20" rx="2"/><path d="M11 18h2"/></svg>
                {:else}
                  <svg viewBox="0 0 24 24" class="w-4 h-4" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round"><rect x="3" y="5" width="18" height="11" rx="1"/><path d="M2 20h20"/></svg>
                {/if}
              </div>
              <div class="flex-1 min-w-0">
                <div class="flex items-baseline gap-2">
                  <span class="text-sm text-text font-medium">{d.label || 'Unknown device'}</span>
                  {#if d.current}
                    <span class="text-[10px] uppercase px-1.5 py-0.5 rounded bg-success/15 text-success">this device</span>
                  {/if}
                  <code class="text-[10px] text-dim font-mono">{d.id}</code>
                </div>
                <div class="text-xs text-dim">
                  active {fmtRelative(d.lastUsed)} · created {fmtRelative(d.createdAt)}
                </div>
              </div>
              {#if !d.current}
                <button
                  onclick={() => revokeDevice(d.id)}
                  disabled={revokeBusy === d.id}
                  class="px-2.5 py-1 text-xs text-error hover:bg-error/10 rounded border border-error/30 disabled:opacity-50 flex-shrink-0"
                >{revokeBusy === d.id ? 'revoking…' : 'revoke'}</button>
              {/if}
            </li>
          {/each}
        </ul>
        <p class="text-[11px] text-dim italic mt-3">
          A new row appears each time you log in from a different browser, phone, or laptop.
          Revoking signs that device out without affecting the others.
        </p>
      {/if}
    </section>

    <!-- Vault info -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Vault</h2>
      {#if !vault}
        <Skeleton class="h-4 w-2/3 mb-2" />
        <Skeleton class="h-4 w-1/2" />
      {:else}
        <dl class="space-y-1.5 text-sm">
          <div class="flex gap-3">
            <dt class="text-dim w-24 flex-shrink-0">Root</dt>
            <dd class="text-text font-mono text-xs break-all">{vault.root}</dd>
          </div>
          <div class="flex gap-3">
            <dt class="text-dim w-24 flex-shrink-0">Notes</dt>
            <dd class="text-text">{vault.notes}</dd>
          </div>
          <div class="flex gap-3">
            <dt class="text-dim w-24 flex-shrink-0">Live</dt>
            <dd class="flex items-center gap-2">
              <span class="w-2 h-2 rounded-full {$wsConnected ? 'bg-success' : 'bg-dim'}"></span>
              <span class="text-text">{$wsConnected ? 'connected' : 'offline'}</span>
            </dd>
          </div>
        </dl>
      {/if}
    </section>

    <!-- Sync status -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Git auto-sync</h2>
      {#if !sync}
        <Skeleton class="h-4 w-1/2" />
      {:else if !sync.enabled}
        <p class="text-sm text-dim leading-relaxed">
          Disabled. Pass <code class="text-xs">--sync</code> to <code class="text-xs">granit web</code> to enable periodic
          <code class="text-xs">git pull</code> + auto-commit/push when this server hosts a git-backed vault.
        </p>
      {:else}
        <dl class="space-y-1.5 text-sm">
          <div class="flex gap-3">
            <dt class="text-dim w-28 flex-shrink-0">Interval</dt>
            <dd class="text-text">{sync.interval ?? '—'}</dd>
          </div>
          <div class="flex gap-3">
            <dt class="text-dim w-28 flex-shrink-0">Last pull</dt>
            <dd class="text-text">{fmtTime(sync.lastPull)} ({sync.pulls ?? 0} total)</dd>
          </div>
          <div class="flex gap-3">
            <dt class="text-dim w-28 flex-shrink-0">Last push</dt>
            <dd class="text-text">{fmtTime(sync.lastPush)} ({sync.pushes ?? 0} total)</dd>
          </div>
          {#if sync.lastErr}
            <div class="flex gap-3">
              <dt class="text-dim w-28 flex-shrink-0">Error</dt>
              <dd class="text-error text-xs">{sync.lastErr}</dd>
            </div>
          {/if}
        </dl>
      {/if}
    </section>

    <!-- Keyboard shortcuts -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4 mb-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">Keyboard shortcuts</h2>
      <ul class="space-y-1.5 text-sm">
        {#each shortcuts as s}
          <li class="flex items-baseline gap-3">
            <kbd class="text-xs font-mono px-2 py-0.5 bg-mantle border border-surface1 rounded text-text whitespace-nowrap">{s.keys}</kbd>
            <span class="text-subtext">{s.what}</span>
          </li>
        {/each}
      </ul>
    </section>

    <!-- About -->
    <section class="bg-surface0 border border-surface1 rounded-lg p-4">
      <h2 class="text-xs uppercase tracking-wider text-dim font-medium mb-3">About</h2>
      <p class="text-sm text-subtext leading-relaxed">
        Granit web — your vault, anywhere. Powered by the same data layer as the granit TUI;
        learn more at <a href="https://github.com/artaeon/granit" rel="noreferrer" target="_blank">github.com/artaeon/granit</a>.
      </p>
      <div class="mt-3">
        <button
          onclick={() => auth.clear()}
          class="text-xs text-error hover:underline"
        >
          Sign out (clears the bearer token from this device)
        </button>
      </div>
    </section>
  </div>
</div>
