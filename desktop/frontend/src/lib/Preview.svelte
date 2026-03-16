<script lang="ts">
  import { createEventDispatcher } from 'svelte'
  import { marked } from 'marked'

  export let content = ''

  const dispatch = createEventDispatcher()

  // Custom marked extensions for wikilinks and image embeds
  const wikilinkExtension = {
    name: 'wikilink',
    level: 'inline' as const,
    start(src: string) { return src.indexOf('[[') },
    tokenizer(src: string) {
      const match = src.match(/^\[\[([^\]|]+)(?:\|([^\]]+))?\]\]/)
      if (match) {
        return {
          type: 'wikilink',
          raw: match[0],
          target: match[1],
          display: match[2] || match[1],
        }
      }
    },
    renderer(token: any) {
      return `<a class="wikilink" href="#" data-note="${token.target}">${token.display}</a>`
    }
  }

  const imageEmbedExtension = {
    name: 'imageEmbed',
    level: 'inline' as const,
    start(src: string) { return src.indexOf('![[') },
    tokenizer(src: string) {
      const match = src.match(/^!\[\[([^\]]+)\]\]/)
      if (match) {
        return {
          type: 'imageEmbed',
          raw: match[0],
          src: match[1],
        }
      }
    },
    renderer(token: any) {
      const ext = token.src.split('.').pop()?.toLowerCase() || ''
      const imageExts = ['png', 'jpg', 'jpeg', 'gif', 'svg', 'webp', 'bmp', 'ico']
      if (imageExts.includes(ext)) {
        return `<img src="/vault-assets/${token.src}" alt="${token.src}" class="note-image" loading="lazy" />`
      }
      return `<a class="wikilink embed" href="#" data-note="${token.src}">📄 ${token.src}</a>`
    }
  }

  // Custom renderer
  const renderer = {
    listitem(text: string, task: boolean, checked: boolean) {
      if (task) {
        const checkbox = `<input type="checkbox" ${checked ? 'checked' : ''} disabled />`
        return `<li class="task-item">${checkbox}${text}</li>\n`
      }
      return `<li>${text}</li>\n`
    },
    // Code blocks with language label
    code(code: string, language: string | undefined) {
      const lang = language || ''
      const escaped = code.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      const langLabel = lang ? `<div class="code-lang-label">${lang}</div>` : ''
      return `<div class="code-block-wrapper">${langLabel}<pre><code class="language-${lang}">${escaped}</code></pre></div>`
    },
    // Blockquote with callout support
    blockquote(quote: string) {
      // Check for callout pattern: [!type] optional title
      const calloutMatch = quote.match(/^\s*<p>\s*\[!([\w-]+)\]\s*(.*?)<\/p>/)
      if (calloutMatch) {
        const type = calloutMatch[1].toLowerCase()
        const title = calloutMatch[2] || type.charAt(0).toUpperCase() + type.slice(1)
        const body = quote.replace(calloutMatch[0], '').trim()

        const icons: Record<string, string> = {
          'note': '📝', 'info': 'ℹ️', 'tip': '💡', 'hint': '💡',
          'warning': '⚠️', 'caution': '⚠️', 'danger': '🔴', 'error': '🔴',
          'success': '✅', 'check': '✅', 'question': '❓', 'faq': '❓',
          'example': '📋', 'quote': '💬', 'cite': '💬',
          'bug': '🐛', 'abstract': '📄', 'summary': '📄', 'todo': '📌',
        }
        const icon = icons[type] || '📝'

        const colorClasses: Record<string, string> = {
          'note': 'callout-note', 'info': 'callout-info', 'tip': 'callout-success', 'hint': 'callout-success',
          'warning': 'callout-warning', 'caution': 'callout-warning',
          'danger': 'callout-danger', 'error': 'callout-danger',
          'success': 'callout-success', 'check': 'callout-success',
          'question': 'callout-info', 'faq': 'callout-info',
          'example': 'callout-note', 'quote': 'callout-note', 'cite': 'callout-note',
          'bug': 'callout-danger', 'abstract': 'callout-info', 'summary': 'callout-info',
          'todo': 'callout-warning',
        }
        const colorClass = colorClasses[type] || 'callout-note'

        return `<div class="callout ${colorClass}">
          <div class="callout-title">${icon} ${title}</div>
          ${body ? `<div class="callout-body">${body}</div>` : ''}
        </div>`
      }
      return `<blockquote>${quote}</blockquote>`
    },
  }

  marked.use({
    extensions: [imageEmbedExtension, wikilinkExtension],
    renderer,
    gfm: true,
    breaks: true,
  })

  let previewEl: HTMLDivElement

  function handlePreviewClick(e: MouseEvent) {
    const target = (e.target as HTMLElement).closest('a.wikilink') as HTMLElement
    if (target) {
      e.preventDefault()
      const note = target.dataset.note
      if (note) dispatch('wikilink', note)
    }
  }

  // Debounce preview rendering for split mode performance
  let renderedHtml = ''
  let renderTimer: ReturnType<typeof setTimeout>
  $: {
    clearTimeout(renderTimer)
    if (content) {
      renderTimer = setTimeout(() => { renderedHtml = marked.parse(content) as string }, 150)
    } else {
      renderedHtml = ''
    }
  }
</script>

<!-- svelte-ignore a11y-click-events-have-key-events -->
<!-- svelte-ignore a11y-no-static-element-interactions -->
<div class="h-full overflow-y-auto bg-ctp-base" bind:this={previewEl} on:click={handlePreviewClick}>
  <article class="prose prose-granit max-w-3xl mx-auto p-8">
    {@html renderedHtml}
  </article>
</div>
