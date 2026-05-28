<script lang="ts">
  // Vault tab — vault info (root path, note count, live/offline)
  // and security (password + sessions). Two short sections,
  // intentionally minimal — this is where you come to check facts
  // and rotate credentials, not to tweak knobs.
  import Skeleton from '$lib/components/Skeleton.svelte';
  import SettingsSection from './SettingsSection.svelte';
  import ConfirmButton from './ConfirmButton.svelte';

  type AuthStatus = { hasPassword: boolean; sessionCount?: number; setupAt?: string };

  type Props = {
    vault: { root: string; notes: number } | null;
    wsConnected: boolean;
    authStatus: AuthStatus | null;
    pwOpen: boolean;
    pwOld: string;
    pwNew: string;
    pwConfirm: string;
    pwBusy: boolean;
    pwError: string;
    pwSuccess: string;
    setPwOpen: (v: boolean) => void;
    resetPwForm: () => void;
    changePassword: (e: Event) => Promise<void>;
    revokeAllSessions: () => Promise<void>;
  };

  let {
    vault,
    wsConnected,
    authStatus,
    pwOpen,
    pwOld = $bindable(),
    pwNew = $bindable(),
    pwConfirm = $bindable(),
    pwBusy,
    pwError,
    pwSuccess,
    setPwOpen,
    resetPwForm,
    changePassword,
    revokeAllSessions
  }: Props = $props();
</script>

<!-- Vault — root path, note count, live status -->
<SettingsSection title="Vault">
  {#snippet children()}
    {#if !vault}
      <Skeleton class="h-4 w-2/3 mb-1" />
      <Skeleton class="h-4 w-1/2" />
    {:else}
      <dl class="text-sm space-y-1 py-1">
        <div class="flex gap-3">
          <dt class="text-dim w-20 flex-shrink-0">Root</dt>
          <dd class="text-text font-mono text-xs break-all">{vault.root}</dd>
        </div>
        <div class="flex gap-3">
          <dt class="text-dim w-20 flex-shrink-0">Notes</dt>
          <dd class="text-text tabular-nums">{vault.notes}</dd>
        </div>
        <div class="flex gap-3">
          <dt class="text-dim w-20 flex-shrink-0">Live</dt>
          <dd class="flex items-center gap-2">
            <span class="w-2 h-2 rounded-full {wsConnected ? 'bg-success' : 'bg-dim'}"></span>
            <span class="text-text text-xs">{wsConnected ? 'connected' : 'offline'}</span>
          </dd>
        </div>
      </dl>
    {/if}
  {/snippet}
</SettingsSection>

<!-- Security — password + active sessions -->
<SettingsSection title="Security">
  {#snippet children()}
    {#if !authStatus}
      <Skeleton class="h-4 w-1/2" />
    {:else}
      <dl class="text-sm space-y-1 py-1 mb-2">
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
          <dd class="text-text tabular-nums">{authStatus.sessionCount ?? 0}</dd>
        </div>
      </dl>

      {#if authStatus.hasPassword}
        {#if !pwOpen}
          <div class="flex gap-1.5 flex-wrap">
            <button
              type="button"
              onclick={() => setPwOpen(true)}
              class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded hover:border-primary text-text"
            >Change password</button>
            <ConfirmButton
              label="Sign out everywhere"
              confirmLabel="Sign out all devices?"
              danger
              title="Revoke every active session across all devices"
              onconfirm={revokeAllSessions}
            />
          </div>
        {:else}
          <form onsubmit={changePassword} class="space-y-2 mt-1">
            <input type="password" bind:value={pwOld} placeholder="current password" autocomplete="current-password" required class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text" />
            <input type="password" bind:value={pwNew} placeholder="new password" autocomplete="new-password" required class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text" />
            <input type="password" bind:value={pwConfirm} placeholder="confirm new password" autocomplete="new-password" required class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text" />
            {#if pwError}<div class="text-sm text-error">{pwError}</div>{/if}
            {#if pwSuccess}<div class="text-sm text-success">{pwSuccess}</div>{/if}
            <div class="flex gap-2">
              <button type="submit" disabled={pwBusy} class="px-2.5 py-1 text-xs bg-primary text-on-primary rounded disabled:opacity-50">
                {pwBusy ? 'changing…' : 'Change password'}
              </button>
              <button type="button" onclick={() => { setPwOpen(false); resetPwForm(); }} class="px-2.5 py-1 text-xs text-dim hover:text-text">cancel</button>
            </div>
          </form>
        {/if}
      {:else}
        <p class="text-xs text-dim italic py-1">Sign in once via the home page to set a password.</p>
      {/if}
    {/if}
  {/snippet}
</SettingsSection>
