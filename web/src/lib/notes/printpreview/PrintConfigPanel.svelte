<script lang="ts">
  // Collapsible "Document settings" panel under the slim toolbar.
  // Hidden by default — the goal at first glance is "click Print",
  // so header/footer/signer fields are tucked away until the user
  // opts in. Bindable so the parent persists every change.
  //
  // Styling lives in PrintPreview.svelte (.config-* selectors) so
  // the @media print rule that hides the panel stays in one place.
  import type { Mode } from './strings';
  import { todayHuman } from './strings';

  let {
    mode,
    header = $bindable(''),
    footer = $bindable(''),
    signer = $bindable(''),
    purpose = $bindable(''),
    signatureOn,
    certLang,
    savingConfig,
    configDirty,
    onSave
  }: {
    mode: Mode;
    header?: string;
    footer?: string;
    signer?: string;
    purpose?: string;
    signatureOn: boolean;
    certLang: 'en' | 'de';
    savingConfig: boolean;
    configDirty: boolean;
    onSave: () => void;
  } = $props();

  let footerPlaceholder = $derived(todayHuman(certLang));
</script>

<section class="config-panel">
  <div class="config-row">
    <label for="print-header">Header</label>
    {#if mode === 'letterhead'}
      <textarea
        id="print-header"
        bind:value={header}
        placeholder={`Acme Corp\n123 Main St\nVienna, AT`}
        class="config-input config-textarea"
        rows="3"
      ></textarea>
    {:else}
      <input
        id="print-header"
        bind:value={header}
        placeholder={mode === 'memo' ? 'Your name (FROM)' : 'e.g. ACME Corp — Internal'}
        class="config-input"
      />
    {/if}
  </div>
  <div class="config-row">
    <label for="print-footer">Footer</label>
    <input
      id="print-footer"
      bind:value={footer}
      placeholder={footerPlaceholder}
      class="config-input"
    />
  </div>
  <!-- Signature claim fields. Only shown when the Sign toggle is on
       so the config panel doesn't fill with irrelevant options for
       users who never sign documents. Both are optional — a signature
       with neither still carries the SHA-256 / timestamp / source
       claim, which is the actual integrity stamp; signer / purpose
       are human-readable attestations on top. -->
  {#if signatureOn}
    <div class="config-row">
      <label for="print-signer">Signer</label>
      <input
        id="print-signer"
        bind:value={signer}
        placeholder="e.g. Jane Doe"
        class="config-input"
      />
    </div>
    <div class="config-row">
      <label for="print-purpose">Purpose</label>
      <input
        id="print-purpose"
        bind:value={purpose}
        placeholder="e.g. Q3 review · Internal use only"
        class="config-input"
      />
    </div>
  {/if}
  <div class="config-actions">
    <span class="config-hint">
      Saved on this device by default. Click <strong>Save as vault default</strong>
      to sync the values to <code>.granit/print-config.json</code> so they
      travel across browsers and devices.
    </span>
    <button
      type="button"
      onclick={onSave}
      disabled={savingConfig || !configDirty}
      class="config-save"
      title={configDirty ? 'Save header/footer/mode to the vault' : 'No changes to save'}
    >{savingConfig ? 'saving…' : configDirty ? 'Save as vault default' : '✓ saved'}</button>
  </div>
</section>
