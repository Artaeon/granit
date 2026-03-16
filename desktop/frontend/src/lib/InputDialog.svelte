<script lang="ts">
  import { createEventDispatcher, onMount } from 'svelte'

  export let title = 'Input'
  export let placeholder = ''
  export let value = ''
  export let action = 'Confirm'
  export let destructive = false

  const dispatch = createEventDispatcher()
  let inputEl: HTMLInputElement

  onMount(() => { if (inputEl) { inputEl.focus(); inputEl.select() } })
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="fixed inset-0 z-[60] flex items-center justify-center" style="background:rgba(0,0,0,0.6);backdrop-filter:blur(4px)"
  on:click|self={() => dispatch('cancel')}>
  <div class="w-full max-w-sm bg-ctp-mantle rounded-xl border border-ctp-surface0 shadow-overlay p-5"
    style="animation: modalSlideIn 200ms cubic-bezier(0.16, 1, 0.3, 1)">
    <h3 class="text-[14px] font-semibold text-ctp-text mb-4">{title}</h3>
    <input bind:this={inputEl} bind:value type="text" {placeholder}
      on:keydown={(e) => { if (e.key === 'Enter' && value.trim()) dispatch('confirm', value.trim()); if (e.key === 'Escape') dispatch('cancel') }}
      class="w-full px-3 py-2.5 text-[13px] bg-ctp-surface0 text-ctp-text rounded-lg border border-ctp-surface1
             outline-none placeholder:text-ctp-overlay0 transition-colors" />
    <div class="flex justify-end gap-2 mt-4">
      <button on:click={() => dispatch('cancel')}
        class="px-4 py-2 text-[12px] font-medium text-ctp-subtext0 bg-ctp-surface0 rounded-lg
               hover:bg-ctp-surface1 transition-colors">
        Cancel
      </button>
      <button on:click={() => { if (value.trim()) dispatch('confirm', value.trim()) }}
        class="px-4 py-2 text-[12px] font-medium rounded-lg transition-colors"
        class:bg-ctp-blue={!destructive}
        class:bg-ctp-red={destructive}
        class:text-ctp-crust={true}
        class:hover:opacity-90={true}>
        {action}
      </button>
    </div>
  </div>
</div>
