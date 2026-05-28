<script lang="ts">
  // AI tab — features + audit + provider config + web research.
  // Two main sections: "AI features" (per-feature toggles + audit/snapshot)
  // and "AI provider" (provider/model/keys). The legacy in-editor
  // features (summarise, extract_tasks, suggest_tags, rewrite, chat)
  // live under "Show legacy features" so the toggle list at default
  // stays at the 6 active capabilities.
  import { api, type OpenAIModelOption } from '$lib/api';
  import { errorMessage } from '$lib/util/errorMessage';
  import Skeleton from '$lib/components/Skeleton.svelte';
  import { toast } from '$lib/components/toast';
  import SettingsSection from './SettingsSection.svelte';
  import SettingsRow from './SettingsRow.svelte';
  import ConfirmButton from './ConfirmButton.svelte';
  import type { AppConfig, AppConfigPatch } from '$lib/api';

  type AIPrefs = {
    features: Record<string, { enabled: boolean; provider?: string }>;
    redaction_enabled: boolean;
    disabled_redaction?: string[];
    default_provider?: string;
  };
  type AIStatus = {
    sabbath_active: boolean;
    global_provider: string;
    global_model: string;
    redaction: boolean;
    default_provider?: string;
    features: Record<string, { enabled: boolean; provider: string; model: string; source: string }>;
  } | null;
  type WebSearchCfg = {
    provider: string;
    brave_key_set: boolean;
    max_results: number;
  };
  type AuditEntry = {
    timestamp: string;
    feature: string;
    provider?: string;
    model?: string;
    prompt_size_bytes: number;
    response_size_bytes?: number;
    prompt_tokens?: number;
    completion_tokens?: number;
    cost_micro_cents?: number;
    redactions?: { name: string; count: number }[];
    error?: string;
  };

  type Props = {
    appCfg: AppConfig | null;
    configBusy: boolean;
    openAIKeyBuf: string;
    openAIModels: OpenAIModelOption[] | null;
    aiPrefs: AIPrefs;
    aiStatus: AIStatus;
    aiAudit: AuditEntry[];
    aiSnapshotJSON: string;
    webSearchCfg: WebSearchCfg;
    webSearchBraveKeyBuf: string;
    webSearchBusy: boolean;
    patchConfig: (patch: AppConfigPatch) => Promise<void>;
    commitOpenAIKey: () => Promise<void>;
    clearOpenAIKey: () => Promise<void>;
    ensureOpenAIModels: () => Promise<void>;
    saveAIPrefs: () => void;
    toggleAIFeature: (id: string, enabled: boolean) => Promise<void>;
    loadAIStatus: () => Promise<void>;
    loadAIAudit: () => Promise<void>;
    clearAIAudit: () => Promise<void>;
    loadAISnapshot: () => Promise<void>;
    patchWebSearch: (p: Partial<{ provider: string; brave_key: string; max_results: number }>) => Promise<void>;
    commitBraveKey: () => Promise<void>;
    clearBraveKey: () => Promise<void>;
  };

  let {
    appCfg,
    configBusy,
    openAIKeyBuf = $bindable(),
    openAIModels,
    aiPrefs = $bindable(),
    aiStatus,
    aiAudit,
    aiSnapshotJSON,
    webSearchCfg,
    webSearchBraveKeyBuf = $bindable(),
    webSearchBusy,
    patchConfig,
    commitOpenAIKey,
    clearOpenAIKey,
    ensureOpenAIModels,
    saveAIPrefs,
    toggleAIFeature,
    loadAIStatus,
    loadAIAudit,
    clearAIAudit,
    loadAISnapshot,
    patchWebSearch,
    commitBraveKey,
    clearBraveKey
  }: Props = $props();

  // Audit-panel reveals. Local to the tab — the audit + snapshot
  // payloads load on-demand the first time you flip them open.
  let auditOpen = $state(false);
  let snapshotOpen = $state(false);

  // Headline-feature catalog. Order = what's prominent at default;
  // legacy in-editor features go under the collapse below.
  const PRIMARY_FEATURES = [
    { id: 'daily_briefing',  label: 'Daily briefing',  desc: 'Morning summary: today\'s events + urgent tasks + 1 deadline.' },
    { id: 'weekly_review',   label: 'Weekly review',   desc: 'Friday/Sunday: drafts a Wins / Setbacks / Learned / Next-week review.' },
    { id: 'inbox_triage',    label: 'Inbox triage',    desc: 'Suggests priority + schedule for untriaged tasks.' },
    { id: 'deadline_detect', label: 'Deadline detect', desc: 'Proposes due dates on open tasks whose title carries a clear deadline signal.' },
    { id: 'annotate_note',   label: 'Annotate note',   desc: 'Proposes 3-5 margin notes anchored to lines. Review + accept in the editor right rail.' },
    { id: 'web_search',      label: 'Web research',    desc: 'Lets agents query the open internet. Off by default — the only feature that opens an outbound connection.' }
  ] as const;
  const LEGACY_FEATURES = [
    { id: 'summarise',       label: 'Summarise',         desc: 'In-editor "summarise selection / whole note".' },
    { id: 'extract_tasks',   label: 'Extract tasks',     desc: 'In-editor "extract tasks from this note".' },
    { id: 'suggest_tags',    label: 'Suggest tags',      desc: 'In-editor "suggest tags for this note".' },
    { id: 'rewrite',         label: 'Rewrite / improve', desc: 'In-editor selection rewriter.' },
    { id: 'chat',            label: 'Chat',              desc: 'The /chat page. Toggle off to disable entirely.' }
  ] as const;

  // ── Usage rollup ─────────────────────────────────────────────────
  // Re-derived from the audit list; identical math to the original.
  function formatBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KB`;
    return `${(n / 1024 / 1024).toFixed(1)} MB`;
  }
  function formatCost(microCents?: number): string {
    if (!microCents || microCents <= 0) return '—';
    const dollars = microCents / 1_000_000 / 100;
    if (dollars < 0.0001) return '<$0.0001';
    let s = dollars.toFixed(4);
    s = s.replace(/(\.\d*?)0+$/, '$1').replace(/\.$/, '');
    return '$' + s;
  }
  function formatTokens(n: number): string {
    if (n < 1000) return `${n}`;
    return `${(n / 1000).toFixed(1)}k`;
  }

  const aiUsageByFeature = $derived.by(() => {
    const sevenDaysAgo = Date.now() - 7 * 24 * 60 * 60 * 1000;
    const buckets = new Map<string, { count: number; tokens: number; cost: number }>();
    for (const e of aiAudit) {
      const t = new Date(e.timestamp).getTime();
      if (t < sevenDaysAgo) continue;
      const key = e.feature || 'unknown';
      const b = buckets.get(key) ?? { count: 0, tokens: 0, cost: 0 };
      b.count++;
      b.tokens += (e.prompt_tokens ?? 0) + (e.completion_tokens ?? 0);
      b.cost += e.cost_micro_cents ?? 0;
      buckets.set(key, b);
    }
    return Array.from(buckets.entries())
      .map(([feature, v]) => ({ feature, ...v }))
      .sort((a, b) => b.cost - a.cost || b.count - a.count);
  });

  const aiUsage = $derived.by(() => {
    const now = Date.now();
    const todayStart = new Date(); todayStart.setHours(0, 0, 0, 0);
    const sevenDaysAgo = now - 7 * 24 * 60 * 60 * 1000;
    let todayN = 0, todayIn = 0, todayOut = 0, todayErr = 0, todayPT = 0, todayCT = 0, todayCost = 0;
    let weekN = 0, weekIn = 0, weekOut = 0, weekErr = 0, weekPT = 0, weekCT = 0, weekCost = 0;
    for (const e of aiAudit) {
      const t = new Date(e.timestamp).getTime();
      const inB = e.prompt_size_bytes ?? 0;
      const outB = e.response_size_bytes ?? 0;
      const pT = e.prompt_tokens ?? 0;
      const cT = e.completion_tokens ?? 0;
      const cost = e.cost_micro_cents ?? 0;
      if (t >= todayStart.getTime()) {
        todayN++; todayIn += inB; todayOut += outB; todayPT += pT; todayCT += cT; todayCost += cost;
        if (e.error) todayErr++;
      }
      if (t >= sevenDaysAgo) {
        weekN++; weekIn += inB; weekOut += outB; weekPT += pT; weekCT += cT; weekCost += cost;
        if (e.error) weekErr++;
      }
    }
    return {
      todayN, todayIn, todayOut, todayErr, todayPT, todayCT, todayCost,
      weekN, weekIn, weekOut, weekErr, weekPT, weekCT, weekCost
    };
  });
</script>

{#snippet featureToggle(f: { id: string; label: string; desc: string })}
  {@const cfg = aiPrefs.features[f.id] ?? { enabled: false }}
  {@const st = aiStatus?.features[f.id]}
  <label class="flex items-start gap-2.5 py-1.5 cursor-pointer">
    <input
      type="checkbox"
      checked={cfg.enabled}
      onchange={(e) => { void toggleAIFeature(f.id, (e.target as HTMLInputElement).checked); }}
      class="mt-1 w-4 h-4 accent-primary cursor-pointer"
    />
    <div class="flex-1 min-w-0">
      <div class="flex items-baseline gap-2 flex-wrap">
        <span class="text-sm text-text">{f.label}</span>
        {#if cfg.enabled && st}
          <span
            class="text-[10px] font-mono px-1.5 py-0.5 rounded bg-surface1 text-subtext"
            title={st.source === 'feature' ? 'Per-feature override' : st.source === 'default' ? 'Prefs default_provider' : 'Global ai_provider from config.json'}
          >via {st.provider} · {st.model}</span>
        {/if}
        {#if cfg.enabled}
          <select
            value={cfg.provider ?? ''}
            onchange={(e) => {
              const v = (e.target as HTMLSelectElement).value;
              aiPrefs.features[f.id] = { ...cfg, provider: v || undefined };
              saveAIPrefs();
              setTimeout(() => { void loadAIStatus(); }, 600);
            }}
            class="text-[10px] bg-surface1 border border-surface1 rounded px-1 py-0.5 text-subtext hover:border-primary cursor-pointer"
            onclick={(e) => e.stopPropagation()}
            title="Override the provider for this feature only"
          >
            <option value="">(default)</option>
            <option value="ollama">ollama</option>
            <option value="openai">openai</option>
          </select>
        {/if}
      </div>
      <div class="text-[11px] text-dim leading-snug">{f.desc}</div>
    </div>
  </label>
{/snippet}

<!-- AI provider — top of the tab because every feature below routes
     through it. Provider switcher + the relevant key/model fields. -->
<SettingsSection title="AI provider">
  {#snippet children()}
    {#if !appCfg}
      <Skeleton class="h-4 w-1/3 mb-2" />
      <Skeleton class="h-4 w-1/2" />
    {:else}
      {@const provider = appCfg.ai_provider || 'ollama'}
      {@const ready = (provider === 'openai' && appCfg.openai_key_set)
        || (provider === 'ollama' || provider === 'local' || provider === '')}
      <SettingsRow label="Provider" help={(appCfg.ai_provider || 'ollama') === 'openai' ? 'Key at platform.openai.com/api-keys · ~$0.0001 per call on gpt-4o-mini.' : 'Run ollama serve locally. Free, private, slower than cloud.'}>
        {#snippet control()}
          <select
            value={appCfg.ai_provider || 'ollama'}
            onchange={(e) => patchConfig({ ai_provider: (e.target as HTMLSelectElement).value })}
            disabled={configBusy}
            class="w-full sm:w-64 px-2 py-1.5 bg-mantle border border-surface1 rounded text-text text-sm"
          >
            <option value="openai">OpenAI (cloud)</option>
            <option value="ollama">Ollama (local)</option>
          </select>
          <span
            class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded font-medium"
            style="color: var(--color-{ready ? 'success' : 'warning'}); background: color-mix(in srgb, var(--color-{ready ? 'success' : 'warning'}) 14%, transparent);"
          >{ready ? 'configured' : 'needs key'}</span>
        {/snippet}
      </SettingsRow>

      {#if appCfg.ai_provider === 'openai' || (!appCfg.ai_provider && appCfg.openai_key_set)}
        {void ensureOpenAIModels()}
        <SettingsRow label="OpenAI model" help={appCfg.openai_model ? `current: ${appCfg.openai_model}` : 'Recommended: gpt-5.4-mini (workhorse) or gpt-5.4-nano (cheap).'}>
          {#snippet control()}
            {#if openAIModels === null}
              <Skeleton class="h-8 w-56" />
            {:else if openAIModels.length === 0}
              <input
                value={appCfg.openai_model}
                onblur={(e) => patchConfig({ openai_model: (e.target as HTMLInputElement).value })}
                placeholder="gpt-4o-mini"
                class="w-full sm:w-64 px-2 py-1.5 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
              />
            {:else}
              <select
                value={appCfg.openai_model}
                onchange={(e) => patchConfig({ openai_model: (e.target as HTMLSelectElement).value })}
                disabled={configBusy}
                class="w-full sm:w-64 px-2 py-1.5 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
              >
                {#if appCfg.openai_model && !openAIModels.find((m) => m.id === appCfg!.openai_model)}
                  <option value={appCfg.openai_model}>{appCfg.openai_model} (custom)</option>
                {/if}
                {#each openAIModels as m (m.id)}
                  <option value={m.id}>
                    {m.id} — in {m.input_per_m} / out {m.output_per_m} per 1M{m.note ? ` · ${m.note}` : ''}
                  </option>
                {/each}
              </select>
            {/if}
          {/snippet}
        </SettingsRow>
        <SettingsRow label="OpenAI API key" help="Stored in ~/.config/granit/config.json. Never echoed back.">
          {#snippet control()}
            <input
              type="password"
              bind:value={openAIKeyBuf}
              placeholder={appCfg.openai_key_set ? 'set — type to replace' : 'sk-…'}
              class="w-full sm:w-56 px-2 py-1.5 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
            />
            <button
              type="button"
              onclick={commitOpenAIKey}
              disabled={!openAIKeyBuf.trim() || configBusy}
              class="px-2.5 py-1 text-xs bg-primary text-on-primary rounded disabled:opacity-50"
            >Save</button>
            {#if appCfg.openai_key_set}
              <ConfirmButton
                label="Clear"
                confirmLabel="Clear key?"
                danger
                title="Clear the OpenAI API key"
                onconfirm={clearOpenAIKey}
              />
            {/if}
          {/snippet}
        </SettingsRow>
      {/if}

      {#if appCfg.ai_provider === 'ollama' || appCfg.ai_provider === 'local' || appCfg.ai_provider === ''}
        <SettingsRow label="Ollama URL">
          {#snippet control()}
            <input
              value={appCfg.ollama_url}
              onblur={(e) => patchConfig({ ollama_url: (e.target as HTMLInputElement).value })}
              placeholder="http://localhost:11434"
              class="w-full sm:w-72 px-2 py-1.5 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
            />
          {/snippet}
        </SettingsRow>
        <SettingsRow label="Ollama model" help={`Pull first with: ollama pull ${appCfg.ollama_model || 'llama3.2'}`}>
          {#snippet control()}
            <input
              value={appCfg.ollama_model}
              onblur={(e) => patchConfig({ ollama_model: (e.target as HTMLInputElement).value })}
              placeholder="llama3.2"
              class="w-full sm:w-64 px-2 py-1.5 bg-mantle border border-surface1 rounded text-text font-mono text-xs"
            />
          {/snippet}
        </SettingsRow>
      {/if}
    {/if}
  {/snippet}
</SettingsSection>

<!-- AI features — per-feature opt-in toggles. Legacy in-editor
     features collapse behind "Show legacy features" so the
     headline list at default is the 6 active capabilities. -->
<SettingsSection
  title="AI features"
  status="opt-in · redacted · audited"
  advancedLabel="Show legacy in-editor features"
>
  {#snippet children()}
    <p class="text-xs text-dim mb-2 leading-snug">
      Each feature checks the toggle before doing any work. Prompts are passed through a PII-redaction pass. Every request is recorded to an audit log you can inspect below.
    </p>

    {#if aiStatus?.sabbath_active}
      <div class="mb-2 px-2.5 py-1.5 text-[11px] bg-surface0 border border-warning rounded text-warning leading-snug">
        Sabbath mode active — AI requests are paused today. Toggles still work; calls just won't fire until Sabbath ends.
      </div>
    {/if}
    {#if aiStatus}
      <div class="mb-2 text-[11px] text-dim">
        Default backend: <span class="text-subtext font-mono">{aiStatus.global_provider} · {aiStatus.global_model}</span>
      </div>
    {/if}

    <div class="space-y-0.5">
      {#each PRIMARY_FEATURES as f (f.id)}
        {@render featureToggle(f)}
      {/each}
    </div>

    <!-- Web research config — only meaningful when web_search is on. -->
    {#if aiPrefs.features['web_search']?.enabled}
      <div class="mt-3 pt-3 border-t border-surface1 space-y-2">
        <div class="flex items-baseline gap-2">
          <h3 class="text-xs uppercase tracking-wider text-dim font-medium">Web research</h3>
          <span class="text-[10px] text-dim italic">opt-in · outbound</span>
        </div>
        <p class="text-[11px] text-dim leading-snug">
          Agents listing <code class="text-subtext">web_search</code> or <code class="text-subtext">fetch_url</code> in their tool catalog can run live queries through your provider. No outbound calls until enabled.
        </p>

        <div class="grid gap-2 sm:grid-cols-2">
          <div>
            <label for="ws-provider" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Provider</label>
            <select
              id="ws-provider"
              value={webSearchCfg.provider}
              onchange={(e) => { void patchWebSearch({ provider: (e.target as HTMLSelectElement).value }); }}
              disabled={webSearchBusy}
              class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text"
            >
              <option value="duckduckgo">DuckDuckGo (no key)</option>
              <option value="brave">Brave Search (API key)</option>
            </select>
          </div>
          <div>
            <label for="ws-limit" class="block text-[11px] uppercase tracking-wider text-dim mb-1">Default result count</label>
            <input
              id="ws-limit"
              type="number"
              min="1"
              max="10"
              value={webSearchCfg.max_results || 5}
              onchange={(e) => {
                const n = parseInt((e.target as HTMLInputElement).value, 10) || 5;
                void patchWebSearch({ max_results: n });
              }}
              disabled={webSearchBusy}
              class="w-full px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm text-text"
            />
          </div>
        </div>

        {#if webSearchCfg.provider === 'brave'}
          <div class="mt-1">
            <label for="ws-brave-key" class="block text-[11px] uppercase tracking-wider text-dim mb-1">
              Brave API key {#if webSearchCfg.brave_key_set}<span class="text-success normal-case">· set</span>{/if}
            </label>
            <div class="flex gap-1.5 flex-wrap">
              <input
                id="ws-brave-key"
                type="password"
                bind:value={webSearchBraveKeyBuf}
                placeholder={webSearchCfg.brave_key_set ? 'set — paste to replace' : 'paste your Brave Search API key'}
                autocomplete="off"
                class="flex-1 min-w-[12rem] px-2 py-1.5 bg-mantle border border-surface1 rounded text-sm font-mono text-text"
              />
              <button
                type="button"
                onclick={() => void commitBraveKey()}
                disabled={!webSearchBraveKeyBuf.trim() || webSearchBusy}
                class="px-2.5 py-1.5 text-xs bg-primary text-on-primary rounded disabled:opacity-50"
              >Save</button>
              {#if webSearchCfg.brave_key_set}
                <ConfirmButton
                  label="Clear"
                  confirmLabel="Clear key?"
                  danger
                  disabled={webSearchBusy}
                  title="Clear the Brave Search API key"
                  onconfirm={clearBraveKey}
                />
              {/if}
            </div>
            <p class="text-[10px] text-dim mt-1 leading-snug">
              Free key at <a href="https://api.search.brave.com/" target="_blank" rel="noopener" class="text-secondary underline">api.search.brave.com</a>. Stored under <code class="text-subtext">.granit/web-search.json</code>.
            </p>
          </div>
        {/if}
      </div>
    {/if}

    <div class="mt-3 pt-3 border-t border-surface1 flex items-center gap-3">
      <label class="flex items-center gap-2 text-xs text-subtext flex-1">
        <input
          type="checkbox"
          checked={aiPrefs.redaction_enabled}
          onchange={(e) => { aiPrefs.redaction_enabled = (e.target as HTMLInputElement).checked; saveAIPrefs(); }}
          class="w-4 h-4 accent-primary"
        />
        Redact PII (emails, phone, IBAN, cards, IPs) before prompts leave the device
      </label>
    </div>

    <div class="mt-2 pt-2 flex items-center gap-2 flex-wrap">
      <button
        type="button"
        onclick={() => { auditOpen = !auditOpen; if (auditOpen) void loadAIAudit(); }}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary"
      >{auditOpen ? 'Hide audit log' : 'View audit log'}</button>
      <button
        type="button"
        onclick={() => { snapshotOpen = !snapshotOpen; if (snapshotOpen) void loadAISnapshot(); }}
        class="px-2.5 py-1 text-xs bg-surface0 border border-surface1 rounded text-subtext hover:border-primary"
      >{snapshotOpen ? 'Hide snapshot' : 'What AI sees'}</button>
      <span class="flex-1"></span>
      <ConfirmButton
        label="Clear AI history"
        confirmLabel="Clear permanently?"
        danger
        title="Permanently delete the AI audit log"
        onconfirm={clearAIAudit}
      />
    </div>

    {#if auditOpen}
      <div class="mt-3 pt-3 border-t border-surface1">
        {#if aiAudit.length === 0}
          <p class="text-xs text-dim italic">No AI requests recorded yet.</p>
        {:else}
          <div class="mb-3 grid grid-cols-2 gap-2 text-[11px]">
            <div class="px-2 py-1.5 bg-mantle border border-surface1 rounded">
              <div class="text-[10px] uppercase tracking-wider text-dim">Today</div>
              <div class="text-text">{aiUsage.todayN} request{aiUsage.todayN === 1 ? '' : 's'}</div>
              {#if aiUsage.todayPT + aiUsage.todayCT > 0}
                <div class="text-dim font-mono">{formatTokens(aiUsage.todayPT)} + {formatTokens(aiUsage.todayCT)} tokens</div>
              {/if}
              <div class="text-dim font-mono">{formatBytes(aiUsage.todayIn)} in / {formatBytes(aiUsage.todayOut)} out</div>
              {#if aiUsage.todayCost > 0}
                <div class="text-secondary font-mono">{formatCost(aiUsage.todayCost)}</div>
              {/if}
              {#if aiUsage.todayErr > 0}
                <div class="text-error">{aiUsage.todayErr} error{aiUsage.todayErr === 1 ? '' : 's'}</div>
              {/if}
            </div>
            <div class="px-2 py-1.5 bg-mantle border border-surface1 rounded">
              <div class="text-[10px] uppercase tracking-wider text-dim">Last 7 days</div>
              <div class="text-text">{aiUsage.weekN} request{aiUsage.weekN === 1 ? '' : 's'}</div>
              {#if aiUsage.weekPT + aiUsage.weekCT > 0}
                <div class="text-dim font-mono">{formatTokens(aiUsage.weekPT)} + {formatTokens(aiUsage.weekCT)} tokens</div>
              {/if}
              <div class="text-dim font-mono">{formatBytes(aiUsage.weekIn)} in / {formatBytes(aiUsage.weekOut)} out</div>
              {#if aiUsage.weekCost > 0}
                <div class="text-secondary font-mono">{formatCost(aiUsage.weekCost)}</div>
              {/if}
              {#if aiUsage.weekErr > 0}
                <div class="text-error">{aiUsage.weekErr} error{aiUsage.weekErr === 1 ? '' : 's'}</div>
              {/if}
            </div>
          </div>

          {#if aiUsageByFeature.length > 1}
            <div class="mb-3 px-2 py-1.5 bg-mantle border border-surface1 rounded overflow-x-auto">
              <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5">Last 7 days · by feature</div>
              <table class="w-full text-[11px]">
                <tbody>
                  {#each aiUsageByFeature as f (f.feature)}
                    <tr>
                      <td class="text-text py-0.5 pr-2 whitespace-nowrap">{f.feature}</td>
                      <td class="text-dim font-mono py-0.5 pr-2 tabular-nums text-right whitespace-nowrap">{f.count}×</td>
                      <td class="text-dim font-mono py-0.5 pr-2 tabular-nums text-right whitespace-nowrap">{formatTokens(f.tokens)} tok</td>
                      <td class="text-secondary font-mono py-0.5 tabular-nums text-right whitespace-nowrap">{f.cost > 0 ? formatCost(f.cost) : '—'}</td>
                    </tr>
                  {/each}
                </tbody>
              </table>
            </div>
          {/if}

          <ul class="space-y-1 text-[11px] font-mono max-h-72 overflow-y-auto">
            {#each aiAudit as e (e.timestamp + e.feature)}
              <li class="px-2 py-1.5 bg-mantle border border-surface1 rounded">
                <div class="flex items-baseline gap-2 flex-wrap">
                  <span class="text-dim">{new Date(e.timestamp).toLocaleString()}</span>
                  <span class="text-text font-semibold">{e.feature}</span>
                  {#if e.provider}<span class="text-secondary">via {e.provider}{e.model ? ` · ${e.model}` : ''}</span>{/if}
                  <span class="text-dim ml-auto">
                    {#if (e.prompt_tokens ?? 0) + (e.completion_tokens ?? 0) > 0}
                      {e.prompt_tokens ?? 0}+{e.completion_tokens ?? 0} tok
                    {:else}
                      {e.prompt_size_bytes}B / {e.response_size_bytes ?? 0}B
                    {/if}
                    {#if e.cost_micro_cents && e.cost_micro_cents > 0}
                      <span class="text-secondary ml-1">{formatCost(e.cost_micro_cents)}</span>
                    {/if}
                  </span>
                </div>
                {#if e.redactions && e.redactions.length > 0}
                  <div class="text-dim mt-0.5">
                    Redacted:
                    {#each e.redactions as r, i (r.name)}
                      <span class="ml-1">{r.count}× {r.name}{i < (e.redactions?.length ?? 0) - 1 ? ',' : ''}</span>
                    {/each}
                  </div>
                {/if}
                {#if e.error}
                  <div class="text-error mt-0.5">{e.error}</div>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </div>
    {/if}

    {#if snapshotOpen}
      <div class="mt-3 pt-3 border-t border-surface1">
        <p class="text-[11px] text-dim mb-2">
          JSON shape AI features pass to your provider. Note bodies and email contents are <strong>not</strong> included by default.
        </p>
        <pre class="max-h-72 overflow-auto bg-mantle border border-surface1 rounded p-2 text-[10px] font-mono text-subtext">{aiSnapshotJSON}</pre>
      </div>
    {/if}
  {/snippet}

  {#snippet advanced()}
    <p class="text-[11px] text-dim mb-2 leading-snug">
      In-editor capabilities that have been shipping since launch. Toggle off to disable entirely.
    </p>
    <div class="space-y-0.5">
      {#each LEGACY_FEATURES as f (f.id)}
        {@render featureToggle(f)}
      {/each}
    </div>
  {/snippet}
</SettingsSection>
