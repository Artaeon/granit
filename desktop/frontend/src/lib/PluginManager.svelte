<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'
  const dispatch = createEventDispatcher()

  interface PluginInfo {
    name: string
    description: string
    version: string
    author: string
    enabled: boolean
    commands: string[]
    hooks: string[]
    path: string
  }

  let plugins: PluginInfo[] = []
  let loading = true
  let message = ''
  let messageType: 'success' | 'error' = 'success'
  let runningCommand = ''
  let selectedPlugin: PluginInfo | null = null

  const api = (window as any).go?.main?.GranitApp

  onMount(loadPlugins)

  async function loadPlugins() {
    loading = true
    try {
      const result = await api?.GetPlugins()
      plugins = result || []
    } catch (e: any) {
      showMessage('Failed to load plugins: ' + e.message, 'error')
    }
    loading = false
  }

  async function togglePlugin(name: string) {
    try {
      await api?.TogglePlugin(name)
      await loadPlugins()
      const plugin = plugins.find(p => p.name === name)
      showMessage(`${name} ${plugin?.enabled ? 'enabled' : 'disabled'}`, 'success')
    } catch (e: any) {
      showMessage('Toggle failed: ' + e.message, 'error')
    }
  }

  async function runCommand(pluginName: string, command: string) {
    runningCommand = pluginName + ':' + command
    try {
      const result = await api?.RunPluginCommand(pluginName, command)
      showMessage(result || 'Command completed', 'success')
    } catch (e: any) {
      showMessage('Command failed: ' + e.message, 'error')
    }
    runningCommand = ''
  }

  function showMessage(msg: string, type: 'success' | 'error') {
    message = msg
    messageType = type
    setTimeout(() => { message = '' }, 5000)
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return bytes + ' B'
    if (bytes < 1024 * 1024) return (bytes / 1024).toFixed(1) + ' KB'
    return (bytes / (1024 * 1024)).toFixed(1) + ' MB'
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-50 flex justify-center pt-[6%]" style="background:rgba(17,17,27,0.55);backdrop-filter:blur(8px)" on:click|self={() => dispatch('close')}>
  <div class="w-full max-w-2xl h-[80vh] bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-2xl flex flex-col overflow-hidden">
    <!-- Header -->
    <div class="flex items-center justify-between px-4 py-3 border-b border-ctp-surface0">
      <div class="flex items-center gap-2">
        <svg width="16" height="16" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-mauve)" stroke-width="1.5" stroke-linecap="round">
          <path d="M6.5 2.5L3 6l3.5 3.5M9.5 2.5L13 6 9.5 9.5M5 13h6" />
        </svg>
        {#if selectedPlugin}
          <button on:click={() => selectedPlugin = null}
            class="text-[13px] text-ctp-overlay1 hover:text-ctp-text transition-colors mr-1">
            Plugins
          </button>
          <span class="text-ctp-overlay1 text-[13px]">/</span>
          <span class="text-sm font-semibold text-ctp-text ml-1">{selectedPlugin.name}</span>
        {:else}
          <span class="text-sm font-semibold text-ctp-text">Plugins</span>
          <span class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded-full">
            {plugins.length}
          </span>
        {/if}
      </div>
      <kbd class="text-[12px] text-ctp-overlay1 bg-ctp-surface0 px-1.5 py-0.5 rounded cursor-pointer hover:bg-ctp-surface1 transition-colors"
        on:click={() => dispatch('close')}>esc</kbd>
    </div>

    <!-- Message bar -->
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

    <!-- Content -->
    <div class="flex-1 overflow-y-auto">
      {#if loading}
        <div class="flex items-center justify-center py-16">
          <span class="text-ctp-overlay1 text-sm">Loading plugins...</span>
        </div>
      {:else if selectedPlugin}
        <!-- Plugin detail view -->
        <div class="p-4 space-y-4">
          <!-- Plugin info card -->
          <div class="bg-ctp-base rounded-lg p-4 border border-ctp-surface0">
            <div class="flex items-start justify-between">
              <div>
                <h3 class="text-ctp-text font-semibold">{selectedPlugin.name}</h3>
                <p class="text-[13px] text-ctp-overlay1 mt-0.5">v{selectedPlugin.version} by {selectedPlugin.author}</p>
                <p class="text-[13px] text-ctp-subtext0 mt-2">{selectedPlugin.description}</p>
              </div>
              <button on:click={() => togglePlugin(selectedPlugin?.name || '')}
                class="shrink-0 px-3 py-1 text-[13px] font-medium rounded-md transition-colors {selectedPlugin.enabled ? 'bg-ctp-green/15 text-ctp-green hover:bg-ctp-green/25' : 'bg-ctp-surface0 text-ctp-overlay1 hover:bg-ctp-surface1'}">
                {selectedPlugin.enabled ? 'Enabled' : 'Disabled'}
              </button>
            </div>
            <div class="mt-3 text-[12px] text-ctp-overlay1 font-mono truncate">
              {selectedPlugin.path}
            </div>
          </div>

          <!-- Commands -->
          <div>
            <h4 class="text-[13px] font-semibold text-ctp-subtext0 uppercase tracking-wider px-1 mb-2">Commands</h4>
            {#if selectedPlugin.commands && selectedPlugin.commands.length > 0}
              <div class="space-y-1">
                {#each selectedPlugin.commands as cmd}
                  <div class="flex items-center justify-between px-3 py-2 bg-ctp-base rounded-lg border border-ctp-surface0 hover:border-ctp-surface1 transition-colors">
                    <span class="text-[13px] text-ctp-text">{cmd}</span>
                    <button on:click={() => runCommand(selectedPlugin?.name || '', cmd)}
                      disabled={runningCommand === (selectedPlugin?.name + ':' + cmd)}
                      class="text-[12px] font-medium px-2.5 py-1 rounded bg-ctp-blue/15 text-ctp-blue hover:bg-ctp-blue/25 transition-colors disabled:opacity-50">
                      {runningCommand === (selectedPlugin?.name + ':' + cmd) ? 'Running...' : 'Run'}
                    </button>
                  </div>
                {/each}
              </div>
            {:else}
              <p class="text-[13px] text-ctp-overlay1 px-1">No commands defined</p>
            {/if}
          </div>

          <!-- Hooks -->
          <div>
            <h4 class="text-[13px] font-semibold text-ctp-subtext0 uppercase tracking-wider px-1 mb-2">Hooks</h4>
            {#if selectedPlugin.hooks && selectedPlugin.hooks.length > 0}
              <div class="flex flex-wrap gap-1.5 px-1">
                {#each selectedPlugin.hooks as hook}
                  <span class="text-[12px] px-2 py-0.5 rounded-full bg-ctp-surface0 text-ctp-subtext0 font-mono">
                    {hook}
                  </span>
                {/each}
              </div>
            {:else}
              <p class="text-[13px] text-ctp-overlay1 px-1">No hooks registered</p>
            {/if}
          </div>
        </div>
      {:else}
        <!-- Plugin list -->
        {#if plugins.length === 0}
          <div class="flex flex-col items-center py-16 gap-3 px-8">
            <svg width="32" height="32" viewBox="0 0 16 16" fill="none" stroke="var(--ctp-overlay1)" stroke-width="1" stroke-linecap="round" class="opacity-40">
              <path d="M6.5 2.5L3 6l3.5 3.5M9.5 2.5L13 6 9.5 9.5M5 13h6" />
            </svg>
            <p class="text-ctp-overlay1 text-sm text-center">No plugins installed</p>
            <div class="text-[13px] text-ctp-overlay1 text-center space-y-1 mt-2">
              <p>Add plugins to:</p>
              <p class="font-mono text-ctp-subtext0">~/.config/granit/plugins/&lt;name&gt;/</p>
              <p class="font-mono text-ctp-subtext0">&lt;vault&gt;/.granit/plugins/&lt;name&gt;/</p>
              <p class="mt-2">Each plugin needs a <span class="font-mono text-ctp-blue">plugin.json</span> manifest.</p>
            </div>
          </div>
        {:else}
          <div class="py-1">
            {#each plugins as plugin}
              <div class="flex items-center gap-3 px-4 py-2.5 hover:bg-ctp-surface0/40 transition-colors cursor-pointer group"
                on:click={() => selectedPlugin = plugin}>
                <!-- Status dot -->
                <div class="w-2 h-2 rounded-full shrink-0 {plugin.enabled ? 'bg-ctp-green' : 'bg-ctp-surface2'}"></div>

                <!-- Info -->
                <div class="flex-1 min-w-0">
                  <div class="flex items-center gap-2">
                    <span class="text-sm font-medium text-ctp-text">{plugin.name}</span>
                    <span class="text-[12px] text-ctp-overlay1">v{plugin.version}</span>
                  </div>
                  <div class="text-[13px] text-ctp-overlay1 truncate">{plugin.description}</div>
                </div>

                <!-- Author -->
                <span class="text-[12px] text-ctp-overlay1 shrink-0 hidden sm:block">{plugin.author}</span>

                <!-- Toggle -->
                <button on:click|stopPropagation={() => togglePlugin(plugin.name)}
                  class="shrink-0 text-[12px] font-medium px-2 py-0.5 rounded transition-colors
                    {plugin.enabled ? 'text-ctp-green bg-ctp-green/10 hover:bg-ctp-green/20' : 'text-ctp-overlay1 bg-ctp-surface0 hover:bg-ctp-surface1'}">
                  {plugin.enabled ? 'ON' : 'OFF'}
                </button>
              </div>
            {/each}
          </div>
        {/if}
      {/if}
    </div>

    <!-- Footer -->
    <div class="px-4 py-2 border-t border-ctp-surface0 text-[12px] text-ctp-overlay1">
      {#if selectedPlugin}
        Click a command to run it. Toggle the status to enable or disable the plugin.
      {:else}
        Click a plugin for details. Plugins run scripts with 10s timeout.
      {/if}
    </div>
  </div>
</div>
