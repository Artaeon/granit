<!--
  New-subscription modal. Account picker is hidden when there are
  no accounts (mirrors the original page behaviour); project
  picker stays visible so users can scope a sub to a project even
  without an account.
-->
<script lang="ts">
  import EditModal from '$lib/components/EditModal.svelte';
  import type { FinAccount, Project } from '$lib/api';
  import type { FinanceSubscriptionFormController } from '$lib/finance/financeSubscriptionForm.svelte';

  type Props = {
    subscriptionForm: FinanceSubscriptionFormController;
    accounts: FinAccount[];
    projects: Project[];
  };
  let { subscriptionForm, accounts, projects }: Props = $props();
</script>

<EditModal
  open={subscriptionForm.open}
  maxWidth="sm"
  title="New subscription"
  onClose={() => subscriptionForm.close()}
>
  <form onsubmit={(e) => { e.preventDefault(); subscriptionForm.submit(); }} class="p-4 space-y-3">
      <input bind:value={subscriptionForm.form.name} required placeholder="Name (Netflix, Spotify…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      <div class="flex gap-2">
        <input type="number" step="0.01" bind:value={subscriptionForm.form.amount} required placeholder="9.99" class="flex-1 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
        <input bind:value={subscriptionForm.form.currency} class="w-20 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
        <select bind:value={subscriptionForm.form.cadence} class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
          <option value="weekly">/ week</option>
          <option value="monthly">/ month</option>
          <option value="quarterly">/ quarter</option>
          <option value="yearly">/ year</option>
        </select>
      </div>
      <label class="block text-xs text-dim">Next renewal
        <input type="date" bind:value={subscriptionForm.form.next_renewal} class="block mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      </label>
      <div class="grid grid-cols-2 gap-2">
        {#if accounts.length > 0}
          <select bind:value={subscriptionForm.form.account_id} class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary">
            <option value="">— no account —</option>
            {#each accounts as a}<option value={a.id}>{a.name}</option>{/each}
          </select>
        {/if}
        <select bind:value={subscriptionForm.form.project} class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary">
          <option value="">— no project —</option>
          {#each projects as p}<option value={p.name}>{p.name}</option>{/each}
        </select>
      </div>
    <input bind:value={subscriptionForm.form.tags} placeholder="Tags (comma-separated)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
    <input bind:value={subscriptionForm.form.category} placeholder="Category (optional)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
    <input bind:value={subscriptionForm.form.url} placeholder="Manage URL (optional)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
    <div class="flex justify-end gap-2 pt-2">
      <button type="button" onclick={() => subscriptionForm.close()} class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
      <button type="submit" class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90">Add</button>
    </div>
  </form>
</EditModal>
