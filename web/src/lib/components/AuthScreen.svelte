<script lang="ts">
  import { onMount } from 'svelte';
  import { api } from '$lib/api';
  import { auth } from '$lib/stores/auth';
  import { focusOnMount } from '$lib/util/focusOnMount';

  // Self-contained auth surface: setup / login / advanced (bearer
  // token) flows in one shell. Lives outside +page.svelte so the
  // dashboard page reads as "dashboard" and the auth screen reads as
  // "auth". Renders only when the parent has verified no valid token
  // exists; on success it writes the token to the auth store, the
  // parent re-renders the dashboard branch.
  //
  // Three screens behind one component:
  //   loading   — pre-flight /auth/status fetch
  //   setup     — no password set yet → create one
  //   login     — password set → enter it (default)
  //   advanced  — paste a CLI bearer token (fallback / power user)

  type AuthScreen = 'loading' | 'setup' | 'login' | 'advanced';
  let authScreen = $state<AuthScreen>('loading');
  let setupAt = $state<string | null>(null);
  let password = $state('');
  let passwordConfirm = $state('');
  let tokenInput = $state('');
  let signingIn = $state(false);
  let signInError = $state('');

  onMount(() => {
    void refreshAuthScreen();
  });

  async function refreshAuthScreen() {
    try {
      const r = await api.authStatus();
      authScreen = r.hasPassword ? 'login' : 'setup';
      setupAt = r.setupAt ?? null;
    } catch {
      // Server unreachable — fall back to advanced (token paste) so the
      // user can at least diagnose. Better than a dead form.
      authScreen = 'advanced';
    }
  }

  function deviceLabel(): string {
    if (typeof navigator === 'undefined') return '';
    // Compact UA hint — saved alongside the session so the user can
    // identify it later when revoking. No fingerprinting beyond UA.
    const ua = navigator.userAgent;
    if (/iPhone|iPad/.test(ua)) return 'iOS';
    if (/Android/.test(ua)) return 'Android';
    if (/Mac OS X/.test(ua)) return 'macOS';
    if (/Linux/.test(ua)) return 'Linux';
    if (/Windows/.test(ua)) return 'Windows';
    return 'Web';
  }

  async function setupPassword(e: Event) {
    e.preventDefault();
    if (password.length < 6) { signInError = 'password must be at least 6 characters'; return; }
    if (password !== passwordConfirm) { signInError = 'passwords do not match'; return; }
    signingIn = true;
    signInError = '';
    try {
      const r = await api.authSetup(password, deviceLabel());
      auth.setToken(r.token);
      password = ''; passwordConfirm = '';
    } catch (e) {
      signInError = e instanceof Error ? e.message : String(e);
    } finally {
      signingIn = false;
    }
  }

  async function login(e: Event) {
    e.preventDefault();
    if (!password) return;
    signingIn = true;
    signInError = '';
    try {
      const r = await api.authLogin(password, deviceLabel());
      auth.setToken(r.token);
      password = '';
    } catch (e) {
      signInError = e instanceof Error ? e.message : 'invalid password';
    } finally {
      signingIn = false;
    }
  }

  async function signInWithToken(e: Event) {
    e.preventDefault();
    if (!tokenInput.trim()) return;
    signingIn = true;
    signInError = '';
    auth.setToken(tokenInput.trim());
    try {
      await api.vault();
      tokenInput = '';
    } catch {
      auth.clear();
      signInError = 'invalid token';
    } finally {
      signingIn = false;
    }
  }
</script>

<div class="h-full overflow-y-auto flex items-center justify-center p-4 sm:p-8">
  <div class="w-full max-w-sm bg-mantle border border-surface1 rounded-lg p-5 sm:p-6 space-y-4">
    <div>
      <h1 class="text-lg font-semibold text-text">Granit</h1>
      {#if authScreen === 'setup'}
        <p class="text-sm text-dim mt-1">First launch — set a password to secure your vault.</p>
      {:else if authScreen === 'login'}
        <p class="text-sm text-dim mt-1">Sign in with your password</p>
        {#if setupAt}<p class="text-[11px] text-dim/70 mt-0.5">Account created {new Date(setupAt).toLocaleDateString()}</p>{/if}
      {:else if authScreen === 'advanced'}
        <p class="text-sm text-dim mt-1">Paste your bearer token</p>
      {:else}
        <p class="text-sm text-dim mt-1">Checking…</p>
      {/if}
    </div>

    {#if authScreen === 'setup'}
      <form onsubmit={setupPassword} class="space-y-3">
        <div>
          <label for="pw" class="block text-xs uppercase tracking-wider text-dim mb-1">New password</label>
          <input
            id="pw"
            type="password"
            autocomplete="new-password"
            bind:value={password}
            placeholder="at least 6 characters"
            required
            class="w-full px-3 py-3 bg-surface0 border border-surface1 rounded text-base text-text placeholder-dim focus:outline-none focus:border-primary"
          />
        </div>
        <div>
          <label for="pwc" class="block text-xs uppercase tracking-wider text-dim mb-1">Confirm password</label>
          <input
            id="pwc"
            type="password"
            autocomplete="new-password"
            bind:value={passwordConfirm}
            required
            class="w-full px-3 py-3 bg-surface0 border border-surface1 rounded text-base text-text placeholder-dim focus:outline-none focus:border-primary"
          />
        </div>
        {#if signInError}<div class="text-sm text-error">{signInError}</div>{/if}
        <button type="submit" disabled={signingIn || !password} class="w-full px-3 py-3 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90 disabled:opacity-50">
          {signingIn ? 'creating…' : 'Set password & sign in'}
        </button>
        <p class="text-[11px] text-dim text-center">
          Stored as an argon2id hash in <code>.granit/web-auth.json</code>.
          Tip: use a passphrase you can remember — there's no recovery flow.
        </p>
      </form>
    {:else if authScreen === 'login'}
      <form onsubmit={login} class="space-y-3">
        <input
          type="password"
          autocomplete="current-password"
          bind:value={password}
          placeholder="password"
          required
          use:focusOnMount
          class="w-full px-3 py-3 bg-surface0 border border-surface1 rounded text-base text-text placeholder-dim focus:outline-none focus:border-primary"
        />
        {#if signInError}<div class="text-sm text-error">{signInError}</div>{/if}
        <button type="submit" disabled={signingIn || !password} class="w-full px-3 py-3 bg-primary text-on-primary rounded text-sm font-medium hover:opacity-90 disabled:opacity-50">
          {signingIn ? 'signing in…' : 'Sign in'}
        </button>
        <button type="button" onclick={() => { authScreen = 'advanced'; signInError = ''; }} class="w-full text-xs text-dim hover:text-text">
          use bearer token instead
        </button>
      </form>
    {:else if authScreen === 'advanced'}
      <form onsubmit={signInWithToken} class="space-y-3">
        <input
          type="password"
          bind:value={tokenInput}
          placeholder="bearer token (CLI)"
          class="w-full px-3 py-3 bg-surface0 border border-surface1 rounded text-base text-text placeholder-dim focus:outline-none focus:border-primary font-mono"
        />
        {#if signInError}<div class="text-sm text-error">{signInError}</div>{/if}
        <button type="submit" disabled={signingIn || !tokenInput.trim()} class="w-full px-3 py-3 bg-surface0 border border-surface1 text-text rounded text-sm font-medium hover:border-primary disabled:opacity-50">
          {signingIn ? 'signing in…' : 'Sign in with token'}
        </button>
        <button type="button" onclick={refreshAuthScreen} class="w-full text-xs text-dim hover:text-text">
          ← back to password
        </button>
        <p class="text-[11px] text-dim break-words">
          Token is stored in <code>.granit/everything-token</code> on the server. Use this for CLI scripts only.
        </p>
      </form>
    {:else}
      <div class="text-sm text-dim text-center py-4">…</div>
    {/if}
  </div>
</div>
