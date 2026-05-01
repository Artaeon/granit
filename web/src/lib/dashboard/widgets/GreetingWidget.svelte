<script lang="ts">
  let { vaultPath = '' }: { vaultPath?: string } = $props();
  const now = new Date();
  const dateLong = now.toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric', year: 'numeric' });
  const hour = now.getHours();
  const greet = hour < 5 ? 'Working late' : hour < 12 ? 'Good morning' : hour < 18 ? 'Good afternoon' : 'Good evening';
</script>

<div>
  <h1 class="text-2xl sm:text-3xl font-semibold text-text">{greet}.</h1>
  <p class="text-sm text-dim mt-1">{dateLong}{vaultPath ? ` · ${vaultPath}` : ''}</p>
</div>
