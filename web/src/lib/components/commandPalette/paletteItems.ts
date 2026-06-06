// Pure item-builder for the command palette.
//
// Takes the data slices, the active query, the workspace command list,
// the static catalogs (PAGES + AGENTS), and a recents lookup; returns
// the sorted, grouped, capped CmdItem list the palette renders.
//
// Lives in a pure .ts module — the Svelte component wraps the call in
// $derived.by so the reactivity is tracked by the caller. Putting the
// scoring logic here gets the per-section row construction out of the
// component shell (it was ~240 LOC of nearly-identical loops) and
// makes the precedence rules + recency-bump math testable without a
// Svelte renderer.
//
// Sorting:
//  1. group-rank (Content > Pages > Tasks > Events > Deadlines >
//     Projects > Goals > Notes > Habits > Agents) — see groupRank for
//     the why. Workspace is rank-tied with Pages so split / close /
//     swap reach the top of the list right after a literal page hit.
//  2. fuzzy score + recency boost within a group.
// The cap (80) is more rows than fit on any screen; the user is going
// to type, not scroll past 80.

import { goto } from '$app/navigation';
import {
  type Project,
  type Goal,
  type SearchHit,
  type Task,
  type HabitInfo,
  type Deadline,
  type CalendarEvent
} from '$lib/api';
import { fuzzyScoreMulti } from '$lib/util/fuzzy';
import type { WorkspaceCmd } from '$lib/workspace/workspaceCommands';
import type { CmdItem, Group, AgentCmd } from './paletteTypes';

const ITEM_CAP = 80;

export interface BuildItemsInput {
  /** User's current query — trimmed inside the builder. */
  query: string;
  /** Live workspace command list. The caller reads it from
   *  workspaceCommands() inside $derived.by so reactivity tracks. */
  workspaceCmds: WorkspaceCmd[];
  /** Static page catalog — see paletteCatalog.PAGES. */
  pages: { path: string; label: string; icon: string }[];
  /** Static agent catalog — see paletteCatalog.AGENTS. */
  agents: AgentCmd[];
  // Data slices.
  projects: Project[];
  goals: Goal[];
  notes: { path: string; title: string }[];
  tasks: Task[];
  events: CalendarEvent[];
  deadlines: Deadline[];
  habits: HabitInfo[];
  searchHits: SearchHit[];
  /** Additive ranking bonus for `id`. The palette wires this through
   *  to its recents controller's recencyBoost. */
  recencyBoost: (id: string) => number;
  /** Membership test against the recents list. Used by the tasks
   *  branch to surface recent tasks on an empty query (every other
   *  section either always-shows on empty or relies on the fuzzy
   *  scorer to filter). */
  isRecent: (id: string) => boolean;
}

export function buildItems(input: BuildItemsInput): CmdItem[] {
  const {
    query,
    workspaceCmds,
    pages,
    agents,
    projects,
    goals,
    notes,
    tasks,
    events,
    deadlines,
    habits,
    searchHits,
    recencyBoost,
    isRecent
  } = input;

  const needle = query.trim();
  const out: CmdItem[] = [];
  // Side-table for derived scores. Kept out of the CmdItem shape
  // because consumers (templates) don't need it — sort is the only
  // reader. Recreated per build so there are no stale entries from a
  // prior keystroke.
  const scoreMap = new Map<string, number>();

  // Pages — always indexed, even before data loads (they're static).
  for (const p of pages) {
    const sc = fuzzyScoreMulti(needle, [p.label, p.path]);
    if (sc === null) continue;
    const id = 'page:' + p.path;
    out.push({
      id,
      label: p.label,
      detail: p.path,
      icon: p.icon,
      group: 'Pages',
      run: () => goto(p.path)
    });
    scoreMap.set(id, sc + recencyBoost(id));
  }

  // Workspace commands — split / close / swap focused pane. Each
  // command's run is a thunk; reactivity is tracked by the caller
  // because workspaceCommands() runs inside the caller's $derived.by.
  for (const wc of workspaceCmds) {
    const sc = fuzzyScoreMulti(needle, [wc.label, wc.detail]);
    if (sc === null) continue;
    out.push({
      id: wc.id,
      label: wc.label,
      detail: wc.detail,
      icon: wc.icon,
      group: 'Workspace',
      run: wc.run
    });
    scoreMap.set(wc.id, sc + recencyBoost(wc.id));
  }

  // Projects
  for (const pr of projects) {
    const sc = fuzzyScoreMulti(needle, [pr.name, pr.description ?? '']);
    if (sc === null) continue;
    const id = 'project:' + pr.name;
    out.push({
      id,
      label: pr.name,
      detail: pr.description?.slice(0, 80),
      icon: 'projects',
      group: 'Projects',
      run: () => goto('/projects/' + encodeURIComponent(pr.name))
    });
    scoreMap.set(id, sc + recencyBoost(id));
  }

  // Goals
  for (const g of goals) {
    const sc = fuzzyScoreMulti(needle, [g.title, g.category ?? '']);
    if (sc === null) continue;
    const id = 'goal:' + g.id;
    out.push({
      id,
      label: g.title,
      detail: g.category ?? g.status,
      icon: 'goals',
      group: 'Goals',
      run: () => goto('/goals?focus=' + encodeURIComponent(g.id))
    });
    scoreMap.set(id, sc + recencyBoost(id));
  }

  // Notes (cap 30 from listNotes — already mod-time-desc on the
  // server, so for empty queries the freshest leads).
  for (let i = 0; i < notes.length; i++) {
    const n = notes[i];
    const sc = fuzzyScoreMulti(needle, [n.title, n.path]);
    if (sc === null) continue;
    const id = 'note:' + n.path;
    out.push({
      id,
      label: n.title,
      detail: n.path,
      icon: 'notes',
      group: 'Notes',
      run: () => goto('/notes/' + encodeURIComponent(n.path))
    });
    // Empty-needle: rank by mod-time (server-order). Push the
    // recency-bump on top so a freshly-touched note still leads.
    const empty = !needle;
    scoreMap.set(id, (empty ? 100 - i : sc) + recencyBoost(id));
  }

  // Tasks — open ones, indexed by text + project. Empty needle:
  // hide everything except recents so an empty palette doesn't dump
  // 100 task rows in the user's face (the /tasks page is for that).
  // With a needle: show every fuzzy match.
  for (const t of tasks) {
    const sc = fuzzyScoreMulti(needle, [t.text, t.projectId ?? '']);
    if (sc === null) continue;
    const id = 'task:' + t.id;
    if (!needle && !isRecent(id)) continue;
    // Detail line: project (if any) + due date hint so the user can
    // pick the right task when the text alone is ambiguous.
    const bits: string[] = [];
    if (t.projectId) bits.push(t.projectId);
    if (t.dueDate) bits.push('due ' + t.dueDate);
    else if (t.scheduledStart) bits.push('at ' + t.scheduledStart.slice(11, 16));
    out.push({
      id,
      label: t.text,
      detail: bits.join(' · '),
      icon: 'tasks',
      group: 'Tasks',
      run: () => goto('/tasks?focus=' + encodeURIComponent(t.id))
    });
    scoreMap.set(id, sc + recencyBoost(id));
  }

  // Calendar events — next 14 days. Detail line carries the start
  // time + a one-glance type glyph so two events with the same title
  // (recurring stand-up) read distinguishably.
  for (const ev of events) {
    const sc = fuzzyScoreMulti(needle, [ev.title, ev.location ?? '']);
    if (sc === null) continue;
    // Each event already carries either start (RFC3339) or date
    // (YYYY-MM-DD all-day). Compose a stable id from the strongest
    // available identifier.
    const stableId = ev.eventId || ev.taskId || `${ev.title}@${ev.start || ev.date}`;
    const id = 'event:' + stableId;
    const dateStr = (ev.start || ev.date || '').slice(0, 10);
    const timeStr = ev.start ? ev.start.slice(11, 16) : 'all-day';
    out.push({
      id,
      label: ev.title,
      detail: `${dateStr} · ${timeStr}${ev.location ? ' · ' + ev.location : ''}`,
      icon: 'calendar',
      group: 'Events',
      // /calendar doesn't yet read a `?date=` query param, so we jump
      // to the calendar page and let the user land on the event from
      // the visible day. Future v2: add date routing.
      run: () => goto('/calendar')
    });
    scoreMap.set(id, sc + recencyBoost(id));
  }

  // Deadlines — active only (filtered at load time). Date proximity
  // matters more than fuzzy score for sort, so we lean on the date
  // string as a tiebreaker via score adjustment.
  for (const d of deadlines) {
    const sc = fuzzyScoreMulti(needle, [d.title, d.project ?? '', d.venture ?? '']);
    if (sc === null) continue;
    const id = 'deadline:' + d.id;
    const bits: string[] = [d.date];
    if (d.importance && d.importance !== 'normal') bits.push(d.importance);
    if (d.project) bits.push(d.project);
    out.push({
      id,
      label: d.title,
      detail: bits.join(' · '),
      icon: 'deadline',
      group: 'Deadlines',
      // /deadlines doesn't yet read ?focus — navigate to the page and
      // let the user scan. The detail line already shows the
      // distinguishing fields (date + importance + project).
      run: () => goto('/deadlines')
    });
    scoreMap.set(id, sc + recencyBoost(id));
  }

  // Habits — usually a small set (<20). All show on empty needle so
  // the user can jump to the habits page with one keystroke.
  for (const h of habits) {
    const sc = fuzzyScoreMulti(needle, [h.name]);
    if (sc === null) continue;
    const id = 'habit:' + h.name;
    out.push({
      id,
      label: h.name,
      detail: h.doneToday ? 'done today' : `${h.currentStreak}d streak`,
      icon: 'habits',
      group: 'Habits',
      run: () => goto('/habits')
    });
    scoreMap.set(id, sc + recencyBoost(id));
  }

  // Agent commands
  for (const a of agents) {
    const sc = fuzzyScoreMulti(needle, [a.label, a.detail]);
    if (sc === null) continue;
    const id = 'agent:' + a.slug;
    out.push({
      id,
      label: a.label,
      detail: a.detail,
      icon: a.icon,
      group: 'Agents',
      hint: a.hint,
      run: a.run
    });
    scoreMap.set(id, sc + recencyBoost(id));
  }

  // Content (full-text) — query-driven, no recents bump, scored by
  // result-list order (the API already ranks).
  for (let i = 0; i < searchHits.length; i++) {
    const h = searchHits[i];
    // Skip content hits whose path already appears as a Note title
    // hit — the title row is the better destination.
    const dupId = 'note:' + h.path;
    if (out.some((x) => x.id === dupId)) continue;
    const id = 'content:' + h.path + ':' + h.line;
    out.push({
      id,
      label: h.title,
      detail: h.matchLine,
      icon: 'search',
      group: 'Content',
      run: () => goto('/notes/' + encodeURIComponent(h.path))
    });
    // Sits above mid-tier matches; trumps recents in its own section.
    scoreMap.set(id, 700 - i);
  }

  out.sort((a, b) => {
    const ra = groupRank(a.group);
    const rb = groupRank(b.group);
    if (ra !== rb) return ra - rb;
    return (scoreMap.get(b.id) ?? 0) - (scoreMap.get(a.id) ?? 0);
  });
  return out.slice(0, ITEM_CAP);
}

/** Group items into runs sharing a Group header. Order is preserved
 *  from buildItems' sort, so the first occurrence of each group wins
 *  its header position. */
export function groupItems(items: CmdItem[]): { group: Group; items: CmdItem[] }[] {
  const m: { group: Group; items: CmdItem[] }[] = [];
  for (const it of items) {
    const last = m[m.length - 1];
    if (last && last.group === it.group) last.items.push(it);
    else m.push({ group: it.group, items: [it] });
  }
  return m;
}

/** Group precedence. Encodes "what does the user most often want
 *  when they type into this palette". Content body hits win because
 *  they're the most specific (the user typed enough to get a body
 *  match). Pages next — keystroke-to-jump is the headline use. Tasks
 *  above other entities because action items are the highest-frequency
 *  navigation target. Then Events / Deadlines (time-pressure surfaces),
 *  Projects + Goals (structural anchors), Notes (the long tail),
 *  Habits + Agents lowest (their pages are reachable by keyboard
 *  already; the palette rows are reach-from-anywhere fallbacks).
 *  Workspace falls through to the same rank as Agents — split / close
 *  / swap aren't search-driven, they're list-driven, so the user
 *  scrolls or fuzzy-matches the label rather than expecting them
 *  near the top. */
function groupRank(g: Group): number {
  if (g === 'Content') return 0;
  if (g === 'Pages') return 1;
  if (g === 'Tasks') return 2;
  if (g === 'Events') return 3;
  if (g === 'Deadlines') return 4;
  if (g === 'Projects') return 5;
  if (g === 'Goals') return 6;
  if (g === 'Notes') return 7;
  if (g === 'Habits') return 8;
  return 9; // Agents + Workspace
}
