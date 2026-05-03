<script lang="ts">
  import { onMount } from 'svelte';
  import { auth } from '$lib/stores/auth';
  import { api, type Person } from '$lib/api';
  import { onWsEvent } from '$lib/ws';
  import { toast } from '$lib/components/toast';
  import PageHeader from '$lib/components/PageHeader.svelte';

  // /people is a lightweight relationship tracker. The schema is
  // intentionally small: name, optional contact info, last-contacted
  // + cadence, freeform notes. The point isn't to be a CRM — it's
  // "remind me to keep in touch." Stale-first sort surfaces who needs
  // a ping; an upcoming-birthdays strip lives at the top.

  let people = $state<Person[]>([]);
  let staleCount = $state(0);
  let upcomingBirthdays = $state<Person[]>([]);
  let loading = $state(false);

  // Filter / search.
  let q = $state('');
  let showArchived = $state(false);
  let filterRel = $state(''); // empty = no filter
  let filtered = $derived.by(() => {
    const term = q.trim().toLowerCase();
    return people.filter((p) => {
      if (!showArchived && p.archived) return false;
      if (filterRel && p.relationship !== filterRel) return false;
      if (term) {
        const hay = [p.name, p.email ?? '', p.phone ?? '', p.relationship ?? '', (p.tags ?? []).join(' '), p.notes ?? '']
          .join(' ').toLowerCase();
        if (!hay.includes(term)) return false;
      }
      return true;
    });
  });

  // Distinct relationship values, sorted by frequency desc — drives
  // the relationship filter chips.
  let relationships = $derived.by(() => {
    const m = new Map<string, number>();
    for (const p of people) {
      const r = (p.relationship ?? '').trim();
      if (!r) continue;
      m.set(r, (m.get(r) ?? 0) + 1);
    }
    return [...m.entries()].sort((a, b) => b[1] - a[1]).map(([r]) => r);
  });

  async function load() {
    if (!$auth) return;
    loading = true;
    try {
      const r = await api.listPeople({ birthdaysWithin: 30 });
      people = r.people;
      staleCount = r.stale_count;
      upcomingBirthdays = r.upcoming_birthdays;
    } catch (e) {
      toast.error('failed to load: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  onMount(() => {
    load();
    return onWsEvent((ev) => {
      if (ev.type === 'state.changed' && ev.path === '.granit/people.json') load();
    });
  });

  // Days-since-contact label. Negative ("never") when last_contacted_at
  // is missing, positive when a date's set. Used in the row's relative
  // time label.
  function daysSince(iso: string | undefined): number | null {
    if (!iso) return null;
    const d = new Date(iso + 'T00:00:00');
    if (Number.isNaN(d.getTime())) return null;
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    return Math.round((today.getTime() - d.getTime()) / 86400000);
  }
  function lastContactLabel(p: Person): { text: string; tone: string } {
    const days = daysSince(p.last_contacted_at);
    if (days === null) return { text: 'never contacted', tone: 'text-warning' };
    if (days === 0) return { text: 'today', tone: 'text-success' };
    if (days === 1) return { text: 'yesterday', tone: 'text-text' };
    if (days < 7) return { text: `${days}d ago`, tone: 'text-text' };
    if (days < 30) return { text: `${days}d ago`, tone: 'text-subtext' };
    if (days < 90) return { text: `${Math.floor(days / 30)}mo ago`, tone: 'text-warning' };
    return { text: `${Math.floor(days / 30)}mo ago`, tone: 'text-error' };
  }

  // Stale = cadence set + last contact older than cadence days.
  // Mirrors the Go-side IsStale logic so the visual marker matches
  // the staleCount the server returns.
  function isStale(p: Person): boolean {
    if (p.archived || !p.cadence_days || p.cadence_days <= 0) return false;
    const days = daysSince(p.last_contacted_at);
    if (days === null) return true;
    return days > p.cadence_days;
  }

  // Birthday rendering — handle "today" / "tomorrow" / "in N days".
  function nextBirthdayLabel(birthday: string | undefined): string {
    if (!birthday) return '';
    // Either YYYY-MM-DD or MM-DD; we project to the next occurrence.
    const m = birthday.match(/(\d{4}-)?(\d{2})-(\d{2})$/);
    if (!m) return '';
    const month = parseInt(m[2], 10);
    const day = parseInt(m[3], 10);
    const today = new Date();
    today.setHours(0, 0, 0, 0);
    let next = new Date(today.getFullYear(), month - 1, day);
    if (next < today) next.setFullYear(next.getFullYear() + 1);
    const diff = Math.round((next.getTime() - today.getTime()) / 86400000);
    if (diff === 0) return '🎂 today';
    if (diff === 1) return '🎂 tomorrow';
    if (diff <= 30) return `🎂 in ${diff} days`;
    return `🎂 ${next.toLocaleDateString(undefined, { month: 'short', day: 'numeric' })}`;
  }

  // ── Modal: create / edit a person ──────────────────────────────────
  // One modal handles both. editingId discriminates; submit branches.
  let modalOpen = $state(false);
  let editingId = $state<string | null>(null);
  let form = $state({
    name: '', email: '', phone: '', birthday: '', relationship: '',
    tags: '', cadence_days: '', last_contacted_at: '', note_path: '', notes: ''
  });
  function openCreate() {
    editingId = null;
    form = {
      name: '', email: '', phone: '', birthday: '', relationship: '',
      tags: '', cadence_days: '', last_contacted_at: '', note_path: '', notes: ''
    };
    modalOpen = true;
  }
  function openEdit(p: Person) {
    editingId = p.id;
    form = {
      name: p.name,
      email: p.email ?? '',
      phone: p.phone ?? '',
      birthday: p.birthday ?? '',
      relationship: p.relationship ?? '',
      tags: (p.tags ?? []).join(', '),
      cadence_days: p.cadence_days ? String(p.cadence_days) : '',
      last_contacted_at: p.last_contacted_at ?? '',
      note_path: p.note_path ?? '',
      notes: p.notes ?? ''
    };
    modalOpen = true;
  }
  async function submitForm() {
    if (!form.name.trim()) return;
    const body: Partial<Person> = {
      name: form.name.trim(),
      email: form.email.trim() || undefined,
      phone: form.phone.trim() || undefined,
      birthday: form.birthday.trim() || undefined,
      relationship: form.relationship.trim() || undefined,
      tags: form.tags.split(',').map((t) => t.trim()).filter(Boolean),
      cadence_days: form.cadence_days ? parseInt(form.cadence_days, 10) : 0,
      last_contacted_at: form.last_contacted_at || undefined,
      note_path: form.note_path.trim() || undefined,
      notes: form.notes.trim() || undefined
    };
    try {
      if (editingId) {
        await api.patchPerson(editingId, body);
        toast.success('updated');
      } else {
        await api.createPerson(body);
        toast.success('added');
      }
      modalOpen = false;
      editingId = null;
      await load();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  async function pingPerson(p: Person) {
    try {
      await api.pingPerson(p.id);
      toast.success(`Pinged ${p.name} — last contact stamped today`);
      await load();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
  async function deletePerson(p: Person) {
    if (!confirm(`Delete ${p.name} from your people list?`)) return;
    try {
      await api.deletePerson(p.id);
      await load();
    } catch (e) {
      toast.error('failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
</script>

<div class="h-full overflow-y-auto">
  <div class="max-w-4xl mx-auto p-4 sm:p-6 lg:p-8">
    <PageHeader title="People" subtitle="Lightweight relationship tracker — last contact, birthdays, cadence reminders" />

    <!-- Header pills + add button -->
    <div class="flex items-baseline gap-3 flex-wrap mb-4">
      <span class="text-xs text-dim">{people.length} people</span>
      {#if staleCount > 0}
        <span class="text-[11px] px-1.5 py-0.5 rounded bg-warning/15 text-warning">
          {staleCount} need a ping
        </span>
      {/if}
      <span class="flex-1"></span>
      <button onclick={openCreate} class="text-xs px-2.5 py-1 bg-primary text-on-primary rounded font-medium hover:opacity-90">+ New person</button>
    </div>

    <!-- Birthdays strip -->
    {#if upcomingBirthdays.length > 0}
      <div class="bg-surface0 border border-surface1 rounded-lg p-3 mb-4">
        <h3 class="text-xs uppercase tracking-wider text-dim font-medium mb-2">Birthdays in the next 30 days</h3>
        <div class="flex flex-wrap gap-2">
          {#each upcomingBirthdays as p (p.id)}
            <button
              type="button"
              onclick={() => openEdit(p)}
              class="text-xs px-2 py-1 rounded bg-mantle border border-surface1 hover:border-primary"
            >
              {p.name} <span class="text-subtext">{nextBirthdayLabel(p.birthday)}</span>
            </button>
          {/each}
        </div>
      </div>
    {/if}

    <!-- Search + filters -->
    <div class="mb-4 space-y-2">
      <input
        bind:value={q}
        placeholder="search name, email, tags, notes…"
        class="w-full px-3 py-2 bg-surface0 border border-surface1 rounded text-sm text-text placeholder-dim focus:outline-none focus:border-primary"
      />
      {#if relationships.length > 0 || people.some((p) => p.archived)}
        <div class="flex flex-wrap items-center gap-1.5 text-xs">
          {#if filterRel}
            <button onclick={() => (filterRel = '')} class="px-2 py-0.5 rounded bg-surface1 text-dim hover:text-text">clear</button>
          {/if}
          {#each relationships as r}
            <button
              onclick={() => (filterRel = filterRel === r ? '' : r)}
              class="px-2 py-0.5 rounded {filterRel === r ? 'bg-primary text-on-primary' : 'bg-surface0 text-subtext hover:bg-surface1'}"
            >{r}</button>
          {/each}
          <span class="flex-1"></span>
          <label class="flex items-center gap-1.5 text-dim cursor-pointer">
            <input type="checkbox" bind:checked={showArchived} class="accent-primary" />
            <span>show archived</span>
          </label>
        </div>
      {/if}
    </div>

    {#if loading && people.length === 0}
      <p class="text-sm text-dim">loading…</p>
    {:else if people.length === 0}
      <div class="bg-surface0 border border-surface1 rounded-lg p-6 text-center">
        <p class="text-sm text-text">No one tracked yet.</p>
        <p class="text-xs text-dim mt-1">Add people you want to keep in touch with — set a cadence and granit reminds you when it's been too long.</p>
      </div>
    {:else if filtered.length === 0}
      <p class="text-sm text-dim italic">No matches.</p>
    {:else}
      <ul class="space-y-2">
        {#each filtered as p (p.id)}
          {@const stale = isStale(p)}
          {@const lab = lastContactLabel(p)}
          <li class="bg-surface0 border border-surface1 rounded-lg p-3 {p.archived ? 'opacity-60' : stale ? 'border-warning/40' : ''}">
            <div class="flex items-baseline gap-3 flex-wrap">
              <button onclick={() => openEdit(p)} class="text-base font-medium text-text hover:underline truncate">{p.name}</button>
              {#if p.relationship}
                <span class="text-[10px] uppercase tracking-wider px-1.5 py-0.5 rounded bg-surface1 text-subtext">{p.relationship}</span>
              {/if}
              {#each (p.tags ?? []) as t}
                <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
              {/each}
              <span class="flex-1"></span>
              <span class="text-xs {lab.tone}">{lab.text}</span>
              {#if p.cadence_days}
                <span class="text-[10px] text-dim">cadence {p.cadence_days}d</span>
              {/if}
              <button
                onclick={() => pingPerson(p)}
                class="text-xs px-2 py-0.5 rounded bg-success/15 text-success hover:bg-success/25"
                title="Stamp last contact = today"
              >ping</button>
              <button onclick={() => deletePerson(p)} class="text-xs text-dim hover:text-error" aria-label="delete">×</button>
            </div>
            <div class="flex flex-wrap items-center gap-3 text-[11px] text-dim mt-1.5">
              {#if p.email}<span>✉ {p.email}</span>{/if}
              {#if p.phone}<span>☎ {p.phone}</span>{/if}
              {#if p.birthday}<span>{nextBirthdayLabel(p.birthday)}</span>{/if}
              {#if p.note_path}
                <a href="/notes/{encodeURIComponent(p.note_path)}" class="text-secondary hover:underline">notes ↗</a>
              {/if}
            </div>
            {#if p.notes}
              <p class="text-[12px] text-subtext mt-1.5 whitespace-pre-line">{p.notes}</p>
            {/if}
          </li>
        {/each}
      </ul>
    {/if}

    <p class="text-[11px] text-dim italic mt-6">
      Synced via <code>.granit/people.json</code> — same file the granit TUI reads.
    </p>
  </div>
</div>

<!-- ── Create / edit modal ──────────────────────────────────────────── -->
{#if modalOpen}
  <div class="fixed inset-0 z-50 bg-black/40 flex items-center justify-center p-4" onclick={() => (modalOpen = false)} role="dialog" tabindex="-1" onkeydown={(e) => { if (e.key === 'Escape') modalOpen = false; }}>
    <!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
    <form onclick={(e) => e.stopPropagation()} onkeydown={(e) => e.stopPropagation()} onsubmit={(e) => { e.preventDefault(); submitForm(); }} class="w-full max-w-md bg-mantle border border-surface1 rounded-lg shadow-xl p-4 space-y-3">
      <h2 class="text-base font-semibold text-text">{editingId ? 'Edit person' : 'New person'}</h2>
      <input bind:value={form.name} required placeholder="Name" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-sm text-text focus:outline-none focus:border-primary" />
      <div class="grid grid-cols-2 gap-2">
        <input bind:value={form.email} placeholder="Email" class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
        <input bind:value={form.phone} placeholder="Phone" class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      </div>
      <div class="grid grid-cols-2 gap-2">
        <input bind:value={form.relationship} placeholder="Relationship (friend, family…)" list="rel-list" class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
        <input type="date" bind:value={form.birthday} placeholder="Birthday" class="bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      </div>
      <datalist id="rel-list">
        {#each relationships as r}<option value={r}></option>{/each}
      </datalist>
      <input bind:value={form.tags} placeholder="Tags (comma-separated)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <div class="grid grid-cols-2 gap-2">
        <label class="block">
          <span class="text-[11px] text-dim">Last contacted</span>
          <input type="date" bind:value={form.last_contacted_at} class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
        </label>
        <label class="block">
          <span class="text-[11px] text-dim">Cadence (days, 0 = no reminder)</span>
          <input type="number" min="0" bind:value={form.cadence_days} placeholder="30" class="mt-1 w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text font-mono text-right focus:outline-none focus:border-primary" />
        </label>
      </div>
      <input bind:value={form.note_path} placeholder="Linked note path (optional, e.g. People/Sebastian.md)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text focus:outline-none focus:border-primary" />
      <textarea bind:value={form.notes} rows="3" placeholder="Notes (recent conversations, things to remember…)" class="w-full bg-surface0 border border-surface1 rounded px-2 py-1.5 text-xs text-text resize-y focus:outline-none focus:border-primary"></textarea>
      <div class="flex justify-end gap-2 pt-2">
        <button type="button" onclick={() => (modalOpen = false)} class="text-xs px-3 py-1.5 rounded bg-surface0 text-subtext hover:bg-surface1">Cancel</button>
        <button type="submit" class="text-xs px-3 py-1.5 rounded bg-primary text-on-primary font-medium hover:opacity-90">{editingId ? 'Save' : 'Add'}</button>
      </div>
    </form>
  </div>
{/if}
