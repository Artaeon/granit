<script lang="ts">
  // Single source of truth for nav icons. Tabler-style stroke icons —
  // small, clean, render reliably in any font / OS combination.
  let { name, class: cls = 'w-5 h-5' }: { name: string; class?: string } = $props();
</script>

{#if name === 'today'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <circle cx="12" cy="12" r="9"/><path d="M12 3a9 9 0 0 1 0 18z" fill="currentColor" stroke="none"/>
  </svg>
{:else if name === 'morning'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <circle cx="12" cy="12" r="4"/><path d="M12 2v2M12 20v2M4.93 4.93l1.41 1.41M17.66 17.66l1.41 1.41M2 12h2M20 12h2M4.93 19.07l1.41-1.41M17.66 6.34l1.41-1.41"/>
  </svg>
{:else if name === 'tasks'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <rect x="3" y="5" width="6" height="6" rx="1"/><path d="m4.5 8 1 1 2-2"/><path d="M13 7h8M13 17h8M3 17h6"/>
  </svg>
{:else if name === 'calendar'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <rect x="3" y="4" width="18" height="18" rx="2"/><path d="M16 2v4M8 2v4M3 10h18"/>
  </svg>
{:else if name === 'habits'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <path d="M12 21s-7-4.5-7-10a4 4 0 0 1 7-2.6A4 4 0 0 1 19 11c0 5.5-7 10-7 10z"/>
  </svg>
{:else if name === 'goals'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <circle cx="12" cy="12" r="9"/><circle cx="12" cy="12" r="5"/><circle cx="12" cy="12" r="1.5" fill="currentColor"/>
  </svg>
{:else if name === 'agents'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <rect x="4" y="6" width="16" height="12" rx="3"/><circle cx="9" cy="12" r="1.2" fill="currentColor"/><circle cx="15" cy="12" r="1.2" fill="currentColor"/><path d="M12 6V3M9 21h6"/>
  </svg>
{:else if name === 'projects'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <path d="M3 7.5V19a2 2 0 0 0 2 2h14a2 2 0 0 0 2-2V8a2 2 0 0 0-2-2h-7l-2-2H5a2 2 0 0 0-2 2v1.5z"/><path d="M3 11h18"/>
  </svg>
{:else if name === 'objects'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <path d="M12 3 4 8v8l8 5 8-5V8z"/><path d="M4 8l8 5 8-5M12 13v8"/>
  </svg>
{:else if name === 'tags'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <path d="M20.6 13.4 13.4 20.6a2 2 0 0 1-2.8 0l-7.2-7.2a2 2 0 0 1-.6-1.4V5a2 2 0 0 1 2-2h6.9a2 2 0 0 1 1.4.6l7.5 7.5a2 2 0 0 1 0 2.8z"/><circle cx="8" cy="8" r="1.5" fill="currentColor"/>
  </svg>
{:else if name === 'templates'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <rect x="3" y="3" width="7" height="7" rx="1"/><rect x="14" y="3" width="7" height="7" rx="1"/><rect x="3" y="14" width="7" height="7" rx="1"/><path d="M14 17.5h7M14 14v7" stroke-dasharray="2 2"/>
  </svg>
{:else if name === 'stats'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <path d="M3 3v18h18"/><rect x="7" y="13" width="3" height="6" fill="currentColor" stroke="none"/><rect x="12" y="9" width="3" height="10" fill="currentColor" stroke="none"/><rect x="17" y="5" width="3" height="14" fill="currentColor" stroke="none"/>
  </svg>
{:else if name === 'notes'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <path d="M14 3v4a1 1 0 0 0 1 1h4"/><path d="M17 21H7a2 2 0 0 1-2-2V5a2 2 0 0 1 2-2h7l5 5v11a2 2 0 0 1-2 2z"/><path d="M9 13h6M9 17h4"/>
  </svg>
{:else if name === 'settings'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <path d="M10.3 3.3a2 2 0 0 1 3.4 0l.4.7a2 2 0 0 0 1.7 1l.8-.1a2 2 0 0 1 2.4 2.4l-.1.8a2 2 0 0 0 1 1.7l.7.4a2 2 0 0 1 0 3.4l-.7.4a2 2 0 0 0-1 1.7l.1.8a2 2 0 0 1-2.4 2.4l-.8-.1a2 2 0 0 0-1.7 1l-.4.7a2 2 0 0 1-3.4 0l-.4-.7a2 2 0 0 0-1.7-1l-.8.1a2 2 0 0 1-2.4-2.4l.1-.8a2 2 0 0 0-1-1.7l-.7-.4a2 2 0 0 1 0-3.4l.7-.4a2 2 0 0 0 1-1.7l-.1-.8a2 2 0 0 1 2.4-2.4l.8.1a2 2 0 0 0 1.7-1z"/><circle cx="12" cy="12" r="3"/>
  </svg>
{:else if name === 'more'}
  <svg viewBox="0 0 24 24" fill="currentColor" class={cls}>
    <circle cx="5" cy="12" r="1.5"/><circle cx="12" cy="12" r="1.5"/><circle cx="19" cy="12" r="1.5"/>
  </svg>
{:else if name === 'search'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" class={cls}>
    <circle cx="11" cy="11" r="8"/><path d="m21 21-4.3-4.3"/>
  </svg>
{:else if name === 'cmd'}
  <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" class={cls}>
    <path d="M9 6a3 3 0 1 0-3 3h12a3 3 0 1 0-3-3v12a3 3 0 1 0 3-3H6a3 3 0 1 0 3 3z"/>
  </svg>
{/if}
