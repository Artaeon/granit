<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { decryptNote, encryptNote, isNoteEncrypted, saveDecryptedNote } from './api'

  export let notePath: string = ''

  const dispatch = createEventDispatcher()
  let isEncrypted = false
  let password = ''
  let showPassword = false
  let loading = false
  let message = ''
  let messageType: 'success' | 'error' | '' = ''
  let decryptedContent = ''
  let showDecrypted = false

  async function checkStatus() {
    if (!notePath) return
    try {
      isEncrypted = await isNoteEncrypted(notePath)
    } catch {
      isEncrypted = false
    }
    password = ''
    message = ''
    messageType = ''
    decryptedContent = ''
    showDecrypted = false
  }

  $: if (notePath) checkStatus()

  async function encrypt() {
    if (!notePath || !password) {
      message = 'Please enter a password'
      messageType = 'error'
      return
    }
    loading = true
    message = ''
    try {
      await encryptNote(notePath, password)
      isEncrypted = true
      message = 'Note encrypted successfully'
      messageType = 'success'
      password = ''
      dispatch('refresh')
    } catch (e: any) {
      message = e?.message || 'Encryption failed'
      messageType = 'error'
    }
    loading = false
  }

  async function decrypt() {
    if (!notePath || !password) {
      message = 'Please enter a password'
      messageType = 'error'
      return
    }
    loading = true
    message = ''
    try {
      decryptedContent = await decryptNote(notePath, password)
      showDecrypted = true
      message = 'Decrypted successfully (preview only)'
      messageType = 'success'
    } catch (e: any) {
      message = e?.message || 'Decryption failed'
      messageType = 'error'
      decryptedContent = ''
      showDecrypted = false
    }
    loading = false
  }

  async function decryptAndSave() {
    if (!notePath || !password) return
    loading = true
    try {
      await saveDecryptedNote(notePath, password)
      isEncrypted = false
      showDecrypted = false
      decryptedContent = ''
      message = 'Note decrypted and saved'
      messageType = 'success'
      password = ''
      dispatch('refresh')
    } catch (e: any) {
      message = e?.message || 'Failed to save decrypted note'
      messageType = 'error'
    }
    loading = false
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[10%]"
  style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)"
  on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-md h-fit bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        {#if isEncrypted}
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-green)" stroke-width="1.5" stroke-linecap="round">
            <rect x="3" y="7" width="10" height="7" rx="1" /><path d="M5 7V5a3 3 0 0 1 6 0v2" />
          </svg>
        {:else}
          <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
            <rect x="3" y="7" width="10" height="7" rx="1" /><path d="M5 7V5a3 3 0 0 1 6 0" />
          </svg>
        {/if}
        <span class="text-sm font-semibold text-ctp-text">Encryption</span>
        {#if isEncrypted}
          <span class="text-[12px] font-medium bg-ctp-green/20 text-ctp-green px-1.5 py-0.5 rounded">Encrypted</span>
        {:else}
          <span class="text-[12px] font-medium bg-ctp-surface0 text-ctp-overlay1 px-1.5 py-0.5 rounded">Decrypted</span>
        {/if}
      </div>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
        on:click={() => dispatch('close')}>esc</kbd>
    </div>

    <!-- Status message -->
    {#if message}
      <div class="flex items-center gap-2 px-4 py-2 text-[13px] border-b border-ctp-surface0"
        style="background: color-mix(in srgb, {messageType === 'error' ? 'var(--ctp-red)' : 'var(--ctp-green)'} 8%, transparent);
               color: {messageType === 'error' ? 'var(--ctp-red)' : 'var(--ctp-green)'}">
        <svg width="12" height="12" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round">
          {#if messageType === 'error'}<circle cx="8" cy="8" r="6" /><path d="M8 5v3m0 2.5v.5" />{:else}<path d="M3 8l3 3 7-7" />{/if}
        </svg>
        {message}
      </div>
    {/if}

    <div class="px-4 py-4 space-y-4">
      <!-- Lock/unlock icon state -->
      <div class="flex justify-center py-2">
        {#if isEncrypted}
          <div class="w-16 h-16 rounded-full bg-ctp-green/10 flex items-center justify-center">
            <svg width="32" height="32" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-green)" stroke-width="1.2" stroke-linecap="round">
              <rect x="3" y="7" width="10" height="7" rx="1" /><path d="M5 7V5a3 3 0 0 1 6 0v2" />
              <circle cx="8" cy="10.5" r="1" fill="var(--ctp-green)" />
            </svg>
          </div>
        {:else}
          <div class="w-16 h-16 rounded-full bg-ctp-surface0 flex items-center justify-center">
            <svg width="32" height="32" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay1)" stroke-width="1.2" stroke-linecap="round">
              <rect x="3" y="7" width="10" height="7" rx="1" /><path d="M5 7V5a3 3 0 0 1 6 0" />
            </svg>
          </div>
        {/if}
      </div>

      <!-- Password input -->
      <div>
        <div class="text-[12px] text-ctp-overlay1 mb-1.5">Password</div>
        <div class="relative">
          {#if showPassword}
            <input type="text" bind:value={password}
              placeholder={isEncrypted ? 'Enter password to decrypt' : 'Enter password to encrypt'}
              on:keydown={(e) => { if (e.key === 'Enter') { isEncrypted ? decrypt() : encrypt() } }}
              class="w-full px-3 py-2 pr-10 text-sm bg-ctp-surface0 text-ctp-text rounded-lg border border-ctp-surface1 outline-none focus:border-ctp-mauve transition-colors" />
          {:else}
            <input type="password" bind:value={password}
              placeholder={isEncrypted ? 'Enter password to decrypt' : 'Enter password to encrypt'}
              on:keydown={(e) => { if (e.key === 'Enter') { isEncrypted ? decrypt() : encrypt() } }}
              class="w-full px-3 py-2 pr-10 text-sm bg-ctp-surface0 text-ctp-text rounded-lg border border-ctp-surface1 outline-none focus:border-ctp-mauve transition-colors" />
          {/if}
          <button on:click={() => showPassword = !showPassword}
            class="absolute right-2 top-1/2 -translate-y-1/2 text-ctp-overlay1 hover:text-ctp-text transition-colors">
            {#if showPassword}
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M2 8s2.5-4 6-4 6 4 6 4-2.5 4-6 4-6-4-6-4" /><circle cx="8" cy="8" r="2" />
              </svg>
            {:else}
              <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round">
                <path d="M2 2l12 12M4.5 6.5C3.3 7.3 2 8 2 8s2.5 4 6 4c.8 0 1.5-.2 2.2-.4M14 8s-2.5-4-6-4c-.4 0-.8 0-1.2.1" />
              </svg>
            {/if}
          </button>
        </div>
      </div>

      <!-- Warning -->
      {#if !isEncrypted}
        <div class="flex gap-2 bg-ctp-yellow/10 rounded-lg px-3 py-2">
          <svg width="14" height="14" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-yellow)" stroke-width="1.5" stroke-linecap="round" class="flex-shrink-0 mt-0.5">
            <path d="M8 2L1 14h14L8 2zM8 6v4m0 2v.5" />
          </svg>
          <span class="text-[12px] text-ctp-yellow/80">
            If you lose your password, the note cannot be recovered. Make sure to remember it or store it securely.
          </span>
        </div>
      {/if}

      <!-- Decrypted preview -->
      {#if showDecrypted && decryptedContent}
        <div>
          <div class="text-[12px] text-ctp-overlay1 mb-1.5">Preview (not saved)</div>
          <div class="bg-ctp-surface0 rounded-lg px-3 py-2 max-h-32 overflow-y-auto">
            <pre class="text-[13px] text-ctp-text whitespace-pre-wrap font-mono">{decryptedContent.slice(0, 500)}{decryptedContent.length > 500 ? '...' : ''}</pre>
          </div>
        </div>
      {/if}

      <!-- Action buttons -->
      <div class="flex gap-2">
        {#if isEncrypted}
          <button on:click={decrypt} disabled={loading || !password}
            class="flex-1 text-[13px] font-semibold bg-ctp-blue text-ctp-crust px-4 py-2.5 rounded-lg hover:opacity-90 transition-opacity disabled:opacity-40">
            {loading ? 'Decrypting...' : 'Decrypt (Preview)'}
          </button>
          {#if showDecrypted}
            <button on:click={decryptAndSave} disabled={loading}
              class="flex-1 text-[13px] font-semibold bg-ctp-green text-ctp-crust px-4 py-2.5 rounded-lg hover:opacity-90 transition-opacity disabled:opacity-40">
              Save Decrypted
            </button>
          {/if}
        {:else}
          <button on:click={encrypt} disabled={loading || !password}
            class="flex-1 text-[13px] font-semibold bg-ctp-mauve text-ctp-crust px-4 py-2.5 rounded-lg hover:opacity-90 transition-opacity disabled:opacity-40">
            {loading ? 'Encrypting...' : 'Encrypt Note'}
          </button>
        {/if}
      </div>
    </div>
  </div>
</div>
