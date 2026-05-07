<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type Email } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';

  // /emails — manual CRM tracker for inbound + outbound mail.
  // Granit doesn't fetch real email; this is a record-keeping
  // surface so important threads don't fall through the cracks.
  // Three filter tabs (All / Inbox / Follow-up due) cover the
  // 95% case; status-edit and follow-up-date live in the per-row
  // editor.

  type Tab = 'all' | 'inbox' | 'followup' | 'archived';

  let emails = $state<Email[]>([]);
  let loading = $state(false);
  let tab = $state<Tab>('inbox');
  let q = $state('');
  let composeOpen = $state(false);
  let composeBusy = $state(false);
  let edit = $state<Email | null>(null);

  // Compose form state. Resets each time the modal opens.
  let cDirection = $state<'in' | 'out'>('in');
  let cSubject = $state('');
  let cFrom = $state('');
  let cTo = $state('');
  let cBody = $state('');
  let cReceived = $state('');
  let cFollowUp = $state('');
  let cProject = $state('');

  async function load() {
    loading = true;
    try {
      const r = await api.listEmails();
      emails = r.emails;
    } catch (err) {
      toast.error('Failed to load emails: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      loading = false;
    }
  }
  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/emails.json') load();
    });
  });

  let todayISO = new Date().toISOString().slice(0, 10);

  let filtered = $derived.by(() => {
    let out = emails;
    if (tab === 'inbox') out = out.filter((e) => e.status === 'inbox' || e.status === 'read');
    else if (tab === 'followup') {
      out = out.filter((e) => e.follow_up_date && e.follow_up_date <= todayISO && e.status !== 'archived');
    } else if (tab === 'archived') {
      out = out.filter((e) => e.status === 'archived');
    }
    if (q.trim()) {
      const ql = q.trim().toLowerCase();
      out = out.filter((e) =>
        e.subject.toLowerCase().includes(ql) ||
        e.from.toLowerCase().includes(ql) ||
        (e.to ?? []).some((t) => t.toLowerCase().includes(ql)) ||
        (e.body ?? '').toLowerCase().includes(ql) ||
        (e.project ?? '').toLowerCase().includes(ql)
      );
    }
    return out;
  });

  // Counts surfaced on the tab strip so the user can see at a
  // glance how many items each filter would show.
  let counts = $derived({
    all: emails.length,
    inbox: emails.filter((e) => e.status === 'inbox' || e.status === 'read').length,
    followup: emails.filter((e) => e.follow_up_date && e.follow_up_date <= todayISO && e.status !== 'archived').length,
    archived: emails.filter((e) => e.status === 'archived').length
  });

  function openCompose() {
    cDirection = 'in';
    cSubject = '';
    cFrom = '';
    cTo = '';
    cBody = '';
    cReceived = new Date().toISOString().slice(0, 16); // datetime-local
    cFollowUp = '';
    cProject = '';
    composeOpen = true;
  }

  async function submitCompose(e: SubmitEvent) {
    e.preventDefault();
    if (composeBusy) return;
    composeBusy = true;
    try {
      const payload: Partial<Email> = {
        direction: cDirection,
        subject: cSubject.trim(),
        from: cFrom.trim(),
        to: cTo.split(',').map((s) => s.trim()).filter(Boolean),
        body: cBody.trim() || undefined,
        status: 'inbox',
        project: cProject.trim() || undefined
      };
      if (cDirection === 'in' && cReceived) {
        payload.received_at = new Date(cReceived).toISOString();
      } else if (cDirection === 'out' && cReceived) {
        payload.sent_at = new Date(cReceived).toISOString();
      }
      if (cFollowUp) payload.follow_up_date = cFollowUp;
      await api.createEmail(payload);
      composeOpen = false;
      toast.success('Email logged');
      await load();
    } catch (err) {
      toast.error('Save failed: ' + (err instanceof Error ? err.message : String(err)));
    } finally {
      composeBusy = false;
    }
  }

  async function setStatus(em: Email, status: Email['status']) {
    try {
      const updated = await api.patchEmail(em.id, { status });
      const i = emails.findIndex((x) => x.id === em.id);
      if (i >= 0) emails[i] = updated;
      // Trigger reactivity (Svelte 5 needs reassignment for arrays)
      emails = [...emails];
    } catch (err) {
      toast.error('Failed: ' + (err instanceof Error ? err.message : String(err)));
    }
  }

  async function deleteEmail(em: Email) {
    if (!confirm(`Delete "${em.subject}"? This can't be undone.`)) return;
    try {
      await api.deleteEmail(em.id);
      emails = emails.filter((x) => x.id !== em.id);
    } catch (err) {
      toast.error('Failed: ' + (err instanceof Error ? err.message : String(err)));
    }
  }

  function fmtDate(iso?: string): string {
    if (!iso) return '';
    try {
      return new Date(iso).toLocaleString(undefined, {
        month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit', hour12: false
      });
    } catch { return iso; }
  }
  function fmtDay(iso?: string): string {
    if (!iso) return '';
    try {
      return new Date(iso).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
    } catch { return iso; }
  }

  // For the "Follow-up due" tab: how overdue is the row?
  function followUpStatus(iso: string): { label: string; cls: string } {
    if (!iso) return { label: '', cls: '' };
    const d = iso;
    if (d < todayISO) return { label: 'overdue', cls: 'text-error' };
    if (d === todayISO) return { label: 'today', cls: 'text-warning' };
    return { label: d, cls: 'text-dim' };
  }
</script>

<svelte:head>
  <title>Emails · Granit</title>
</svelte:head>

<div class="h-full overflow-y-auto">
  <div class="p-4 sm:p-6 lg:p-8 max-w-4xl mx-auto">
    <PageHeader title="Emails" subtitle="Manual record of inbound + outbound correspondence">
      {#snippet actions()}
        <button
          onclick={openCompose}
          class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm font-medium"
        >+ Log email</button>
      {/snippet}
    </PageHeader>

    <!-- Tabs + search row. -->
    <div class="flex flex-wrap items-center gap-2 mb-4">
      <div class="flex bg-surface0 border border-surface1 rounded overflow-hidden text-sm">
        {#each [
          { id: 'inbox', label: 'Inbox' },
          { id: 'followup', label: 'Follow-up' },
          { id: 'all', label: 'All' },
          { id: 'archived', label: 'Archive' }
        ] as t}
          {@const c = counts[t.id as Tab]}
          <button
            type="button"
            class="px-3 py-1.5 transition-colors flex items-center gap-1.5
              {tab === t.id ? 'bg-primary text-on-primary' : 'text-subtext hover:bg-surface1'}"
            onclick={() => (tab = t.id as Tab)}
          >
            <span>{t.label}</span>
            <span class="text-[10px] font-mono tabular-nums opacity-70">{c}</span>
          </button>
        {/each}
      </div>
      <input
        bind:value={q}
        placeholder="Filter…"
        class="flex-1 min-w-0 max-w-xs px-3 py-1.5 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      <span class="text-xs text-dim font-mono tabular-nums ml-auto">{filtered.length}/{emails.length}</span>
    </div>

    {#if loading && emails.length === 0}
      <div class="text-sm text-dim italic">loading…</div>
    {:else if filtered.length === 0 && emails.length === 0}
      <!-- Onboarding empty state for the very first email. -->
      <div class="max-w-md mx-auto py-12 text-center">
        <div class="text-5xl mb-3 opacity-30">✉</div>
        <h2 class="text-lg font-semibold text-text mb-2">No emails logged yet</h2>
        <p class="text-sm text-dim mb-4">
          Granit doesn't fetch your inbox. Use this as a record of important threads — set a follow-up date and the dashboard will surface it on the day.
        </p>
        <button
          onclick={openCompose}
          class="px-3 py-1.5 bg-primary text-on-primary rounded text-sm"
        >Log your first email</button>
      </div>
    {:else if filtered.length === 0}
      <div class="max-w-md mx-auto py-12 text-center">
        <div class="text-3xl mb-3 opacity-30">🔍</div>
        <p class="text-sm text-dim">No emails match the current filter.</p>
      </div>
    {:else}
      <ul class="space-y-2">
        {#each filtered as em (em.id)}
          {@const dirArrow = em.direction === 'in' ? '←' : '→'}
          {@const dirColor = em.direction === 'in' ? 'text-info' : 'text-secondary'}
          {@const fu = em.follow_up_date ? followUpStatus(em.follow_up_date) : null}
          <li class="bg-surface0 border border-surface1 rounded p-3 hover:border-primary/40 transition-colors">
            <div class="flex items-start gap-3">
              <span class="text-base font-mono {dirColor} flex-shrink-0 mt-0.5" title={em.direction === 'in' ? 'incoming' : 'outgoing'}>
                {dirArrow}
              </span>
              <div class="flex-1 min-w-0">
                <div class="flex items-baseline gap-2 flex-wrap">
                  <h3 class="text-sm font-medium text-text truncate">{em.subject}</h3>
                  {#if em.status === 'replied'}
                    <span class="text-[10px] uppercase px-1.5 rounded bg-success/15 text-success">replied</span>
                  {:else if em.status === 'archived'}
                    <span class="text-[10px] uppercase px-1.5 rounded bg-surface1 text-dim">archived</span>
                  {:else if em.status === 'inbox'}
                    <span class="text-[10px] uppercase px-1.5 rounded bg-primary/15 text-primary">inbox</span>
                  {/if}
                  {#if fu}
                    <span class="text-[10px] {fu.cls}" title="follow-up">⏰ {fu.label}</span>
                  {/if}
                </div>
                <div class="text-[11px] text-dim mt-0.5 truncate">
                  <span class="font-mono">{em.direction === 'in' ? 'from' : 'to'}</span>
                  <span class="text-subtext">{em.direction === 'in' ? em.from : (em.to ?? []).join(', ')}</span>
                  {#if em.received_at || em.sent_at}
                    <span class="ml-2 font-mono tabular-nums">{fmtDate(em.received_at || em.sent_at)}</span>
                  {/if}
                  {#if em.project}
                    <span class="ml-2 text-secondary">· {em.project}</span>
                  {/if}
                </div>
                {#if em.body}
                  <p class="text-xs text-subtext mt-1.5 line-clamp-2 whitespace-pre-wrap break-words">{em.body}</p>
                {/if}
              </div>
              <div class="flex flex-col gap-1 flex-shrink-0">
                {#if em.status !== 'replied'}
                  <button
                    onclick={() => setStatus(em, 'replied')}
                    title="Mark replied"
                    class="px-2 py-0.5 text-[10px] bg-success/15 text-success rounded hover:bg-success/25"
                  >replied</button>
                {/if}
                {#if em.status !== 'archived'}
                  <button
                    onclick={() => setStatus(em, 'archived')}
                    title="Archive"
                    class="px-2 py-0.5 text-[10px] bg-surface1 text-subtext rounded hover:bg-surface2"
                  >archive</button>
                {:else}
                  <button
                    onclick={() => setStatus(em, 'inbox')}
                    title="Move back to inbox"
                    class="px-2 py-0.5 text-[10px] bg-surface1 text-subtext rounded hover:bg-surface2"
                  >unarchive</button>
                {/if}
                <button
                  onclick={() => deleteEmail(em)}
                  title="Delete"
                  class="px-2 py-0.5 text-[10px] text-error hover:bg-error/10 rounded"
                >×</button>
              </div>
            </div>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

{#if composeOpen}
  <div
    class="fixed inset-0 z-50 bg-mantle/80 flex items-center justify-center p-3"
    onclick={() => (composeOpen = false)}
    role="presentation"
  >
    <form
      onclick={(e) => e.stopPropagation()}
      onsubmit={submitCompose}
      class="bg-base border border-surface1 rounded-lg shadow-2xl p-4 sm:p-6 w-full max-w-lg max-h-[90vh] overflow-y-auto"
    >
      <h2 class="text-base font-semibold text-text mb-3">Log email</h2>
      <div class="space-y-3">
        <div class="flex gap-2">
          {#each [
            { id: 'in', label: 'Incoming ←' },
            { id: 'out', label: 'Outgoing →' }
          ] as d}
            <button
              type="button"
              onclick={() => (cDirection = d.id as 'in' | 'out')}
              class="flex-1 px-3 py-2 rounded text-sm border-2 transition-colors
                {cDirection === d.id ? 'border-primary bg-primary/10 text-primary' : 'border-surface1 bg-surface0 text-subtext'}"
            >{d.label}</button>
          {/each}
        </div>
        <label class="block text-sm">
          <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Subject</span>
          <input bind:value={cSubject} required placeholder="Q3 review proposal"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary" />
        </label>
        <label class="block text-sm">
          <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">{cDirection === 'in' ? 'From' : 'From (you)'}</span>
          <input bind:value={cFrom} required placeholder="alice@example.com"
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary" />
        </label>
        <label class="block text-sm">
          <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">To <span class="text-dim/70">(comma-separated)</span></span>
          <input bind:value={cTo} placeholder={cDirection === 'in' ? 'me@example.com' : 'bob@example.com'}
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary" />
        </label>
        <label class="block text-sm">
          <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">{cDirection === 'in' ? 'Received at' : 'Sent at'}</span>
          <input type="datetime-local" bind:value={cReceived}
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary" />
        </label>
        <div class="grid grid-cols-2 gap-3">
          <label class="block text-sm">
            <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Follow-up <span class="text-dim/70">(optional)</span></span>
            <input type="date" bind:value={cFollowUp}
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary" />
          </label>
          <label class="block text-sm">
            <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Project</span>
            <input bind:value={cProject} placeholder="(optional)"
              class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text focus:outline-none focus:border-primary" />
          </label>
        </div>
        <label class="block text-sm">
          <span class="block text-[11px] uppercase tracking-wider text-dim mb-1">Body / notes</span>
          <textarea bind:value={cBody} rows="5" placeholder="Paste the email or jot what they said + your response plan."
            class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text font-mono focus:outline-none focus:border-primary"></textarea>
        </label>
      </div>
      <div class="flex items-center gap-2 mt-4">
        <button type="button" onclick={() => (composeOpen = false)} class="px-3 py-1.5 text-sm text-subtext hover:text-text">cancel</button>
        <span class="flex-1"></span>
        <button type="submit" disabled={composeBusy || !cSubject.trim()} class="px-4 py-1.5 bg-primary text-on-primary rounded text-sm font-medium disabled:opacity-50">
          {composeBusy ? 'saving…' : 'Save'}
        </button>
      </div>
    </form>
  </div>
{/if}
