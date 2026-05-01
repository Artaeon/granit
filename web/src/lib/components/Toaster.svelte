<script lang="ts">
  import { fly, fade } from 'svelte/transition';
  import { toasts, dismiss, type ToastKind } from './toast';

  function classFor(kind: ToastKind): string {
    switch (kind) {
      case 'success':
        return 'bg-success/15 border-success/30 text-success';
      case 'warning':
        return 'bg-warning/15 border-warning/30 text-warning';
      case 'error':
        return 'bg-error/15 border-error/30 text-error';
      default:
        return 'bg-info/15 border-info/30 text-info';
    }
  }
  function iconFor(kind: ToastKind): string {
    switch (kind) {
      case 'success': return '✓';
      case 'warning': return '!';
      case 'error': return '×';
      default: return 'i';
    }
  }
</script>

<div
  class="fixed top-3 right-3 z-[70] flex flex-col gap-2 pointer-events-none max-w-sm w-[calc(100vw-1.5rem)] sm:w-96"
  aria-live="polite"
  aria-atomic="true"
>
  {#each $toasts as t (t.id)}
    <div
      role="status"
      in:fly={{ x: 32, duration: 180 }}
      out:fade={{ duration: 150 }}
      class="pointer-events-auto flex items-start gap-2 px-3 py-2 rounded-lg border shadow-lg backdrop-blur {classFor(t.kind)}"
    >
      <span class="text-sm font-bold w-4 text-center flex-shrink-0">{iconFor(t.kind)}</span>
      <span class="flex-1 text-sm break-words">{t.message}</span>
      <button
        onclick={() => dismiss(t.id)}
        aria-label="dismiss"
        class="text-lg leading-none opacity-60 hover:opacity-100 -mt-0.5 px-1"
      >
        ×
      </button>
    </div>
  {/each}
</div>
