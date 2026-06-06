// Goal detail-drawer state for the goals surface.
//
//   • selectedId / detailOpen — what the GoalDetail drawer renders.
//     selected is a $derived on dataCtl.goals + selectedId so a live
//     edit during a refetch finds the new copy without snapping the
//     drawer to a stale state.
//   • openDetail(g) also publishes to the workspace context bus so
//     an adjacent AI pane can surface the goal as context.
//
// Dashboard overlay (open/close + URL nav) stays in the parent because
// it's $page-driven and uses goto(); the parent reads selectedId via
// the getter to compose its dashboard URL.

import { workspaceContext } from '$lib/workspace/workspaceContext.svelte';
import type { Goal } from '$lib/api';
import type { GoalsDataController } from './goalsData.svelte';

export interface GoalsDetailController {
  selectedId: string | null;
  detailOpen: boolean;
  /** Live-resolved goal — null when selectedId doesn't match any
   *  loaded goal or no selection has been made. */
  readonly selected: Goal | null;
  /** Open the drawer for a goal; publishes to workspaceContext so
   *  an adjacent AI pane can surface this goal. */
  openDetail(g: Goal): void;
  /** Open by id; silent no-op when the id doesn't resolve. */
  openDetailById(id: string): void;
}

export interface GoalsDetailDeps {
  dataCtl: GoalsDataController;
}

export function createGoalsDetail(deps: GoalsDetailDeps): GoalsDetailController {
  let selectedId = $state<string | null>(null);
  let detailOpen = $state(false);
  const selected = $derived(deps.dataCtl.goals.find((g) => g.id === selectedId) ?? null);

  function openDetail(g: Goal) {
    selectedId = g.id;
    detailOpen = true;
    workspaceContext.publish({
      paneKind: 'goals',
      itemId: g.id,
      label: g.title,
      excerpt: g.description ?? undefined
    });
  }

  function openDetailById(id: string) {
    const g = deps.dataCtl.goals.find((x) => x.id === id);
    if (g) openDetail(g);
  }

  return {
    get selectedId() {
      return selectedId;
    },
    set selectedId(v) {
      selectedId = v;
    },
    get detailOpen() {
      return detailOpen;
    },
    set detailOpen(v) {
      detailOpen = v;
    },
    get selected() {
      return selected;
    },
    openDetail,
    openDetailById
  };
}
