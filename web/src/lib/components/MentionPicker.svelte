<script lang="ts" module>
  // @-mention reference types — exported so the parent can hold a queue
  // of MentionRef[] for the next outgoing message and the existing chip
  // strip / removeMention plumbing keeps working unchanged.
  export type MentionKind = 'task' | 'goal' | 'project' | 'deadline' | 'event' | 'note';

  export interface MentionRef {
    kind: MentionKind;
    /** Stable id (task id, goal id, deadline id, project name, note path...). */
    id: string;
    /** Display title. */
    title: string;
    /** Pre-formatted system-prompt fragment describing the entity's
     *  key fields. Built at pick time so we don't need a second fetch
     *  on send. */
    contextLine: string;
  }

  export type MentionCandidate = {
    kind: MentionKind;
    id: string;
    title: string;
    subtitle: string;
    contextLine: string;
  };
</script>

<script lang="ts">
  import { tick } from 'svelte';
  import { api } from '$lib/api';
  import { loadRagIndex, getRagIndex, isRagIndexLoaded } from '$lib/chat/rag';

  // MentionPicker — the dropdown that appears when the user types "@"
  // anywhere in the AI overlay's composer. Lists tasks, goals, projects,
  // deadlines, events, and notes (the latter piggy-backs on the RAG
  // index so we don't double-fetch). Selecting one stamps the input
  // with @<title> for the user's eyes; the parent receives a structured
  // MentionRef via onPick which it folds into the next send() as a
  // strict system message. Cleaner than splicing raw entity bodies into
  // the user message — lets the model ground its reply on real fields
  // (id, title, due date, status…) instead of the user's prose
  // glossing them.
  //
  // Extracted from AIOverlay.svelte. Same keydown contract as the
  // SlashCommandPicker: handleKey(e) returns true when the picker
  // swallowed the event, false when the parent should keep handling.

  interface Props {
    /** Two-way bound to the parent composer's textarea value. */
    value: string;
    /** Two-way bound to the parent's open flag so Esc-from-parent and
     *  outside integrations can both flip it. */
    open: boolean;
    /** The actual <textarea> element — needed to read selection start
     *  and to refocus after a pick. */
    inputEl: HTMLTextAreaElement | undefined;
    /** Fired when the user picks a candidate. Parent stashes the ref
     *  for the next outgoing message. */
    onPick: (ref: MentionRef) => void;
  }
  let {
    value = $bindable(),
    open = $bindable(),
    inputEl,
    onPick
  }: Props = $props();

  let query = $state('');
  // Anchor: where in input the @ sits (start) — replaced on pick.
  let anchor = $state(-1);
  let candidates = $state<MentionCandidate[]>([]);
  let loading = $state(false);
  let selectedIdx = $state(0);

  // Cached entity index — populated lazily on first @-mention. Same
  // shape as the prior in-component state (small enough to hold full
  // list per type).
  let mentionIndex = $state<{
    tasks: { id: string; text: string; priority: number; dueDate?: string; done: boolean }[];
    goals: { id: string; title: string; status?: string; target_date?: string }[];
    projects: { name: string; description?: string; status?: string }[];
    deadlines: { id: string; title: string; date: string; importance: string }[];
    events: { id: string; title: string; date: string; start_time?: string }[];
  }>({ tasks: [], goals: [], projects: [], deadlines: [], events: [] });
  let mentionIndexLoaded = $state(false);

  async function loadMentionIndex() {
    if (mentionIndexLoaded) return;
    loading = true;
    try {
      // Parallel fetch — each endpoint is small. Failures fall through
      // (the user still gets the working subset). Notes piggy-back on
      // the RAG index so we don't double-fetch them.
      const [tasks, goals, projects, deadlines, events] = await Promise.all([
        api.listTasks({ status: 'open' }).catch(() => ({ tasks: [], total: 0 })),
        api.listGoals().catch(() => ({ goals: [], total: 0 })),
        api.listProjects().catch(() => ({ projects: [], total: 0 })),
        api.listDeadlines().catch(() => ({ deadlines: [], total: 0 })),
        api.listEvents().catch(() => ({ events: [], total: 0 }))
      ]);
      // Pre-warm the note index for @-mention note matches. Fire-and-
      // forget — note results show up once the index lands.
      void loadRagIndex();
      mentionIndex = {
        tasks: tasks.tasks.map((t) => ({
          id: t.id,
          text: t.text,
          priority: t.priority,
          dueDate: t.dueDate,
          done: t.done
        })),
        goals: goals.goals.map((g) => ({
          id: g.id,
          title: g.title,
          status: g.status,
          target_date: g.target_date
        })),
        projects: projects.projects.map((p) => ({
          name: p.name,
          description: p.description,
          status: p.status
        })),
        deadlines: deadlines.deadlines.map((d) => ({
          id: d.id,
          title: d.title,
          date: d.date,
          importance: d.importance
        })),
        events: events.events.map((e) => ({
          id: e.id,
          title: e.title,
          date: e.date,
          start_time: e.start_time
        }))
      };
    } finally {
      mentionIndexLoaded = true;
      loading = false;
    }
  }

  // Score a candidate against the user's typed query. Substring match
  // on title/text wins over prefix; everything is lowercase. Empty
  // query returns the most recent / highest-priority entries per type.
  function buildCandidates(q: string): MentionCandidate[] {
    const ql = q.trim().toLowerCase();
    const out: MentionCandidate[] = [];
    // Tasks
    for (const t of mentionIndex.tasks) {
      const text = t.text.toLowerCase();
      if (ql && !text.includes(ql)) continue;
      const due = t.dueDate ? ` · due ${t.dueDate}` : '';
      const prio = t.priority > 0 ? `P${t.priority}` : '';
      out.push({
        kind: 'task',
        id: t.id,
        title: t.text,
        subtitle: `${prio}${due || ' · no due'}`.trim(),
        contextLine:
          `Task ${t.id}: ${t.text}` +
          (t.dueDate ? ` (due ${t.dueDate})` : '') +
          (t.priority > 0 ? ` (priority P${t.priority})` : '') +
          (t.done ? ' [done]' : '')
      });
    }
    // Goals
    for (const g of mentionIndex.goals) {
      if (ql && !g.title.toLowerCase().includes(ql)) continue;
      out.push({
        kind: 'goal',
        id: g.id,
        title: g.title,
        subtitle: `${g.status ?? 'active'}${g.target_date ? ' · ' + g.target_date : ''}`,
        contextLine:
          `Goal ${g.id}: ${g.title}` +
          (g.target_date ? ` (target ${g.target_date})` : '') +
          (g.status ? ` [status: ${g.status}]` : '')
      });
    }
    // Projects
    for (const p of mentionIndex.projects) {
      if (ql && !p.name.toLowerCase().includes(ql)) continue;
      out.push({
        kind: 'project',
        id: p.name,
        title: p.name,
        subtitle: p.status || 'project',
        contextLine:
          `Project "${p.name}"` +
          (p.description ? ` — ${p.description.slice(0, 200)}` : '')
      });
    }
    // Deadlines
    for (const d of mentionIndex.deadlines) {
      if (ql && !d.title.toLowerCase().includes(ql)) continue;
      out.push({
        kind: 'deadline',
        id: d.id,
        title: d.title,
        subtitle: `${d.date} · ${d.importance}`,
        contextLine: `Deadline "${d.title}" on ${d.date} (importance: ${d.importance})`
      });
    }
    // Events
    for (const e of mentionIndex.events) {
      if (ql && !e.title.toLowerCase().includes(ql)) continue;
      const when = e.start_time ? `${e.date} ${e.start_time}` : e.date;
      out.push({
        kind: 'event',
        id: e.id,
        title: e.title,
        subtitle: when,
        contextLine: `Event "${e.title}" on ${when}`
      });
    }
    // Notes — reuse the RAG index. Cheap subset; we only show the top
    // 8 by title match so the picker isn't dominated by 5k notes.
    if (isRagIndexLoaded()) {
      for (const n of getRagIndex()) {
        if (ql && !n.title.toLowerCase().includes(ql)) continue;
        out.push({
          kind: 'note',
          id: n.path,
          title: n.title,
          subtitle: n.path,
          // Note context is a back-pointer. Body injection is handled
          // separately via attachNote / RAG; this just tells the
          // model "the user is asking about this specific note".
          contextLine: `Note "${n.title}" at path \`${n.path}\``
        });
      }
    }
    // Cap total candidates so the picker stays scannable.
    const limit = 12;
    if (out.length <= limit) return out;
    // Prefer exact-prefix matches when query has content.
    if (ql) {
      out.sort((a, b) => {
        const ap = a.title.toLowerCase().startsWith(ql) ? 0 : 1;
        const bp = b.title.toLowerCase().startsWith(ql) ? 0 : 1;
        return ap - bp;
      });
    }
    return out.slice(0, limit);
  }

  // Walk back from caret to find a leading "@" with no whitespace
  // between it and the caret; bail if we hit whitespace first. Exposed
  // so the parent can call from oninput AND from onclick (caret moved
  // without typing).
  export function detectTrigger() {
    if (!inputEl) return;
    const caret = inputEl.selectionStart ?? -1;
    if (caret < 0) {
      open = false;
      return;
    }
    let i = caret - 1;
    while (i >= 0) {
      const c = value[i];
      if (c === '@') {
        const prev = i > 0 ? value[i - 1] : ' ';
        if (prev === ' ' || prev === '\n' || prev === '\t' || i === 0) {
          anchor = i;
          query = value.slice(i + 1, caret);
          if (!open) {
            open = true;
            selectedIdx = 0;
            void loadMentionIndex().then(() => {
              if (open) candidates = buildCandidates(query);
            });
          } else {
            candidates = buildCandidates(query);
            selectedIdx = 0;
          }
          return;
        }
        break;
      }
      if (c === ' ' || c === '\n' || c === '\t') break;
      i--;
    }
    open = false;
  }

  function pick(c: MentionCandidate) {
    if (anchor < 0) {
      open = false;
      return;
    }
    // Splice "@<query>" → "@<title> " in the input, and report the ref
    // to the parent.
    const before = value.slice(0, anchor);
    const after = value.slice((inputEl?.selectionStart ?? anchor) ?? anchor);
    const insert = `@${c.title} `;
    value = before + insert + after;
    const newCaret = before.length + insert.length;
    onPick({ kind: c.kind, id: c.id, title: c.title, contextLine: c.contextLine });
    open = false;
    anchor = -1;
    query = '';
    tick().then(() => {
      if (inputEl) {
        inputEl.focus();
        inputEl.setSelectionRange(newCaret, newCaret);
      }
    });
  }

  // Keyboard handler. Returns true when the event was swallowed.
  export function handleKey(e: KeyboardEvent): boolean {
    if (!open || candidates.length === 0) return false;
    if (e.key === 'ArrowDown') {
      e.preventDefault();
      selectedIdx = (selectedIdx + 1) % candidates.length;
      return true;
    }
    if (e.key === 'ArrowUp') {
      e.preventDefault();
      selectedIdx = (selectedIdx - 1 + candidates.length) % candidates.length;
      return true;
    }
    if (e.key === 'Enter' || e.key === 'Tab') {
      const c = candidates[selectedIdx];
      if (c) {
        e.preventDefault();
        pick(c);
        return true;
      }
    }
    if (e.key === 'Escape') {
      e.preventDefault();
      open = false;
      return true;
    }
    return false;
  }
</script>

{#if open}
  <!-- @-mention picker. Floats above the composer; arrow keys navigate,
       Enter / Tab picks, Esc dismisses. Candidates pulled from a cached
       entity index loaded on first @-trigger. -->
  <div
    role="listbox"
    class="absolute left-0 right-0 bottom-full mb-1 bg-mantle border border-surface1 rounded-lg shadow-xl z-30 max-h-64 overflow-y-auto"
  >
    {#if loading && candidates.length === 0}
      <div class="px-3 py-2 text-[11px] text-dim italic">Loading…</div>
    {:else if candidates.length === 0}
      <div class="px-3 py-2 text-[11px] text-dim italic">No matches for "{query}".</div>
    {:else}
      {#each candidates as c, i (c.kind + ':' + c.id)}
        <button
          type="button"
          role="option"
          aria-selected={i === selectedIdx}
          onmousedown={(e) => { e.preventDefault(); pick(c); }}
          onmouseenter={() => (selectedIdx = i)}
          class="w-full flex items-baseline gap-2 px-3 py-1.5 text-left hover:bg-surface0 {i === selectedIdx ? 'bg-surface0' : ''}"
        >
          <span class="text-[9px] uppercase tracking-wider text-secondary flex-shrink-0 w-12">{c.kind}</span>
          <span class="text-xs text-text truncate flex-1">{c.title}</span>
          <span class="text-[10px] text-dim truncate max-w-[40%]">{c.subtitle}</span>
        </button>
      {/each}
    {/if}
  </div>
{/if}
