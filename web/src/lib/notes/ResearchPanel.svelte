<!--
  ResearchPanel — right-rail aggregator that turns a research-style
  note into a navigable index. Three sections, all derived from the
  note body in O(n) so the panel stays fast on big docs:
    - Highlights (==text==): every yellow-marker span with its line.
    - Footnotes ([^id]): defs + refs with broken-link warnings.
    - Outbound URLs: every linked source in the note, so the user
      can scan their references without scrolling the body.

  Each entry is clickable → jump to the line in the editor (host
  page passes the onJump callback). The panel renders nothing when
  the note has none of the above — saves rail space on a fresh note.
-->
<script lang="ts">
  import { parseBody } from '$lib/util/bodyParse';

  let {
    body = '',
    onJump
  }: {
    body: string;
    onJump?: (lineNum: number) => void;
  } = $props();

  // Pull lines + fence flags from the shared parser cache. Previously
  // each of the three derivations below ran its own full-body
  // stripFences regex + split — three passes per keystroke per panel
  // mount, doubled by the desktop-rail / mobile-drawer rendering both
  // copies of the rail at every viewport. That work added up to a
  // material chunk of the per-keystroke freeze on long notes.
  //
  // Even with the shared parser, the three regex scans below
  // (highlights, footnotes, sources) iterate every line on every
  // keystroke. The user can't read a refreshed Research panel
  // faster than they can type, so we debounce 250ms — the panel
  // tracks the last "settled" body snapshot, which only advances
  // after the user pauses. While typing, the panel shows the prior
  // state; on the next pause it catches up. Cuts another big chunk
  // of per-keystroke work on long notes without user-visible lag
  // (the panel's value is "what's in this finished doc", not "what
  // I'm typing right now").
  let debouncedBody = $state('');
  let primed = false;
  $effect(() => {
    // Track body explicitly. The first run lands the current value
    // immediately (no flicker on mount); subsequent runs schedule a
    // 250ms catch-up so a typing burst doesn't fire 30 derives.
    const next = body;
    if (!primed) {
      primed = true;
      debouncedBody = next;
      return;
    }
    const id = setTimeout(() => { debouncedBody = next; }, 250);
    return () => clearTimeout(id);
  });

  type Highlight = { text: string; line: number };
  type Footnote = { id: string; line: number; defined: boolean; refOnly: boolean };
  type Source = { url: string; label: string; line: number };

  let highlights = $derived.by<Highlight[]>(() => {
    const parsed = parseBody(debouncedBody);
    const out: Highlight[] = [];
    const re = /==([^=\n][^=]*?)==/g;
    for (let i = 0; i < parsed.lines.length; i++) {
      if (parsed.inFence[i]) continue;
      let m: RegExpExecArray | null;
      re.lastIndex = 0;
      while ((m = re.exec(parsed.lines[i])) !== null) {
        const text = m[1].trim();
        if (text) out.push({ text, line: i + 1 });
      }
    }
    return out;
  });

  let footnotes = $derived.by<Footnote[]>(() => {
    const parsed = parseBody(debouncedBody);
    const refs = new Map<string, number>(); // first-occurrence line
    const defs = new Set<string>();
    const defLines = new Map<string, number>();
    for (let i = 0; i < parsed.lines.length; i++) {
      if (parsed.inFence[i]) continue;
      const ln = parsed.lines[i];
      const dm = /^\[\^([^\]\s]+)\]:\s/.exec(ln);
      if (dm) {
        defs.add(dm[1]);
        defLines.set(dm[1], i + 1);
      }
      const refRe = /\[\^([^\]\s]+)\]/g;
      let rm: RegExpExecArray | null;
      while ((rm = refRe.exec(ln)) !== null) {
        // Skip definition lines — those start with `[^id]:` and the
        // bare `[^id]` at the front matched by refRe is the def, not
        // a ref.
        if (ln.startsWith('[^' + rm[1] + ']:')) continue;
        if (!refs.has(rm[1])) refs.set(rm[1], i + 1);
      }
    }
    const all = new Set<string>([...refs.keys(), ...defs]);
    const out: Footnote[] = [];
    for (const id of all) {
      const line = refs.get(id) ?? defLines.get(id) ?? 1;
      out.push({
        id,
        line,
        defined: defs.has(id),
        refOnly: refs.has(id) && !defs.has(id)
      });
    }
    return out.sort((a, b) => a.line - b.line);
  });

  let sources = $derived.by<Source[]>(() => {
    const parsed = parseBody(debouncedBody);
    const out: Source[] = [];
    const seen = new Set<string>();
    // Markdown link form first (richest — gives us a label).
    const linkRe = /\[([^\]]+)\]\((https?:\/\/[^)\s]+)\)/g;
    for (let i = 0; i < parsed.lines.length; i++) {
      if (parsed.inFence[i]) continue;
      let m: RegExpExecArray | null;
      linkRe.lastIndex = 0;
      while ((m = linkRe.exec(parsed.lines[i])) !== null) {
        if (seen.has(m[2])) continue;
        seen.add(m[2]);
        out.push({ url: m[2], label: m[1], line: i + 1 });
      }
    }
    // Bare URLs (no label) — fall back to a hostname display.
    const bareRe = /(?:^|[\s(])(https?:\/\/[^\s)<>]+)/g;
    for (let i = 0; i < parsed.lines.length; i++) {
      if (parsed.inFence[i]) continue;
      let m: RegExpExecArray | null;
      bareRe.lastIndex = 0;
      while ((m = bareRe.exec(parsed.lines[i])) !== null) {
        const url = m[1].replace(/[.,;:]+$/, '');
        if (seen.has(url)) continue;
        seen.add(url);
        let host = url;
        try { host = new URL(url).hostname.replace(/^www\./, ''); } catch {}
        out.push({ url, label: host, line: i + 1 });
      }
    }
    return out;
  });

  let isEmpty = $derived(highlights.length === 0 && footnotes.length === 0 && sources.length === 0);
</script>

{#if !isEmpty}
  <div class="space-y-3">
    {#if highlights.length > 0}
      <section>
        <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5 flex items-baseline gap-1.5">
          <span class="text-warning">●</span>
          Highlights · {highlights.length}
        </div>
        <ul class="space-y-1">
          {#each highlights as h, i (i)}
            <li>
              <button
                type="button"
                onclick={() => onJump?.(h.line)}
                class="w-full text-left text-xs text-text hover:bg-surface0 rounded px-2 py-1 leading-snug"
                title={`line ${h.line}`}
              >
                <span class="text-warning/80">▌</span>
                <span class="ml-1">{h.text.length > 80 ? h.text.slice(0, 80) + '…' : h.text}</span>
              </button>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if footnotes.length > 0}
      <section>
        <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5 flex items-baseline gap-1.5">
          <span>¹</span>
          Footnotes · {footnotes.length}
          {#if footnotes.some((f) => f.refOnly)}
            <span class="text-error">· {footnotes.filter((f) => f.refOnly).length} unresolved</span>
          {/if}
        </div>
        <ul class="space-y-0.5">
          {#each footnotes as f (f.id)}
            <li>
              <button
                type="button"
                onclick={() => onJump?.(f.line)}
                class="w-full text-left text-xs hover:bg-surface0 rounded px-2 py-0.5 leading-snug flex items-baseline gap-2 {f.refOnly ? 'text-error' : 'text-text'}"
                title={f.refOnly ? 'unresolved — no [^id]: definition' : `definition on line ${f.line}`}
              >
                <span class="font-mono text-dim">[^{f.id}]</span>
                {#if f.refOnly}<span class="text-[10px]">⚠</span>{/if}
              </button>
            </li>
          {/each}
        </ul>
      </section>
    {/if}

    {#if sources.length > 0}
      <section>
        <div class="text-[10px] uppercase tracking-wider text-dim mb-1.5 flex items-baseline gap-1.5">
          <span>↗</span>
          Sources · {sources.length}
        </div>
        <ul class="space-y-0.5">
          {#each sources as s (s.url)}
            <li class="flex items-center gap-1.5 group">
              <button
                type="button"
                onclick={() => onJump?.(s.line)}
                class="flex-1 text-left text-xs text-text hover:text-primary px-2 py-0.5 rounded hover:bg-surface0 truncate"
                title={s.url}
              >
                {s.label}
              </button>
              <a
                href={s.url}
                target="_blank"
                rel="noopener noreferrer"
                aria-label="open source in new tab"
                class="opacity-0 group-hover:opacity-100 text-dim hover:text-primary text-xs px-1"
              >↗</a>
            </li>
          {/each}
        </ul>
      </section>
    {/if}
  </div>
{/if}
