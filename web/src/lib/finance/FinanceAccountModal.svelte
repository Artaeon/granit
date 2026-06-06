<!--
  New-account modal. Mirrors the inline form the page used to host;
  swatch row iterates the shared ACCOUNT_COLORS palette.
-->
<script lang="ts">
  import EditModal from '$lib/components/EditModal.svelte';
  import { ACCOUNT_COLORS, accColor } from '$lib/finance/financeFmt';
  import type { FinanceAccountFormController } from '$lib/finance/financeAccountForm.svelte';

  type Props = { accountForm: FinanceAccountFormController };
  let { accountForm }: Props = $props();
</script>

<EditModal
  open={accountForm.open}
  maxWidth="sm"
  title="New account"
  onClose={() => accountForm.close()}
>
  <form onsubmit={(e) => { e.preventDefault(); accountForm.submit(); }} class="p-4 space-y-3">
      <input bind:value={accountForm.form.name} required placeholder="Name" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      <select bind:value={accountForm.form.kind} class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
        <option value="checking">Checking</option>
        <option value="savings">Savings</option>
        <option value="cash">Cash</option>
        <option value="credit">Credit card</option>
        <option value="investment">Investment</option>
        <option value="loan">Loan</option>
      </select>
      <div class="flex gap-2">
        <input bind:value={accountForm.form.currency} placeholder="USD" class="w-20 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
        <input type="number" step="0.01" bind:value={accountForm.form.balance} placeholder="0.00" class="flex-1 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
      </div>
      <input bind:value={accountForm.form.institution} placeholder="Institution (Chase, Apple Card…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <!-- Color palette swatches — visually pick the row pip rather
           than typing a name. Empty pip = "no color". -->
      <div class="flex items-center gap-2">
        <span class="text-[11px] text-dim">Color</span>
        <button type="button" onclick={() => (accountForm.form.color = '')} class="w-5 h-5 rounded-full border border-surface2 {accountForm.form.color === '' ? 'ring-2 ring-primary' : ''}" aria-label="no color"></button>
        {#each ACCOUNT_COLORS as c}
          <button type="button" onclick={() => (accountForm.form.color = c)} class="w-5 h-5 rounded-full {accountForm.form.color === c ? 'ring-2 ring-primary' : ''}" style="background: {accColor(c)}" aria-label={c}></button>
        {/each}
      </div>
    <input bind:value={accountForm.form.tags} placeholder="Tags (comma-separated)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
    <input bind:value={accountForm.form.notes} placeholder="Notes (optional)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
    <div class="flex justify-end gap-2 pt-2">
      <button type="button" onclick={() => accountForm.close()} class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
      <button type="submit" class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90">Create</button>
    </div>
  </form>
</EditModal>
