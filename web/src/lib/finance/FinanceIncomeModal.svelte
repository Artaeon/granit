<!--
  New / edit income-stream modal — UX is the same form for both
  flows; the create / edit branch lives on incomeForm.editingId.
  Project + account pickers come from dataCtl (the page passes
  both arrays in so this component stays prop-only and doesn't
  reach into the controller for non-form data).
-->
<script lang="ts">
  import EditModal from '$lib/components/EditModal.svelte';
  import type { FinAccount, Project } from '$lib/api';
  import type { FinanceIncomeFormController } from '$lib/finance/financeIncomeForm.svelte';

  type Props = {
    incomeForm: FinanceIncomeFormController;
    accounts: FinAccount[];
    projects: Project[];
  };
  let { incomeForm, accounts, projects }: Props = $props();
</script>

<EditModal
  open={incomeForm.open}
  title={incomeForm.editingId ? 'Edit income source' : 'New income source'}
  onClose={() => incomeForm.close()}
>
  <form onsubmit={(e) => { e.preventDefault(); incomeForm.submit(); }} class="p-4 space-y-3">
      <input bind:value={incomeForm.form.name} required placeholder="Name (Day job, Side SaaS, Dividends…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      <div class="grid grid-cols-2 gap-2">
        <label class="block">
          <span class="text-[11px] text-dim">Status</span>
          <select bind:value={incomeForm.form.status} class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
            <option value="idea">Idea (could bring money)</option>
            <option value="planned">Planned (working on it)</option>
            <option value="active">Active (bringing money now)</option>
            <option value="paused">Paused</option>
          </select>
        </label>
        <label class="block">
          <span class="text-[11px] text-dim">Type</span>
          <select bind:value={incomeForm.form.kind} class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
            <option value="employment">Employment / salary</option>
            <option value="freelance">Freelance / contract</option>
            <option value="business">Business / SaaS</option>
            <option value="investment">Investment / dividends</option>
            <option value="royalty">Royalty</option>
            <option value="other">Other</option>
          </select>
        </label>
      </div>
      <div class="grid grid-cols-3 gap-2 items-end">
        <label class="block col-span-1">
          <span class="text-[11px] text-dim">Projected / mo</span>
          <input type="number" step="0.01" bind:value={incomeForm.form.projected} placeholder="0.00" class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
        </label>
        <label class="block col-span-1">
          <span class="text-[11px] text-dim">Actual / mo</span>
          <input type="number" step="0.01" bind:value={incomeForm.form.actual} placeholder="0.00" class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
        </label>
        <input bind:value={incomeForm.form.currency} class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      </div>

      <!-- Payout schedule. Day-of-month + cadence drives the
           cashflow timeline projection. Empty day = unknown
           schedule; the stream still shows everywhere else but
           doesn't render on the date strip. -->
      <fieldset class="border border-surface1 rounded p-3 space-y-2">
        <legend class="text-[11px] text-dim px-1">Payout schedule</legend>
        <div class="grid grid-cols-2 gap-2">
          <label class="block">
            <span class="text-[11px] text-dim">Day of month (1-31)</span>
            <input type="number" min="0" max="31" bind:value={incomeForm.form.payout_day} placeholder="e.g. 5" class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text font-mono text-right focus:outline-none focus:border-primary" />
          </label>
          <label class="block">
            <span class="text-[11px] text-dim">Cadence</span>
            <select bind:value={incomeForm.form.payout_cadence} class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary">
              <option value="monthly">Monthly</option>
              <option value="yearly">Yearly (anchor month from started date)</option>
              <option value="quarterly">Quarterly (approx)</option>
              <option value="weekly">Weekly (approx)</option>
            </select>
          </label>
        </div>
        <p class="text-[11px] text-dim">
          Salary on the 5th? Set day=5, cadence=monthly. Leave day blank if you don't want it on the timeline.
        </p>
      </fieldset>

      <!-- Project + account links. Both optional — useful for
           ventures (link to the project that's the venture) and
           dividend streams (link to the investment account). -->
      <div class="grid grid-cols-2 gap-2">
        <label class="block">
          <span class="text-[11px] text-dim">Lands in account</span>
          <select bind:value={incomeForm.form.account_id} class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary">
            <option value="">— none —</option>
            {#each accounts as a}<option value={a.id}>{a.name}</option>{/each}
          </select>
        </label>
        <label class="block">
          <span class="text-[11px] text-dim">Linked project</span>
          <select bind:value={incomeForm.form.project} class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary">
            <option value="">— none —</option>
            {#each projects as p}<option value={p.name}>{p.name}</option>{/each}
          </select>
        </label>
      </div>
      <input bind:value={incomeForm.form.tags} placeholder="Tags (comma-separated, e.g. primary, w2)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <input bind:value={incomeForm.form.url} placeholder="URL (optional)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <textarea bind:value={incomeForm.form.notes} rows="2" placeholder="Notes (idea details, next steps…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text resize-y focus:outline-none focus:border-primary"></textarea>
    <div class="flex justify-end gap-2 pt-2">
      <button type="button" onclick={() => incomeForm.close()} class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
      <button type="submit" class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90">{incomeForm.editingId ? 'Save' : 'Add'}</button>
    </div>
  </form>
</EditModal>
