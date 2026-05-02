<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type RecurringTask } from '$lib/api';
  import { toast } from '$lib/components/toast';

  // Embedded editor for the recurring-task list. Drops into the
  // settings page; would also fit a dedicated /recurring route if we
  // ever want one.
  //
  // Trade-off: full-list put-on-save (matches the server's PUT
  // semantics) means we never have to sync individual rule IDs across
  // surfaces. Cost: every change ships the whole array. Fine — the
  // list is tiny.

  let rules = $state<RecurringTask[]>([]);
  let loading = $state(true);
  let busy = $state(false);

  async function load() {
    loading = true;
    try {
      const r = await api.listRecurring();
      rules = r.rules ?? [];
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }
  onMount(load);

  async function save() {
    busy = true;
    try {
      const r = await api.putRecurring(rules);
      rules = r.rules ?? [];
      toast.success('recurring rules saved');
    } catch (e) {
      toast.error('save failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      busy = false;
    }
  }

  function addRule() {
    rules = [...rules, { text: '', frequency: 'daily', day_of_week: 1, day_of_month: 1, enabled: true }];
  }

  function removeRule(idx: number) {
    rules = rules.filter((_, i) => i !== idx);
  }

  // Day-of-week labels match Go's time.Weekday: 0=Sun.
  const weekdayLabels = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
</script>

<div class="space-y-2">
  {#if loading}
    <div class="text-sm text-dim">loading…</div>
  {:else if rules.length === 0}
    <p class="text-sm text-dim italic">No recurring rules yet. Add one to auto-create tasks daily, weekly, or monthly.</p>
  {:else}
    <ul class="space-y-2">
      {#each rules as rule, i}
        <li class="flex flex-wrap items-center gap-2 p-2 bg-mantle border border-surface1 rounded">
          <input
            type="checkbox"
            bind:checked={rule.enabled}
            class="w-4 h-4 accent-primary cursor-pointer flex-shrink-0"
            title="enable / disable"
          />
          <input
            bind:value={rule.text}
            placeholder="Workout"
            class="flex-1 min-w-[12rem] px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
          />
          <select
            bind:value={rule.frequency}
            class="px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
          >
            <option value="daily">daily</option>
            <option value="weekly">weekly</option>
            <option value="monthly">monthly</option>
          </select>
          {#if rule.frequency === 'weekly'}
            <select
              bind:value={rule.day_of_week}
              class="px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
            >
              {#each weekdayLabels as label, w}
                <option value={w}>{label}</option>
              {/each}
            </select>
          {:else if rule.frequency === 'monthly'}
            <input
              type="number"
              min="1"
              max="31"
              bind:value={rule.day_of_month}
              class="w-20 px-2 py-1 bg-surface0 border border-surface1 rounded text-sm text-text"
              title="day of month"
            />
          {/if}
          <button
            onclick={() => removeRule(i)}
            class="text-error hover:bg-error/10 px-2 py-1 rounded text-xs"
            aria-label="remove"
          >×</button>
        </li>
      {/each}
    </ul>
  {/if}

  <div class="flex gap-2">
    <button onclick={addRule} class="px-3 py-1.5 text-xs bg-surface0 border border-surface1 rounded hover:border-primary text-text">+ add rule</button>
    <span class="flex-1"></span>
    <button onclick={save} disabled={busy} class="px-3 py-1.5 text-xs bg-primary text-mantle rounded font-medium disabled:opacity-50">{busy ? 'saving…' : 'save'}</button>
  </div>
  <p class="text-[11px] text-dim italic">
    Stored in <code>.granit/recurring.json</code> (same file the TUI's recurring overlay edits).
    Tasks are auto-created at midnight + on first request after midnight, with origin <code>recurring</code> on the sidecar.
  </p>
</div>
