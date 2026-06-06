// Folder breadcrumbs for the note header. Each crumb carries its own
// folder-filter URL so a mid-path click goes "show me everything
// under <root>/a/b/" without recomputing the prefix in markup.
// Deeply-nested paths (5+ segments) collapse the middle into a
// clickable ellipsis so the bar stays one line.

export interface Crumb {
  label: string;
  href: string;
}

/** When the path has more than this many folder segments we collapse
 *  the middle ones into an ellipsis. Showing the first two + last
 *  keeps the most relevant context (top-level area + immediate
 *  parent) without truncating the title. */
const CRUMB_COLLAPSE_THRESHOLD = 4;

/** Folder crumbs for a note path. Last path segment (the filename)
 *  is intentionally excluded — the note's title already renders
 *  alongside the bar. */
export function noteCrumbs(notePath: string | undefined): Crumb[] {
  if (!notePath) return [];
  const segs = notePath.split('/').slice(0, -1);
  return segs.map((seg, i) => ({
    label: seg,
    href: `/notes?folder=${encodeURIComponent(segs.slice(0, i + 1).join('/'))}`
  }));
}

/** Apply the collapse rule. `expanded=true` always shows the full
 *  list; otherwise drops the middle when the path is deep. */
export function visibleCrumbs(all: Crumb[], expanded: boolean): Crumb[] {
  if (expanded) return all;
  if (all.length <= CRUMB_COLLAPSE_THRESHOLD) return all;
  return [...all.slice(0, 2), ...all.slice(-1)];
}

export function crumbsCollapsed(all: Crumb[], expanded: boolean): boolean {
  return !expanded && all.length > CRUMB_COLLAPSE_THRESHOLD;
}
