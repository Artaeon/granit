<script lang="ts">
  // The home route. Two distinct surfaces stacked on auth state:
  //
  //   1. Logged out → password setup / login / token paste forms.
  //      The branch that gets new users into the app.
  //   2. Logged in  → the Heute-Karte (Rhythmus-OS surface).
  //      Single quiet card with five pillars, mode picker, and the
  //      derived next-action verb. NOT a widget grid. See
  //      $lib/rhythmus/Heute.svelte for what renders here.
  //
  // The widget-grid dashboard that used to live here (config-driven,
  // 13+ tiles, drag-to-reorder) was retired on 2026-05-19 in favour
  // of the Rhythmus-OS direction. The dashboard config endpoint
  // still exists on the server and other surfaces can keep using it
  // — this page just stops being its renderer.
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api } from '$lib/api';
  import { focusOnMount } from '$lib/util/focusOnMount';
  import Heute from '$lib/rhythmus/Heute.svelte';

  // Auth state machine on the landing page:
  //   loading      → checking /auth/status
  //   setup        → no password yet → set one
  //   login        → password is set → enter it
  //   advanced     → user wants to paste a CLI bearer token instead
  type AuthScreen = 'loading' | 'setup' | 'login' | 'advanced';
  let authScreen = $state<AuthScreen>('loading');
  let setupAt = $state<string | null>(null);
  let password = $state('');
  let passwordConfirm = $state('');
  let tokenInput = $state('');
  let signingIn = $state(false);
  let signInError = $state('');

  // First paint: if we already have a token, try a cheap call to
  // confirm it's still valid. Invalid → clear the token and drop
  // back to the login form. Valid tokens fall through to render the
  // Heute-Karte below.
  onMount(async () => {
    if ($auth) {
      try {
        await api.vault();
        return;
      } catch {
        auth.clear();
      }
    }
    await refreshAuthScreen();
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

  // AI setup hint. Shown until the user has either configured a cloud
  // provider key OR explicitly dismissed it. Detects the common
  // first-launch state where the user has all the AI features in the
  // UI (Plan my day / Reflect / Chat / Agents) but no provider that
  // can actually run them — and points at /settings instead of letting
  // those features error out cryptically.
  let appCfg = $state<import('$lib/api').AppConfig | null>(null);
  let aiHintDismissed = $state(false);
  if (typeof localStorage !== 'undefined') {
    aiHintDismissed = localStorage.getItem('granit.ai.hint.dismissed') === '1';
  }
  $effect(() => {
    if ($auth && !appCfg) {
      api.getConfig().then((c) => (appCfg = c)).catch(() => {});
    }
  });
  let aiNotConfigured = $derived.by(() => {
    if (!appCfg || aiHintDismissed) return false;
    const p = appCfg.ai_provider || 'local';
    if (p === 'openai') return !appCfg.openai_key_set;
    if (p === 'anthropic') return !appCfg.anthropic_key_set;
    // Ollama / local: we can't tell from config whether the daemon is
    // reachable, but the default model (qwen2.5:0.5b) is rarely pulled.
    // Show the hint when no cloud provider is set at all — covers the
    // most common "AI features just don't work" state.
    return !appCfg.openai_key_set && !appCfg.anthropic_key_set;
  });
  function dismissAiHint() {
    aiHintDismissed = true;
    try { localStorage.setItem('granit.ai.hint.dismissed', '1'); } catch {}
  }
</script>

{#if !$auth}
  <div class="h-full overflow-y-auto flex items-center justify-center p-4 sm:p-8">
    <div class="w-full max-w-sm bg-mantle border border-surface1 rounded-lg p-5 sm:p-6 space-y-4">
      <div>
        <h1 class="text-lg font-semibold text-text">everything</h1>
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
{:else}
  <div class="h-full overflow-y-auto">
    <div class="p-3 sm:p-4 lg:p-6 max-w-3xl mx-auto">
      {#if aiNotConfigured}
        <div class="mb-4 p-4 bg-surface0 border border-warning rounded-lg flex items-start gap-3">
          <div class="w-8 h-8 rounded-full bg-surface0 flex items-center justify-center flex-shrink-0">
            <svg viewBox="0 0 24 24" class="w-4 h-4 text-warning" fill="none" stroke="currentColor" stroke-width="2">
              <path d="M12 9v4M12 17h.01" stroke-linecap="round"/>
              <circle cx="12" cy="12" r="9"/>
            </svg>
          </div>
          <div class="flex-1 min-w-0">
            <p class="text-sm text-text font-medium">AI provider not configured</p>
            <p class="text-xs text-dim mt-0.5">
              Plan my day, Chat, Reflect, deep research, morning AI suggestion — none of these will work until you set an API key.
            </p>
            <div class="flex items-center gap-3 mt-2">
              <a href="/settings" class="px-3 py-1.5 text-xs bg-warning text-mantle rounded font-medium">Open Settings</a>
              <button onclick={dismissAiHint} class="text-xs text-dim hover:text-text">dismiss</button>
            </div>
          </div>
        </div>
      {/if}

      <Heute />
    </div>
  </div>
{/if}
