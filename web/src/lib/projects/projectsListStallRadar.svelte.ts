// Stalled-projects radar for the projects LIST page.
//
// Fourth extraction step out of routes/projects/+page.svelte. Scans
// active projects locally for stall signals — no completion in N days,
// project mtime older than the threshold — then asks the AI for a
// one-line "what could unblock this" suggestion per stalled project.
//
// The local heuristic is the floor (we never surface anything as
// stalled that is provably alive); the AI's job is the qualitative
// "why" and the unblock idea, not the detection itself. This split
// keeps the AI grounded in real data and means a flaky AI response
// still produces a usable dashboard (just without the unblock copy).
//
// One compact chatStream call carries all stalled projects bundled —
// N stalled projects = 1 AI call, not N. The model returns a JSON
// array we parse and zip back to the list. JSON failure falls back
// to showing the radar without unblock copy.
//
// Stop/Close split mirrors projectAIHealth: cancelRadar() aborts the
// in-flight stream but keeps the partial rows visible, while close()
// also drops the open flag. AbortError is filtered so the user-typed
// cancel doesn't surface as a toast/error.

import { api, todayISO, type Project, type Task } from '$lib/api';
import { toast } from '$lib/components/toast';
import { isAbortError } from '$lib/util/aiErrors';

export const STALL_DAYS = 14;

export interface StalledRow {
  name: string;
  color?: string;
  venture?: string;
  daysSinceCompletion: number | null;
  daysSinceUpdate: number | null;
  openTasks: number;
  overdueTasks: number;
  unblock?: string;
}

export interface ProjectsListStallRadarDeps {
  /** Live projects sidecar — read via getter so the controller picks
   *  up reactivity from projectsListData without owning it. */
  getProjects: () => Project[];
  /** Live tasks sidecar — used for stall signal extraction. */
  getTasks: () => Task[];
  /** Page-level reload after a successful archive — reconciles
   *  optimistic row drops with the server. */
  reload: () => Promise<void> | void;
}

export interface ProjectsListStallRadarController {
  /** Drawer flag — toggled by the sidebar button. */
  open: boolean;
  /** True while the AI call is in flight. Drives the cancel button. */
  readonly busy: boolean;
  /** One-liner error from the AI stream (or JSON parse). Empty when
   *  the radar is healthy. */
  readonly error: string;
  /** Current radar table — local stall list with AI unblock copy
   *  zipped in when the stream completes successfully. */
  readonly rows: StalledRow[];
  /** Local wall-clock HH:MM of the last runRadar() call. */
  readonly ranAt: string;

  /** Live local stall detection — independent of the AI so the
   *  drawer can render even when the AI is offline. */
  readonly stalledLocally: StalledRow[];

  /** Open the drawer (and kick off a scan if rows are empty). */
  openAndScan(): void;
  /** Run the local + AI scan. Safe to call while busy — it bails
   *  rather than queueing. */
  runRadar(): Promise<void>;
  /** Abort the in-flight AI stream. Keeps partial rows visible. */
  cancelRadar(): void;
  /** Close the drawer. Also cancels any in-flight stream so the
   *  hidden radar can't keep streaming into nowhere. */
  close(): void;

  /** Optimistically archive a project: drop the row, fire the PATCH,
   *  toast on success, reload the page to reconcile. Errors fall
   *  back to a toast and leave the radar state untouched (the next
   *  reload reconciles). */
  archiveProject(name: string): Promise<void>;
}

export function createProjectsListStallRadar(
  deps: ProjectsListStallRadarDeps
): ProjectsListStallRadarController {
  let open = $state(false);
  let busy = $state(false);
  let error = $state('');
  let rows = $state<StalledRow[]>([]);
  let ranAt = $state('');
  // AbortController is plain mutable state (not $state) because the
  // signal is consumed inside the await; we only need referential
  // identity for cancelRadar() to call .abort().
  let abort: AbortController | null = null;

  const stalledLocally = $derived.by<StalledRow[]>(() => {
    const projects = deps.getProjects();
    const tasks = deps.getTasks();
    const today = new Date();
    const out: StalledRow[] = [];
    for (const p of projects) {
      if ((p.status ?? 'active') !== 'active') continue;
      // Bucket this project's tasks (mirroring detail-panel match
      // logic: explicit projectId OR notePath under folder).
      const folder = (p.folder ?? '').replace(/\/$/, '');
      const matched = tasks.filter(
        (t) => t.projectId === p.name || (folder && t.notePath.startsWith(folder + '/'))
      );
      let lastCompletion: Date | null = null;
      let openCount = 0;
      let overdueCount = 0;
      for (const t of matched) {
        if (t.done && t.completedAt) {
          const d = new Date(t.completedAt);
          if (!Number.isNaN(d.getTime()) && (!lastCompletion || d > lastCompletion)) lastCompletion = d;
        }
        if (!t.done) {
          openCount++;
          if (t.dueDate) {
            const d = new Date(t.dueDate);
            if (!Number.isNaN(d.getTime()) && d.getTime() < today.getTime()) overdueCount++;
          }
        }
      }
      const daysSinceCompletion = lastCompletion
        ? Math.floor((today.getTime() - lastCompletion.getTime()) / 86400000)
        : null;
      const updatedAt = p.updated_at ? new Date(p.updated_at) : null;
      const daysSinceUpdate = updatedAt && !Number.isNaN(updatedAt.getTime())
        ? Math.floor((today.getTime() - updatedAt.getTime()) / 86400000)
        : null;

      // Stall criteria: an active project with EITHER no completions
      // in STALL_DAYS days (incl. never), OR no edits in STALL_DAYS
      // days. A project with overdue tasks but recent activity is
      // not "stalled" — it's just busy and behind. We require BOTH
      // signals to age out before flagging, otherwise a project
      // that just got created shows up as stalled (no completions
      // yet) which is noise.
      const completionStalled =
        daysSinceCompletion === null || daysSinceCompletion >= STALL_DAYS;
      const updateStalled =
        daysSinceUpdate === null || daysSinceUpdate >= STALL_DAYS;
      // Special case: a project with zero tasks at all is dead in
      // a different sense — surface it too.
      const empty = matched.length === 0;
      if ((completionStalled && updateStalled) || empty) {
        out.push({
          name: p.name,
          color: p.color,
          venture: p.venture,
          daysSinceCompletion,
          daysSinceUpdate,
          openTasks: openCount,
          overdueTasks: overdueCount
        });
      }
    }
    // Sort: most-stalled first (highest daysSinceCompletion, then
    // daysSinceUpdate). null = never-completed = sort to top.
    return out.sort((a, b) => {
      const ad = a.daysSinceCompletion ?? 9999;
      const bd = b.daysSinceCompletion ?? 9999;
      if (ad !== bd) return bd - ad;
      const au = a.daysSinceUpdate ?? 9999;
      const bu = b.daysSinceUpdate ?? 9999;
      return bu - au;
    });
  });

  async function runRadar() {
    if (busy) return;
    const stalled = stalledLocally;
    busy = true;
    error = '';
    rows = stalled.map((r) => ({ ...r })); // render rows immediately, AI fills in unblock
    abort = new AbortController();
    // Local wall-clock HH:MM. toISOString() returns UTC which would
    // show the wrong time for any user not in UTC.
    ranAt = new Date().toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', hour12: false });

    if (stalled.length === 0) {
      busy = false;
      abort = null;
      return;
    }

    // One compact JSON payload — the model gets the project name +
    // signals and returns a parallel array of unblock suggestions.
    // No verbose "tell me about each project" loop; one prompt, one
    // response, N suggestions.
    const payload = stalled.map((r) => ({
      name: r.name,
      days_since_completion: r.daysSinceCompletion,
      days_since_update: r.daysSinceUpdate,
      open_tasks: r.openTasks,
      overdue_tasks: r.overdueTasks
    }));

    const system =
      'You diagnose stalled projects. For each project you receive, return ONE unblock suggestion in <= 14 words. ' +
      'Output STRICT JSON only — no preamble, no fence, no commentary. Schema:\n' +
      '{ "unblocks": [ { "name": string, "unblock": string }, ... ] }\n\n' +
      'Rules:\n' +
      '- The "name" MUST exactly match the input project name.\n' +
      '- Each "unblock" is a verb-led concrete suggestion the user could try this week.\n' +
      '- If a project has 0 tasks, suggest "write down what done looks like, or archive it".\n' +
      '- If a project has many overdue tasks, suggest a 30-min triage / reschedule pass.\n' +
      '- If days_since_completion is null and days_since_update is high, the project may be dead — suggest archiving.\n' +
      '- No corporate sludge: no "synergy", "leverage", "circle back", "let\'s align".\n' +
      '- Never invent details (no fake names, no fake deadlines). You only know what is in the input.';

    const user =
      `Today is ${todayISO()}. Stalled projects:\n\n` +
      '```json\n' +
      JSON.stringify(payload, null, 2) +
      '\n```\n\n' +
      'Return the JSON object with one unblock per project.';

    let buf = '';
    try {
      await api.chatStream(
        [
          { role: 'system', content: system },
          { role: 'user', content: user }
        ],
        undefined,
        {
          onChunk: (c) => {
            buf += c;
          },
          onError: (err) => {
            if (isAbortError(err)) return;
            error = err.message;
          }
        },
        abort.signal
      );
      const trimmed = buf.trim();
      if (trimmed) {
        try {
          const cleaned = trimmed.replace(/^```(?:json)?\s*/i, '').replace(/\s*```$/i, '');
          const parsed = JSON.parse(cleaned) as { unblocks?: { name: string; unblock: string }[] };
          if (parsed && Array.isArray(parsed.unblocks)) {
            const byName = new Map(parsed.unblocks.map((u) => [u.name, u.unblock]));
            rows = rows.map((r) => ({ ...r, unblock: byName.get(r.name) }));
          } else {
            error = 'AI returned unexpected shape — radar shown without unblock copy.';
          }
        } catch {
          error = 'AI did not return valid JSON — radar shown without unblock copy.';
        }
      }
    } catch (err) {
      // Cancel-driven throws also land here on some engines (the
      // signal aborts mid-await before onError fires); filter them
      // so the user-typed cancel doesn't surface as an error.
      if (!isAbortError(err)) {
        error = err instanceof Error ? err.message : String(err);
      }
    } finally {
      busy = false;
      abort = null;
    }
  }

  // Stop — abort + flip busy + null abort synchronously so the
  // "Stop" button swaps back to "Rerun" instantly. Without these
  // the UI lags until chatStream's finally settles (which can
  // take a tick when the abort fires mid-await). Same shape as
  // projectAIHealth/projectAIBrief/etc.
  function cancelRadar() {
    abort?.abort();
    abort = null;
    busy = false;
  }

  function openAndScan() {
    open = true;
    if (rows.length === 0 && !busy) void runRadar();
  }

  function close() {
    open = false;
    // Hidden radar shouldn't keep streaming into nowhere.
    abort?.abort();
  }

  async function archiveProject(name: string) {
    if (!confirm(`Archive "${name}"? It stays in the vault, just out of the active list.`)) return;
    try {
      await api.patchProject(name, { status: 'archived' });
      // Drop the row optimistically; the reload below reconciles.
      rows = rows.filter((r) => r.name !== name);
      await deps.reload();
      toast.success(`archived "${name}"`);
    } catch (e) {
      toast.error('archive failed: ' + (e instanceof Error ? e.message : String(e)));
    }
  }

  return {
    get open() {
      return open;
    },
    set open(v) {
      open = v;
    },
    get busy() {
      return busy;
    },
    get error() {
      return error;
    },
    get rows() {
      return rows;
    },
    get ranAt() {
      return ranAt;
    },
    get stalledLocally() {
      return stalledLocally;
    },
    openAndScan,
    runRadar,
    cancelRadar,
    close,
    archiveProject
  };
}
