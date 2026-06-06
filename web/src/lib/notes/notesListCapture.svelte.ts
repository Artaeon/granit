// Quick-capture dialog state for the notes list surface.
//
// Third extraction step out of routes/notes/+page.svelte. Owns the
// three-mode capture flow (capture textarea → AI-staged review →
// manual fallback), the streaming AI hint state, every staging field
// (title / tags / folder / wikilink toggles), the manual-mode legacy
// fields, every action (open / close / runAi / fast-save / save
// staged / manual create + add/remove tag + toggle wikilink), the
// capture-dialog keyboard handler, and the pure helpers (slugify /
// sanitizeFolder / parseAiJson / sampleTitles).
//
// The page still owns the onMount install of:
//   - ⌘N / Ctrl+N keyboard shortcut    → calls openCapture()
//   - /notes?capture=1 deep-link       → calls openCapture()
//   - Web Share Target intake          → calls openCapture() + sets captureText
//   - the abort-on-unmount cleanup     → calls dispose()
// All of these are page-side because they're URL / window plumbing,
// not dialog state — same shape every other route follows.

import { goto } from '$app/navigation';
import { api, type Note } from '$lib/api';
import { toast } from '$lib/components/toast';
import { rafThrottle } from '$lib/util/streamThrottle';
import { isAbortError } from '$lib/util/aiErrors';

export type CaptureMode = 'capture' | 'staging' | 'manual';

export interface AiCapture {
  title: string;
  tags: string[];
  folder: string;
  wikilinkCandidates: string[];
}

export interface NotesListCaptureDeps {
  /** Live read of the loaded notes — sampleTitles draws a reservoir
   *  for the AI prompt, and the AI response is filtered against the
   *  current titles before staging wikilink toggles. */
  getNotes: () => Note[];
  /** Live read of the top-tag list (capped at 30) used to hint the
   *  AI so it prefers existing tag names. */
  getTopTags: () => string[];
}

export interface NotesListCaptureController {
  // Bindable state.
  createOpen: boolean;
  captureMode: CaptureMode;
  captureText: string;
  captureBusy: boolean;
  captureRaw: string;

  stageTitle: string;
  stageTags: string[];
  stageTagInput: string;
  stageFolder: string;
  stageWikilinkCandidates: string[];
  stageWikilinksChosen: Set<string>;

  createTitle: string;
  createFolder: string;
  creating: boolean;

  openCapture(): void;
  closeCapture(): void;
  runAiCapture(): Promise<void>;
  fastCapture(): Promise<void>;
  saveStaged(): Promise<void>;
  manualCreate(): Promise<void>;
  addStageTag(): void;
  removeStageTag(t: string): void;
  toggleWikilink(title: string): void;
  onCaptureKey(e: KeyboardEvent): void;
  /** Abort any in-flight AI stream on page unmount. */
  dispose(): void;
}

function slugify(s: string): string {
  return s.replace(/[^\w\s-]/g, '').trim().replace(/\s+/g, '-') || 'untitled';
}
function sanitizeFolder(s: string): string {
  return s.trim().replace(/^\/+|\/+$/g, '');
}

function parseAiJson(raw: string): AiCapture | null {
  let cleaned = raw.trim();
  if (cleaned.startsWith('```')) {
    cleaned = cleaned.replace(/^```json\s*/i, '').replace(/^```\s*/, '').replace(/```\s*$/, '').trim();
  }
  // Some models wrap with extra prose — try to grab the first
  // {...} block as a fallback.
  if (!cleaned.startsWith('{')) {
    const m = cleaned.match(/\{[\s\S]*\}/);
    if (m) cleaned = m[0];
  }
  try {
    const parsed = JSON.parse(cleaned) as Record<string, unknown>;
    const title = typeof parsed.title === 'string' ? parsed.title.trim() : '';
    const folder = typeof parsed.folder === 'string' ? parsed.folder.trim() : '';
    const tagsRaw = Array.isArray(parsed.tags) ? parsed.tags : [];
    const tags = tagsRaw
      .filter((t): t is string => typeof t === 'string' && !!t.trim())
      .map((t) => t.trim().replace(/^#/, ''));
    const wlRaw = Array.isArray(parsed.wikilinkCandidates) ? parsed.wikilinkCandidates : [];
    const wl = wlRaw.filter((t): t is string => typeof t === 'string' && !!t.trim()).map((t) => t.trim());
    if (!title) return null;
    return { title, tags, folder: folder || 'Inbox', wikilinkCandidates: wl };
  } catch {
    return null;
  }
}

export function createNotesListCapture(
  deps: NotesListCaptureDeps
): NotesListCaptureController {
  let createOpen = $state(false);
  let captureMode = $state<CaptureMode>('capture');
  let captureText = $state('');
  let captureBusy = $state(false);
  let captureAbort: AbortController | null = null;
  let captureRaw = $state(''); // streaming token buffer (for the user's "AI is thinking…" hint)

  // Staging fields (populated from AI JSON, then editable).
  let stageTitle = $state('');
  let stageTags = $state<string[]>([]);
  let stageTagInput = $state('');
  let stageFolder = $state('Inbox');
  let stageWikilinkCandidates = $state<string[]>([]);
  let stageWikilinksChosen = $state<Set<string>>(new Set());

  // Manual-mode fields (kept around for the "Skip AI" fallback).
  let createTitle = $state('');
  let createFolder = $state('');
  let creating = $state(false);

  function openCapture() {
    captureAbort?.abort();
    captureAbort = null;
    captureMode = 'capture';
    captureText = '';
    captureBusy = false;
    captureRaw = '';
    stageTitle = '';
    stageTags = [];
    stageTagInput = '';
    stageFolder = 'Inbox';
    stageWikilinkCandidates = [];
    stageWikilinksChosen = new Set();
    createTitle = '';
    createFolder = '';
    creating = false;
    createOpen = true;
  }
  function closeCapture() {
    captureAbort?.abort();
    captureAbort = null;
    captureBusy = false;
    creating = false;
    createOpen = false;
  }

  // The AI prompt is built fresh on each capture so the existing-tag
  // list and sampled titles reflect the *current* vault. Sampling: we
  // take a random slice of titles so the AI sees variety without us
  // shipping the entire vault in the prompt — the prompt is bounded
  // even on huge vaults.
  function sampleTitles(max: number): string[] {
    const notes = deps.getNotes();
    if (notes.length <= max) return notes.map((n) => n.title);
    // Reservoir-style — cheap, no shuffle of the whole array.
    const out: string[] = [];
    for (let i = 0; i < notes.length; i++) {
      if (out.length < max) out.push(notes[i].title);
      else {
        const j = Math.floor(Math.random() * (i + 1));
        if (j < max) out[j] = notes[i].title;
      }
    }
    return out;
  }

  async function runAiCapture() {
    const text = captureText.trim();
    if (!text || captureBusy) return;
    captureBusy = true;
    captureRaw = '';
    captureAbort = new AbortController();
    // rAF coalescer — keeps "AI is thinking…" hint fluid without
    // spamming reactive writes.
    const throttle = rafThrottle((acc) => { captureRaw = acc; });

    const topTags = deps.getTopTags();
    const tagsHint = topTags.length > 0 ? topTags.join(', ') : '(none yet)';
    const titlesHint = sampleTitles(40).join('\n');
    const system =
      'You read a freeform note capture from the user and produce a STRICT JSON ' +
      'object (no fences, no prose) of the shape: ' +
      '{"title": "<short, human-readable title — Title Case, no trailing punctuation, no filename>", ' +
      '"tags": ["tag1", "tag2"], ' +
      '"folder": "<folder path inside the vault, default \\"Inbox\\" if unsure>", ' +
      '"wikilinkCandidates": ["Existing Note Title", "..."]}. ' +
      'Prefer reusing tags from this list of existing tags (lowercase, no leading #): ' + tagsHint + '. ' +
      'wikilinkCandidates MUST be picked verbatim from this sample of existing note titles ' +
      '(omit the field or use [] if none seem related): \n' + titlesHint;
    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: text }
        ],
        undefined,
        {
          onChunk: throttle.onChunk,
          onDone: () => {
            throttle.flush();
            const parsed = parseAiJson(throttle.value());
            if (!parsed) {
              toast.error('AI returned bad JSON — switching to manual.');
              // Pre-fill the manual title with the first line so the
              // user doesn't lose the typed body.
              const first = text.split(/\n/)[0]?.trim() ?? '';
              createTitle = first.slice(0, 80);
              captureMode = 'manual';
              return;
            }
            stageTitle = parsed.title;
            stageTags = parsed.tags;
            stageFolder = parsed.folder || 'Inbox';
            const notes = deps.getNotes();
            stageWikilinkCandidates = parsed.wikilinkCandidates.filter(
              (t) => notes.some((n) => n.title === t)
            );
            stageWikilinksChosen = new Set();
            captureMode = 'staging';
          },
          onError: (err) => {
            throttle.flush();
            // AbortError = user clicked Cancel on the capture stream.
            // Don't toast + don't force-flip to manual; the dismiss
            // path owns the wipe.
            if (isAbortError(err)) return;
            toast.error('AI failed: ' + err.message + ' — switching to manual.');
            const first = text.split(/\n/)[0]?.trim() ?? '';
            createTitle = first.slice(0, 80);
            captureMode = 'manual';
          }
        },
        captureAbort.signal
      );
    } finally {
      captureBusy = false;
      captureAbort = null;
    }
  }

  // ⌘Enter without waiting for AI — drop the captured text straight
  // into Inbox/{first-line}.md with empty frontmatter. The user did
  // not gate this on AI; we honour the intent.
  async function fastCapture() {
    const text = captureText.trim();
    if (!text || creating) return;
    const first = text.split(/\n/)[0]?.trim() || 'untitled';
    const title = first.slice(0, 80);
    const path = `Inbox/${slugify(title)}.md`;
    creating = true;
    try {
      await api.createNote({ path, frontmatter: {}, body: text });
      closeCapture();
      goto(`/notes/${encodeURIComponent(path)}`);
    } catch (e) {
      toast.error('create failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      creating = false;
    }
  }

  function addStageTag() {
    const t = stageTagInput.trim().replace(/^#/, '');
    if (!t) return;
    if (!stageTags.includes(t)) stageTags = [...stageTags, t];
    stageTagInput = '';
  }
  function removeStageTag(t: string) {
    stageTags = stageTags.filter((x) => x !== t);
  }
  function toggleWikilink(title: string) {
    const next = new Set(stageWikilinksChosen);
    if (next.has(title)) next.delete(title);
    else next.add(title);
    stageWikilinksChosen = next;
  }

  async function saveStaged() {
    const title = stageTitle.trim();
    if (!title || creating) return;
    const folder = sanitizeFolder(stageFolder || 'Inbox');
    const path = (folder ? folder + '/' : '') + slugify(title) + '.md';
    let body = captureText.trim();
    if (stageWikilinksChosen.size > 0) {
      const links = [...stageWikilinksChosen].map((t) => `[[${t}]]`).join(', ');
      body = body + '\n\nRelated: ' + links + '\n';
    }
    const frontmatter: Record<string, unknown> = {};
    if (stageTags.length > 0) frontmatter.tags = stageTags;
    creating = true;
    try {
      await api.createNote({ path, frontmatter, body });
      closeCapture();
      goto(`/notes/${encodeURIComponent(path)}`);
    } catch (e) {
      toast.error('create failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      creating = false;
    }
  }

  // Manual-mode fallback (legacy two-field create). Kept for the user
  // who deliberately bypasses the AI flow with "Skip AI".
  async function manualCreate() {
    const title = createTitle.trim();
    if (!title) return;
    creating = true;
    try {
      const folder = sanitizeFolder(createFolder);
      const path = (folder ? folder + '/' : '') + slugify(title) + '.md';
      const body = captureText.trim() || `# ${title}\n\n`;
      await api.createNote({ path, body });
      closeCapture();
      goto(`/notes/${encodeURIComponent(path)}`);
    } catch (e) {
      toast.error('create failed: ' + (e instanceof Error ? e.message : String(e)));
    } finally {
      creating = false;
    }
  }

  // Capture dialog keyboard handler.
  function onCaptureKey(e: KeyboardEvent) {
    if (e.key === 'Escape') {
      e.preventDefault();
      closeCapture();
      return;
    }
    if ((e.metaKey || e.ctrlKey) && e.key === 'Enter') {
      e.preventDefault();
      if (captureMode === 'capture') {
        if (!captureBusy && captureText.trim()) fastCapture();
      } else if (captureMode === 'staging') {
        saveStaged();
      } else if (captureMode === 'manual') {
        manualCreate();
      }
    }
  }

  function dispose() {
    captureAbort?.abort();
    captureAbort = null;
  }

  return {
    get createOpen() { return createOpen; },
    set createOpen(v) { createOpen = v; },
    get captureMode() { return captureMode; },
    set captureMode(v) { captureMode = v; },
    get captureText() { return captureText; },
    set captureText(v) { captureText = v; },
    get captureBusy() { return captureBusy; },
    set captureBusy(v) { captureBusy = v; },
    get captureRaw() { return captureRaw; },
    set captureRaw(v) { captureRaw = v; },
    get stageTitle() { return stageTitle; },
    set stageTitle(v) { stageTitle = v; },
    get stageTags() { return stageTags; },
    set stageTags(v) { stageTags = v; },
    get stageTagInput() { return stageTagInput; },
    set stageTagInput(v) { stageTagInput = v; },
    get stageFolder() { return stageFolder; },
    set stageFolder(v) { stageFolder = v; },
    get stageWikilinkCandidates() { return stageWikilinkCandidates; },
    set stageWikilinkCandidates(v) { stageWikilinkCandidates = v; },
    get stageWikilinksChosen() { return stageWikilinksChosen; },
    set stageWikilinksChosen(v) { stageWikilinksChosen = v; },
    get createTitle() { return createTitle; },
    set createTitle(v) { createTitle = v; },
    get createFolder() { return createFolder; },
    set createFolder(v) { createFolder = v; },
    get creating() { return creating; },
    set creating(v) { creating = v; },
    openCapture,
    closeCapture,
    runAiCapture,
    fastCapture,
    saveStaged,
    manualCreate,
    addStageTag,
    removeStageTag,
    toggleWikilink,
    onCaptureKey,
    dispose
  };
}
