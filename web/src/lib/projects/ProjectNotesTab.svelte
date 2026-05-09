<script lang="ts">
  import { api, todayISO, type Note, type Project } from '$lib/api';
  import { slugifyTitle } from '$lib/util/slug';
  import { toast } from '$lib/components/toast';
  import NoteLinkDialog from './NoteLinkDialog.svelte';

  // Notes linked to this project. Three matching signals, ordered by
  // strength so a note that asserts the link in frontmatter beats one
  // that just mentions the name in passing:
  //
  // 1. project: "Name" (or array containing Name) in frontmatter — strongest
  // 2. [[Name]] wikilink in body                                  — strong
  // 3. notePath includes folder OR project name segment           — fallback
  //
  // Capped to 40 results, sorted by recency. Each row shows match
  // reason as a badge so the user understands why a note surfaced.

  let { project }: { project: Project } = $props();

  let notes = $state<MatchedNote[]>([]);
  let loading = $state(false);
  let linkDialogOpen = $state(false);
  let creating = $state(false);

  type MatchReason = 'frontmatter' | 'wikilink' | 'path';
  type MatchedNote = Note & { reason: MatchReason };

  function ymd(d: Date): string {
    return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, '0')}-${String(d.getDate()).padStart(2, '0')}`;
  }

  function noteTitle(n: Note): string {
    if (n.title && n.title.trim() !== '') return n.title;
    const base = n.path.split('/').pop() ?? n.path;
    return base.replace(/\.md$/i, '');
  }

  function relativeDate(iso?: string): string {
    if (!iso) return '';
    const d = new Date(iso);
    if (Number.isNaN(d.getTime())) return '';
    const now = new Date();
    const diff = Math.floor((now.getTime() - d.getTime()) / 86400000);
    if (diff < 0) return ymd(d);
    if (diff === 0) return 'today';
    if (diff === 1) return 'yesterday';
    if (diff < 7) return `${diff}d ago`;
    if (diff < 30) return `${Math.floor(diff / 7)}w ago`;
    if (diff < 365) return `${Math.floor(diff / 30)}mo ago`;
    return ymd(d);
  }

  function noteFolder(n: Note): string {
    const parts = n.path.split('/');
    if (parts.length <= 1) return '';
    return parts.slice(0, -1).join('/');
  }

  function frontmatterHasProject(fm: Record<string, unknown> | undefined, name: string): boolean {
    if (!fm) return false;
    const v = fm.project ?? fm.Project ?? fm.projects ?? fm.Projects;
    if (typeof v === 'string') return v === name;
    if (Array.isArray(v)) return v.some((x) => typeof x === 'string' && x === name);
    return false;
  }

  function bodyHasWikilink(body: string | undefined, name: string): boolean {
    if (!body) return false;
    // [[Name]] or [[Name|alias]] — check both, escaped to handle names
    // with regex-special characters.
    const escaped = name.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
    return new RegExp(`\\[\\[${escaped}(?:\\|[^\\]]+)?\\]\\]`).test(body);
  }

  function pathMatches(n: Note, project: Project): boolean {
    const folder = (project.folder ?? '').replace(/\/$/, '');
    if (folder && (n.path === folder || n.path.startsWith(folder + '/'))) return true;
    // Last-resort heuristic — the project name appears as a path
    // segment. Avoids false positives from substring matches by
    // splitting on /.
    const segs = n.path.split('/');
    return segs.includes(project.name) || segs.some((s) => s.replace(/\.md$/i, '') === project.name);
  }

  function bodyExcerpt(n: Note): string {
    if (!n.body) return '';
    const lower = n.body.toLowerCase();
    const idx = lower.indexOf(project.name.toLowerCase());
    const start = idx >= 0 ? Math.max(0, idx - 60) : 0;
    return n.body.slice(start, start + 220).replace(/\s+/g, ' ').trim();
  }

  async function load() {
    loading = true;
    try {
      // Server full-text search seeds the candidate set — far cheaper
      // than scanning every note in the vault. We then post-filter
      // for the three match signals so a server hit in passing copy
      // doesn't surface a note that doesn't actually link to the
      // project. The server doesn't expose structured project-field
      // queries, so this client-side validation is the gate.
      const r = await api.listNotes({ q: project.name, limit: 60 });
      const out: MatchedNote[] = [];
      for (const n of r.notes) {
        let reason: MatchReason | null = null;
        if (frontmatterHasProject(n.frontmatter, project.name)) reason = 'frontmatter';
        else if (bodyHasWikilink(n.body, project.name)) reason = 'wikilink';
        else if (pathMatches(n, project)) reason = 'path';
        if (reason) out.push({ ...n, reason });
      }
      // If the project has a folder, we need a second pull to surface
      // notes whose body never mentions the project name but live
      // under the folder — those wouldn't show up in the q=name search.
      const folder = (project.folder ?? '').replace(/\/$/, '');
      if (folder) {
        try {
          const r2 = await api.listNotes({ folder, limit: 60 });
          for (const n of r2.notes) {
            if (out.some((x) => x.path === n.path)) continue;
            out.push({ ...n, reason: 'path' });
          }
        } catch {
          // Best-effort — if the folder query fails we still have
          // the q-based results.
        }
      }
      // Sort by recency (modTime desc), cap at 40 — beyond that the
      // tab is just noise.
      out.sort((a, b) => (b.modTime ?? '').localeCompare(a.modTime ?? ''));
      notes = out.slice(0, 40);
    } catch (e) {
      console.error('failed to load linked notes', e);
      toast.error('failed to load notes: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    void project.name;
    void project.folder;
    load();
  });

  // Match-reason badge tone — frontmatter is the user's explicit
  // assertion, so it gets the primary tone; wikilink is a body link,
  // secondary; path is the weakest signal, dim.
  function reasonTone(r: MatchReason): string {
    if (r === 'frontmatter') return 'primary';
    if (r === 'wikilink') return 'secondary';
    return 'subtext';
  }
  function reasonLabel(r: MatchReason): string {
    if (r === 'frontmatter') return 'frontmatter';
    if (r === 'wikilink') return 'wikilink';
    return 'path';
  }

  // ── + New note in this project ─────────────────────────────────────
  // Drops a fresh note into the project's folder (or vault root if
  // none) with project: "<name>" in frontmatter pre-filled, then
  // hands the user off to the editor. This is the "fast path" — the
  // alternative is creating in /notes and remembering to set the
  // project field.
  async function newNoteInProject() {
    if (creating) return;
    creating = true;
    try {
      const ts = todayISO();
      const slug = slugifyTitle(project.name);
      const folder = (project.folder ?? '').replace(/\/$/, '');
      const path = (folder ? `${folder}/` : '') + `${ts}-${slug || 'note'}.md`;
      const fm: Record<string, unknown> = { project: project.name };
      const body = `# Note for ${project.name}\n\n`;
      const created = await api.createNote({ path, frontmatter: fm, body });
      toast.success('note created');
      // Soft refresh the list so the new note shows up immediately;
      // navigating to /notes/<path> will mount the editor with the
      // frontmatter primed.
      void load();
      window.location.href = `/notes/${encodeURI(created.path)}`;
    } catch (e) {
      toast.error('create failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      creating = false;
    }
  }

  // ── + Link existing note ──────────────────────────────────────────
  // Search dialog (NoteLinkDialog component) returns a Note. We then
  // PUT it back with project: "<name>" merged into its frontmatter.
  // If the note already had a different project field we surface a
  // confirm so we don't silently clobber.
  async function linkExistingNote(n: Note) {
    try {
      const fm = { ...(n.frontmatter ?? {}) } as Record<string, unknown>;
      const existing = fm.project;
      if (typeof existing === 'string' && existing && existing !== project.name) {
        const ok = confirm(
          `"${noteTitle(n)}" is already linked to project "${existing}". Replace with "${project.name}"?`
        );
        if (!ok) return;
      } else if (Array.isArray(existing)) {
        // Append to array if it isn't already there — supports
        // notes that legitimately span multiple projects.
        if (!existing.includes(project.name)) {
          fm.project = [...existing, project.name];
        }
        await api.putNote(n.path, { frontmatter: fm, body: n.body ?? '' });
        toast.success(`linked "${noteTitle(n)}"`);
        linkDialogOpen = false;
        await load();
        return;
      }
      fm.project = project.name;
      await api.putNote(n.path, { frontmatter: fm, body: n.body ?? '' });
      toast.success(`linked "${noteTitle(n)}"`);
      linkDialogOpen = false;
      await load();
    } catch (e) {
      toast.error('link failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  // ── Unlink — clear the project frontmatter field on a linked note.
  // Only meaningful for frontmatter-matched notes; wikilink/path
  // matches require body-text edits we don't presume to make.
  async function unlinkNote(n: MatchedNote) {
    if (n.reason !== 'frontmatter') {
      toast.error('only frontmatter links can be unlinked here — open the note to edit body links');
      return;
    }
    if (!confirm(`Unlink "${noteTitle(n)}" from this project?`)) return;
    try {
      const fm = { ...(n.frontmatter ?? {}) } as Record<string, unknown>;
      const v = fm.project;
      if (Array.isArray(v)) {
        const next = v.filter((x) => x !== project.name);
        if (next.length === 0) delete fm.project;
        else fm.project = next;
      } else {
        delete fm.project;
      }
      await api.putNote(n.path, { frontmatter: fm, body: n.body ?? '' });
      toast.success('unlinked');
      await load();
    } catch (e) {
      toast.error('unlink failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }
</script>

<section>
  <div class="flex items-baseline gap-2 mb-2">
    <h3 class="text-xs uppercase tracking-wider text-dim font-medium flex-1">Notes · {notes.length}{notes.length === 40 ? '+' : ''}</h3>
    <button
      onclick={() => (linkDialogOpen = true)}
      class="text-[11px] px-2 py-0.5 rounded bg-surface0 border border-surface1 text-subtext hover:border-primary"
      title="search the vault and link an existing note to this project"
    >+ Link existing</button>
    <button
      onclick={() => void newNoteInProject()}
      disabled={creating}
      class="text-[11px] px-2 py-0.5 rounded bg-primary text-on-primary hover:opacity-90 disabled:opacity-50"
      title="create a fresh note with project frontmatter pre-filled"
    >{creating ? 'creating…' : '+ New note'}</button>
  </div>

  {#if loading && notes.length === 0}
    <div class="text-xs text-dim">scanning vault…</div>
  {:else if notes.length === 0}
    <div class="text-xs text-dim italic px-3 py-2 bg-surface0/50 border border-dashed border-surface1 rounded">
      No notes link to this project yet. A note links if it has
      <code class="text-secondary">project: "{project.name}"</code> in frontmatter,
      a <code class="text-secondary">[[{project.name}]]</code> wikilink in the body,
      or sits under <code class="text-secondary">{project.folder || '<this project\'s folder>'}</code>.
    </div>
  {:else}
    <ul class="space-y-1.5">
      {#each notes as n (n.path)}
        {@const tone = reasonTone(n.reason)}
        {@const folder = noteFolder(n)}
        {@const excerpt = bodyExcerpt(n)}
        <li class="group">
          <a
            href={`/notes/${encodeURI(n.path)}`}
            class="block px-3 py-2 bg-surface0 border border-surface1 rounded hover:border-primary/40 transition-colors"
          >
            <div class="flex items-baseline gap-2">
              <span class="text-sm font-medium text-text group-hover:text-primary truncate flex-1 min-w-0">{noteTitle(n)}</span>
              <span
                class="text-[9px] uppercase tracking-wider px-1.5 py-0.5 rounded flex-shrink-0"
                style="background: var(--color-{tone})/15; color: var(--color-{tone});"
                title="match reason: {reasonLabel(n.reason)}"
              >{reasonLabel(n.reason)}</span>
              {#if n.modTime}
                <span class="text-[10px] text-dim font-mono flex-shrink-0">{relativeDate(n.modTime)}</span>
              {/if}
            </div>
            {#if folder}
              <p class="text-[11px] text-dim font-mono truncate mt-0.5">{folder}/</p>
            {/if}
            {#if excerpt}
              <p class="text-xs text-subtext line-clamp-2 mt-1">…{excerpt}…</p>
            {/if}
          </a>
          {#if n.reason === 'frontmatter'}
            <!-- Unlink only offered for frontmatter matches — those are
                 the ones we can cleanly reverse. Body-link / path-match
                 unlinks would require body edits we shouldn't presume. -->
            <button
              onclick={() => void unlinkNote(n)}
              class="text-[10px] text-dim hover:text-error mt-0.5 ml-3"
              title="remove project frontmatter from this note"
            >unlink</button>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</section>

<NoteLinkDialog
  bind:open={linkDialogOpen}
  excludePaths={notes.map((n) => n.path)}
  onPick={linkExistingNote}
/>
