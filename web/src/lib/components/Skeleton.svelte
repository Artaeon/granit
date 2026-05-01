<script lang="ts">
  // A subtle shimmer block. Pass `class` to size it.
  let { class: className = 'h-4 w-full' }: { class?: string } = $props();
</script>

<div class="skeleton {className}"></div>

<style>
  .skeleton {
    background: linear-gradient(
      90deg,
      var(--color-surface1) 0%,
      var(--color-surface2) 50%,
      var(--color-surface1) 100%
    );
    background-size: 200% 100%;
    animation: shimmer 1.4s ease-in-out infinite;
    border-radius: 4px;
  }
  @keyframes shimmer {
    0% { background-position: 200% 0; }
    100% { background-position: -200% 0; }
  }
  @media (prefers-reduced-motion: reduce) {
    .skeleton {
      animation: none;
      background: var(--color-surface1);
    }
  }
</style>
