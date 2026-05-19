<script lang="ts">
  // Three-way pill picker for the day mode. The picker shows whatever
  // the user *currently* is (or "Wähle Modus" when null), and clicking
  // a pill commits. No "save" button — there's nothing else to confirm.
  //
  // The labels are deliberately not translated through pillar-config:
  // they describe a meta-state of the user's day, not a pillar.

  import type { DayMode } from './dayState';

  type Props = {
    value: DayMode | null;
    onChange: (next: DayMode) => void;
  };

  let { value, onChange }: Props = $props();

  const OPTIONS: Array<{ key: DayMode; label: string; tone: string }> = [
    { key: 'normal',    label: 'Normal',     tone: 'success' },
    { key: 'chaotic',   label: 'Chaotisch',  tone: 'warning' },
    { key: 'emergency', label: 'Notfall',    tone: 'error' }
  ];

  function activeClass(mode: DayMode, tone: string): string {
    if (value !== mode) return 'bg-surface0 border-surface1 text-subtext hover:border-primary';
    if (tone === 'success') return 'bg-success/15 border-success text-success font-medium';
    if (tone === 'warning') return 'bg-warning/15 border-warning text-warning font-medium';
    return 'bg-error/15 border-error text-error font-medium';
  }
</script>

<div class="inline-flex items-center gap-1.5 text-xs">
  <span class="text-dim uppercase tracking-wider mr-1">Modus</span>
  {#each OPTIONS as opt}
    <button
      type="button"
      onclick={() => onChange(opt.key)}
      aria-pressed={value === opt.key}
      class="px-3 py-1.5 min-h-9 rounded border transition-colors {activeClass(opt.key, opt.tone)}"
    >
      {opt.label}
    </button>
  {/each}
</div>
