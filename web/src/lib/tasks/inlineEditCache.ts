// Tinyly cached lists of projects / goals / tags so the per-card
// inline-edit popovers don't refetch the same data on every open.
//
// Caches are module-level (process-singleton) and lazy: the first
// caller kicks off a single in-flight fetch, every subsequent caller
// awaits the same Promise, and the resolved items stay cached until
// invalidate() is called.
//
// Invalidation note: these caches are best-effort. We invalidate them
// on the obvious mutation hooks (createProject etc.) but if a NEW
// project / goal is created outside this app session, the popovers
// won't show it until the user reloads the page or hits the manual
// refresh button. That's an acceptable trade for the dramatic UX
// improvement of "click chip → see list instantly" instead of
// "click chip → see spinner → see list".

import { api, type Project, type Goal } from '$lib/api';

type Cache<T> = {
  loaded: boolean;
  items: T[];
  promise: Promise<T[]> | null;
};

function makeCache<T>(): Cache<T> {
  return { loaded: false, items: [], promise: null };
}

const projectsCache: Cache<Project> = makeCache();
const goalsCache: Cache<Goal> = makeCache();
const tagsCache: Cache<{ tag: string; count: number }> = makeCache();

export async function loadProjectsOnce(): Promise<Project[]> {
  if (projectsCache.loaded) return projectsCache.items;
  if (projectsCache.promise) return projectsCache.promise;
  projectsCache.promise = api
    .listProjects()
    .then((res) => {
      // Active projects only — archived ones aren't a sensible
      // assignment target from the card-level popover.
      projectsCache.items = res.projects.filter((p) => p.status !== 'archived');
      projectsCache.loaded = true;
      projectsCache.promise = null;
      return projectsCache.items;
    })
    .catch((e) => {
      projectsCache.promise = null;
      throw e;
    });
  return projectsCache.promise;
}

export async function loadGoalsOnce(): Promise<Goal[]> {
  if (goalsCache.loaded) return goalsCache.items;
  if (goalsCache.promise) return goalsCache.promise;
  goalsCache.promise = api
    .listGoals()
    .then((res) => {
      goalsCache.items = res.goals;
      goalsCache.loaded = true;
      goalsCache.promise = null;
      return goalsCache.items;
    })
    .catch((e) => {
      goalsCache.promise = null;
      throw e;
    });
  return goalsCache.promise;
}

export async function loadTagsOnce(): Promise<{ tag: string; count: number }[]> {
  if (tagsCache.loaded) return tagsCache.items;
  if (tagsCache.promise) return tagsCache.promise;
  tagsCache.promise = api
    .listTags()
    .then((res) => {
      tagsCache.items = res.tags;
      tagsCache.loaded = true;
      tagsCache.promise = null;
      return tagsCache.items;
    })
    .catch((e) => {
      tagsCache.promise = null;
      throw e;
    });
  return tagsCache.promise;
}

export function invalidateProjects() {
  projectsCache.loaded = false;
  projectsCache.items = [];
  projectsCache.promise = null;
}
export function invalidateGoals() {
  goalsCache.loaded = false;
  goalsCache.items = [];
  goalsCache.promise = null;
}
export function invalidateTags() {
  tagsCache.loaded = false;
  tagsCache.items = [];
  tagsCache.promise = null;
}
