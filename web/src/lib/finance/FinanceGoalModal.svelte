<!--
  New financial-goal modal. Smallest of the four — three named
  kinds (savings / payoff / networth), required name + target,
  optional date.
-->
<script lang="ts">
  import EditModal from '$lib/components/EditModal.svelte';
  import type { FinanceGoalFormController } from '$lib/finance/financeGoalForm.svelte';

  type Props = { goalForm: FinanceGoalFormController };
  let { goalForm }: Props = $props();
</script>

<EditModal
  open={goalForm.open}
  maxWidth="sm"
  title="New financial goal"
  onClose={() => goalForm.close()}
>
  <form onsubmit={(e) => { e.preventDefault(); goalForm.submit(); }} class="p-4 space-y-3">
      <input bind:value={goalForm.form.name} required placeholder="Name (Emergency fund, Pay off card…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      <select bind:value={goalForm.form.kind} class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
        <option value="savings">Savings (build up to target)</option>
        <option value="payoff">Payoff (shrink debt to zero)</option>
        <option value="networth">Net worth (aggregate target)</option>
      </select>
      <div class="flex gap-2">
        <input type="number" step="0.01" bind:value={goalForm.form.target} required placeholder="Target" class="flex-1 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
        <input type="number" step="0.01" bind:value={goalForm.form.current} placeholder="Current" class="flex-1 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
        <input bind:value={goalForm.form.currency} class="w-20 bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      </div>
      <label class="block text-xs text-dim">Target date (optional)
        <input type="date" bind:value={goalForm.form.target_date} class="block mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      </label>
    <div class="flex justify-end gap-2 pt-2">
      <button type="button" onclick={() => goalForm.close()} class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
      <button type="submit" class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90">Add</button>
    </div>
  </form>
</EditModal>
