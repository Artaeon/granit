<script lang="ts">
  import { createEventDispatcher, afterUpdate } from 'svelte'
  import { marked } from 'marked'
  import { markedHighlight } from 'marked-highlight'
  import hljs from 'highlight.js'

  export let content = ''

  const dispatch = createEventDispatcher()

  // Setup syntax highlighting
  marked.use(markedHighlight({
    langPrefix: 'hljs language-',
    highlight(code: string, lang: string) {
      if (lang && hljs.getLanguage(lang)) {
        return hljs.highlight(code, { language: lang }).value
      }
      return hljs.highlightAuto(code).value
    }
  }))

  // Custom marked extensions for wikilinks and image embeds
  const wikilinkExtension = {
    name: 'wikilink',
    level: 'inline' as const,
    start(src: string) { return src.indexOf('[[') },
    tokenizer(src: string) {
      const match = src.match(/^\[\[([^\]|]+)(?:\|([^\]]+))?\]\]/)
      if (match) {
        return { type: 'wikilink', raw: match[0], target: match[1], display: match[2] || match[1] }
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
      if (match) { return { type: 'imageEmbed', raw: match[0], src: match[1] } }
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

  const renderer = {
    listitem(text: string, task: boolean, checked: boolean) {
      if (task) {
        const checkbox = `<input type="checkbox" ${checked ? 'checked' : ''} disabled />`
        return `<li class="task-item">${checkbox}${text}</li>\n`
      }
      return `<li>${text}</li>\n`
    },
    code(code: string, language: string | undefined) {
      const lang = language || ''
      let highlighted: string
      if (lang && hljs.getLanguage(lang)) {
        highlighted = hljs.highlight(code, { language: lang }).value
      } else if (lang) {
        highlighted = hljs.highlightAuto(code).value
      } else {
        highlighted = code.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')
      }
      const langLabel = lang ? `<div class="code-lang-label">${lang}</div>` : ''
      const copyBtn = `<button class="code-copy-btn" onclick="navigator.clipboard.writeText(this.parentElement.querySelector('code').textContent).then(()=>{this.textContent='Copied!';setTimeout(()=>this.textContent='Copy',1500)})">Copy</button>`
      return `<div class="code-block-wrapper">${langLabel}${copyBtn}<pre><code class="hljs language-${lang}">${highlighted}</code></pre></div>`
    },
    blockquote(quote: string) {
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
        return `<div class="callout ${colorClasses[type] || 'callout-note'}">
          <div class="callout-title">${icons[type] || '📝'} ${title}</div>
          ${body ? `<div class="callout-body">${body}</div>` : ''}
        </div>`
      }
      return `<blockquote>${quote}</blockquote>`
    },
    table(header: string, body: string) {
      return `<div class="table-wrapper"><table><thead>${header}</thead><tbody>${body}</tbody></table></div>`
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
    const copyBtn = (e.target as HTMLElement).closest('.code-copy-btn')
    if (copyBtn) { e.preventDefault(); return }

    const target = (e.target as HTMLElement).closest('a.wikilink') as HTMLElement
    if (target) {
      e.preventDefault()
      const note = target.dataset.note
      if (note) dispatch('wikilink', note)
    }
  }

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
