<script lang="ts">
  // Hero / header block for the venture detail page.
  //
  // Fourth extraction step out of routes/ventures/[name]/+page.svelte.
  // Renders the back-arrow, the color dot, the name / mission /
  // description stack, the status pill (which doubles as an inline
  // status-change select), URL link, tag chips, "since" timestamp,
  // and the small AI-summary trigger button.
  //
  // Behavior preserved 1:1 — onChangeStatus is the same callback the
  // page used for the status select, onSummarize / aiBusy / aiText
  // are the same wiring the AI panel uses. The hero is a pure
  // presentation component: no $state of its own.
  import { type Venture } from '$lib/api';
  import { colorVar, statusTone } from './venturesDetailHelpers';

  type Status = 'active' | 'paused' | 'archived';

  interface Props {
    venture: Venture;
    aiBusy: boolean;
    aiText: string;
    onChangeStatus: (s: Status) => void | Promise<void>;
    onSummarize: () => void | Promise<void>;
  }
  let { venture, aiBusy, aiText, onChangeStatus, onSummarize }: Props = $props();
</script>

<header class="mb-4">
  <div class="flex items-start gap-3">
    <a
      href="/ventures"
      aria-label="back to ventures"
      class="flex-shrink-0 w-9 h-9 flex items-center justify-center text-subtext hover:text-primary hover:bg-surface0 rounded -ml-1"
    >
      <svg viewBox="0 0 24 24" class="w-5 h-5" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M15 18l-6-6 6-6" stroke-linecap="round" stroke-linejoin="round" />
      </svg>
    </a>
    <span
      class="w-3 h-3 rounded-full flex-shrink-0 mt-3"
      style="background: {colorVar(venture.color)}"
      aria-hidden="true"
    ></span>
    <div class="flex-1 min-w-0">
      <h1 class="text-2xl sm:text-3xl font-semibold text-text break-words">{venture.name}</h1>
      {#if venture.mission}
        <p class="text-sm sm:text-base text-subtext italic mt-1 break-words">{venture.mission}</p>
      {/if}
      {#if venture.description}
        <p class="text-sm text-subtext mt-2 break-words">{venture.description}</p>
      {/if}
      <div class="flex flex-wrap items-center gap-x-3 gap-y-1.5 text-xs text-dim mt-3">
        <!-- Status pill is also the inline change control —
             tapping the select fires patchVenture. The pill
             styling stays consistent with the cards page. -->
        <label
          class="px-2 py-0.5 rounded uppercase tracking-wider text-[10px] inline-flex items-center gap-1 cursor-pointer"
          style="background: color-mix(in srgb, var(--color-{statusTone(venture.status)}) 14%, transparent); color: var(--color-{statusTone(venture.status)});"
          title="Change status"
        >
          <span aria-hidden="true">●</span>
          <select
            value={venture.status ?? 'active'}
            onchange={(e) => onChangeStatus((e.currentTarget as HTMLSelectElement).value as Status)}
            class="bg-transparent appearance-none outline-none text-[10px] uppercase tracking-wider cursor-pointer"
            aria-label="Venture status"
            style="color: inherit;"
          >
            <option value="active">active</option>
            <option value="paused">paused</option>
            <option value="archived">archived</option>
          </select>
        </label>
        {#if venture.url}
          <a
            href={venture.url}
            target="_blank"
            rel="noopener noreferrer"
            class="text-secondary hover:underline truncate font-mono text-[11px]"
          >↗ {venture.url.replace(/^https?:\/\//, '').replace(/\/$/, '')}</a>
        {/if}
        {#if venture.tags && venture.tags.length > 0}
          <span class="flex flex-wrap items-center gap-1">
            {#each venture.tags as t}
              <span class="text-[10px] px-1.5 py-0.5 bg-surface1 text-subtext rounded">#{t}</span>
            {/each}
          </span>
        {/if}
        {#if venture.created_at}
          <span class="text-[11px]" title="created {venture.created_at}">since {venture.created_at}</span>
        {/if}
      </div>
    </div>
    <!-- AI summary trigger — kept top-right so it's discoverable
         without crowding the title row on small screens. The
         actual summary renders below the metric strip. -->
    <button
      onclick={onSummarize}
      disabled={aiBusy}
      class="hidden sm:inline-flex flex-shrink-0 items-center gap-1.5 px-2.5 py-1.5 text-xs bg-surface0 border border-surface1 hover:border-primary rounded text-subtext hover:text-primary disabled:opacity-50"
      title="AI status summary"
    >
      <svg viewBox="0 0 24 24" class="w-3.5 h-3.5" fill="none" stroke="currentColor" stroke-width="2">
        <path d="M12 2v4M12 18v4M4.93 4.93l2.83 2.83M16.24 16.24l2.83 2.83M2 12h4M18 12h4M4.93 19.07l2.83-2.83M16.24 7.76l2.83-2.83" stroke-linecap="round" />
      </svg>
      <span>{aiBusy ? 'thinking…' : aiText ? 'regenerate' : 'AI summary'}</span>
    </button>
  </div>
</header>
