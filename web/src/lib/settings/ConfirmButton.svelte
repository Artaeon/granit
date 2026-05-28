<script lang="ts">
  // ConfirmButton — inline two-step confirm. First click reveals
  // "Confirm? · cancel" inline; second click executes. Replaces
  // native window.confirm() which is jarring inside the settings
  // surface. Auto-cancels after 5s if the user walks away.
  let {
    label,
    confirmLabel = 'Confirm?',
    danger = false,
    disabled = false,
    title,
    onconfirm
  }: {
    label: string;
    confirmLabel?: string;
    danger?: boolean;
    disabled?: boolean;
    title?: string;
    onconfirm: () => void | Promise<void>;
  } = $props();

  let armed = $state(false);
  let timer: ReturnType<typeof setTimeout> | null = null;

  function arm() {
    armed = true;
    if (timer) clearTimeout(timer);
    timer = setTimeout(() => { armed = false; timer = null; }, 5000);
  }
  function fire() {
    if (timer) { clearTimeout(timer); timer = null; }
    armed = false;
    void onconfirm();
  }
  function cancel() {
    if (timer) { clearTimeout(timer); timer = null; }
    armed = false;
  }

  const base = 'px-2.5 py-1 text-xs rounded border transition-colors disabled:opacity-50';
  const dangerCls = $derived(danger
    ? 'border-error text-error hover:bg-error/10'
    : 'border-surface1 text-subtext hover:border-primary');
</script>

{#if armed}
  <span class="inline-flex items-center gap-1">
    <button
      type="button"
      onclick={fire}
      {disabled}
      class="{base} bg-error text-on-primary border-error hover:opacity-90"
    >{confirmLabel}</button>
    <button
      type="button"
      onclick={cancel}
      class="text-[11px] text-dim hover:text-text px-1.5 py-1"
    >cancel</button>
  </span>
{:else}
  <button
    type="button"
    onclick={arm}
    {disabled}
    {title}
    class="{base} {dangerCls}"
  >{label}</button>
{/if}
