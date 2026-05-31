<script lang="ts">
  import { EVENT_STATUSES, type EventStatus } from '$lib/api';

  // Content-pipeline editor for kind==='content' events. Three fields:
  // status (segmented control), channels (chip input), tags (chip
  // input). Rendered inside EventDetail's edit form when the event is
  // a content kind — invisible otherwise, so non-content users see no
  // extra clutter.
  //
  // Status is a single-select. Re-clicking the active chip clears it
  // back to '' so a user can demote a half-published item without
  // hunting through a dropdown for a "none" entry. The view-mode
  // counterpart is rendered by EventDetail directly from event.status
  // / event.channels / event.tags so the read shape stays simple.
  //
  // Channels + Tags are freeform string lists. Enter / comma commits
  // the current draft; clicking × on a chip removes it. We dedupe
  // case-insensitively so 'Twitter' and 'twitter' don't both land,
  // but we preserve the casing the user typed first so a brand-name
  // capitalisation sticks ('LinkedIn' not 'linkedin').

  interface Props {
    status: EventStatus | '';
    channels: string[];
    tags: string[];
    /** When true the chips render read-only — view-mode use. */
    readonly?: boolean;
    onStatusChange: (s: EventStatus | '') => void;
    onChannelsChange: (c: string[]) => void;
    onTagsChange: (t: string[]) => void;
  }

  let {
    status,
    channels,
    tags,
    readonly = false,
    onStatusChange,
    onChannelsChange,
    onTagsChange
  }: Props = $props();

  let channelInput = $state('');
  let tagInput = $state('');

  function statusLabel(s: EventStatus): string {
    return s[0].toUpperCase() + s.slice(1);
  }

  function statusClass(s: EventStatus): string {
    if (status !== s) {
      return 'bg-surface0 border-surface1 text-subtext hover:bg-surface1';
    }
    // Tinted by stage so a quick glance at the picker tells the user
    // where this item sits in the funnel without reading the label.
    switch (s) {
      case 'idea':
        return 'bg-surface1 border-overlay0 text-text';
      case 'drafting':
        return 'bg-blue/15 border-blue/40 text-blue';
      case 'review':
        return 'bg-yellow/15 border-yellow/40 text-yellow';
      case 'scheduled':
        return 'bg-lavender/15 border-lavender/40 text-lavender';
      case 'published':
        return 'bg-green/15 border-green/40 text-green';
      case 'archived':
        return 'bg-surface0 border-overlay0 text-dim line-through';
    }
  }

  function addFromInput(
    raw: string,
    list: string[],
    apply: (next: string[]) => void,
    clearInput: () => void
  ): void {
    // Trim + dedupe case-insensitively. Preserve original casing on
    // the kept entry so a brand-name capitalisation isn't flattened
    // ('LinkedIn' stays 'LinkedIn' even if the dup attempt was
    // 'linkedin'). Comma-separated paste also splits cleanly:
    // 'twitter, linkedin, blog' → three chips in one Enter.
    const parts = raw
      .split(',')
      .map((p) => p.trim())
      .filter(Boolean);
    if (parts.length === 0) {
      clearInput();
      return;
    }
    const seen = new Set(list.map((c) => c.toLowerCase()));
    const additions: string[] = [];
    for (const p of parts) {
      const key = p.toLowerCase();
      if (seen.has(key)) continue;
      seen.add(key);
      additions.push(p);
    }
    if (additions.length > 0) apply([...list, ...additions]);
    clearInput();
  }

  function addChannel() {
    addFromInput(channelInput, channels, onChannelsChange, () => (channelInput = ''));
  }
  function removeChannel(idx: number) {
    onChannelsChange(channels.filter((_, i) => i !== idx));
  }
  function addTag() {
    addFromInput(tagInput, tags, onTagsChange, () => (tagInput = ''));
  }
  function removeTag(idx: number) {
    onTagsChange(tags.filter((_, i) => i !== idx));
  }

  function onChipKey(e: KeyboardEvent, add: () => void) {
    if (e.key === 'Enter' || e.key === ',') {
      e.preventDefault();
      add();
    }
  }
</script>

<div class="space-y-3 border-t border-surface1 pt-3">
  <!-- Status — segmented control. Click an inactive chip to set;
       click the active one to clear back to ''. -->
  <div>
    <div class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Status</div>
    <div class="flex flex-wrap gap-1">
      {#each EVENT_STATUSES as s (s)}
        <button
          type="button"
          disabled={readonly}
          onclick={() => onStatusChange(status === s ? '' : s)}
          class="text-[11px] px-2 py-1 rounded border transition-colors disabled:cursor-default {statusClass(s)}"
        >{statusLabel(s)}</button>
      {/each}
    </div>
  </div>

  <!-- Channels — freeform chips. Drives the week-view swim lane
       grouping (channels[0]) and the per-event display strip. -->
  <div>
    <div class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Channels</div>
    {#if channels.length > 0}
      <div class="flex flex-wrap gap-1 mb-1.5">
        {#each channels as c, i (c + ':' + i)}
          <span class="inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded bg-lavender/15 border border-lavender/40 text-lavender">
            {c}
            {#if !readonly}
              <button
                type="button"
                onclick={() => removeChannel(i)}
                class="text-lavender/70 hover:text-text"
                aria-label="Remove {c}"
              >×</button>
            {/if}
          </span>
        {/each}
      </div>
    {/if}
    {#if !readonly}
      <div class="flex items-center gap-1.5">
        <input
          type="text"
          bind:value={channelInput}
          onkeydown={(e) => onChipKey(e, addChannel)}
          onblur={addChannel}
          placeholder="twitter, linkedin, blog..."
          class="flex-1 text-xs px-2 py-1 bg-surface0 border border-surface1 rounded focus:outline-none focus:border-lavender text-text"
        />
        <button
          type="button"
          onclick={addChannel}
          disabled={!channelInput.trim()}
          class="text-[11px] px-2 py-1 rounded bg-surface1 hover:bg-overlay0 text-subtext disabled:opacity-40"
        >add</button>
      </div>
    {/if}
  </div>

  <!-- Tags — same shape as channels; general-purpose grouping. -->
  <div>
    <div class="text-[11px] uppercase tracking-wider text-dim mb-1.5">Tags</div>
    {#if tags.length > 0}
      <div class="flex flex-wrap gap-1 mb-1.5">
        {#each tags as t, i (t + ':' + i)}
          <span class="inline-flex items-center gap-1 text-[11px] px-2 py-0.5 rounded bg-surface1 border border-overlay0 text-subtext">
            {t}
            {#if !readonly}
              <button
                type="button"
                onclick={() => removeTag(i)}
                class="text-dim hover:text-text"
                aria-label="Remove {t}"
              >×</button>
            {/if}
          </span>
        {/each}
      </div>
    {/if}
    {#if !readonly}
      <div class="flex items-center gap-1.5">
        <input
          type="text"
          bind:value={tagInput}
          onkeydown={(e) => onChipKey(e, addTag)}
          onblur={addTag}
          placeholder="campaign, q3, retrospective..."
          class="flex-1 text-xs px-2 py-1 bg-surface0 border border-surface1 rounded focus:outline-none focus:border-primary text-text"
        />
        <button
          type="button"
          onclick={addTag}
          disabled={!tagInput.trim()}
          class="text-[11px] px-2 py-1 rounded bg-surface1 hover:bg-overlay0 text-subtext disabled:opacity-40"
        >add</button>
      </div>
    {/if}
  </div>
</div>
