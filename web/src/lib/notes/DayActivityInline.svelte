<script lang="ts">
  import { onMount } from 'svelte';
  import { api, type DayActivityItem } from '$lib/api';

  // DayActivityInline — the live "What happened that day" feed
  // that the daily-note renderer substitutes for the
  // `<!-- granit:day-activity -->` marker. Fetches /day-activity
  // for the given date, buckets the items by Kind, and renders
  // each as a navigable link. The marker itself is plain HTML
  // comment so external editors leave it alone; the actual
  // content is computed on every render so it stays current
  // without rewriting the underlying file.

  let { date }: { date: string } = $props();

  type Bucket = { kind: string; label: string; items: DayActivityItem[] };

  const KIND_LABELS: Record<string, string> = {
    note_created: 'Notes created',
    task_created: 'Tasks created',
    task_completed: 'Tasks completed',
    event: 'Calendar',
    habit: 'Habits',
    prayer: 'Prayer',
    hub_item: 'Hub',
    jot: 'Jots'
  };
  const BUCKET_ORDER: string[] = [
    'event',
    'task_created',
    'task_completed',
    'note_created',
    'jot',
    'habit',
    'prayer',
    'hub_item'
  ];

  let items = $state<DayActivityItem[]>([]);
  let loaded = $state(false);
  let loading = $state(false);
  let err = $state('');

  async function load() {
    if (loading) return;
    loading = true;
    err = '';
    try {
      const r = await api.dayActivity(date);
      items = r.items;
      loaded = true;
    } catch (e) {
      err = e instanceof Error ? e.message : String(e);
    } finally {
      loading = false;
    }
  }

  onMount(load);

  function bucketize(xs: DayActivityItem[]): Bucket[] {
    const groups = new Map<string, DayActivityItem[]>();
    for (const it of xs) {
      const arr = groups.get(it.kind) ?? [];
      arr.push(it);
      groups.set(it.kind, arr);
    }
    const out: Bucket[] = [];
    for (const k of BUCKET_ORDER) {
      const arr = groups.get(k);
      if (arr && arr.length > 0) out.push({ kind: k, label: KIND_LABELS[k] ?? k, items: arr });
    }
    for (const [k, arr] of groups) {
      if (BUCKET_ORDER.indexOf(k) === -1 && arr.length > 0) {
        out.push({ kind: k, label: KIND_LABELS[k] ?? k, items: arr });
      }
    }
    return out;
  }

  function activityHref(it: DayActivityItem): string {
    if (it.path) return `/notes/${encodeURIComponent(it.path)}`;
    if (it.kind === 'event') return '/calendar';
    if (it.kind === 'prayer') return '/prayer';
    if (it.kind === 'hub_item') return '/hub';
    if (it.kind === 'habit') return '/habits';
    if (it.kind === 'task_created' || it.kind === 'task_completed') return '/tasks';
    return '#';
  }

  function activityTime(at: string): string {
    const d = new Date(at);
    if (Number.isNaN(d.getTime())) return '';
    const hh = String(d.getHours()).padStart(2, '0');
    const mm = String(d.getMinutes()).padStart(2, '0');
    return `${hh}:${mm}`;
  }
</script>

<section
  class="day-activity-inline my-3 bg-surface0 border border-surface1 rounded p-3"
  aria-label="day activity"
>
  {#if loading && !loaded}
    <p class="text-xs text-dim italic">loading day activity…</p>
  {:else if err}
    <p class="text-xs text-error">Couldn't load day activity: {err}</p>
  {:else if items.length === 0}
    <p class="text-xs text-dim italic">No other activity tracked on this day.</p>
  {:else}
    {#each bucketize(items) as bucket (bucket.kind)}
      <div class="mb-3 last:mb-0">
        <h4 class="text-[10px] uppercase tracking-wider text-primary font-medium mb-1">
          {bucket.label}
          <span class="text-dim font-normal">({bucket.items.length})</span>
        </h4>
        <ul class="space-y-0.5">
          {#each bucket.items as it (it.kind + ':' + (it.target_id ?? it.path ?? it.title) + ':' + it.at)}
            <li class="flex items-baseline gap-2 text-xs">
              <span class="text-dim font-mono w-10 shrink-0">{activityTime(it.at)}</span>
              <a
                href={activityHref(it)}
                class="text-text hover:text-primary truncate"
              >{it.title}</a>
              {#if it.detail}
                <span class="text-dim truncate">· {it.detail}</span>
              {/if}
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  {/if}
</section>
