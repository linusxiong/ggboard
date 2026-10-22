import { useEffect, useState } from 'react'
import { cx } from '@/lib/cn'

type Renderer = (source: string) => string
let rendererPromise: Promise<Renderer> | null = null

/** Loads `marked` + `dompurify` once (separate lazy chunk). */
function loadRenderer(): Promise<Renderer> {
  rendererPromise ??= Promise.all([import('marked'), import('dompurify')]).then(([{ marked }, { default: DOMPurify }]) => {
    DOMPurify.addHook('afterSanitizeAttributes', (node) => {
      if (node.tagName === 'A' && node.getAttribute('href')?.match(/^https?:\/\//)) {
        node.setAttribute('target', '_blank')
        node.setAttribute('rel', 'noopener noreferrer')
      }
    })
    return (source: string) =>
      DOMPurify.sanitize(marked.parse(source, { async: false, gfm: true, breaks: true }), {
        USE_PROFILES: { html: true },
        FORBID_TAGS: ['style', 'iframe', 'form', 'input'],
      })
  })
  return rendererPromise
}

interface MarkdownProps {
  /** Markdown source (notices, knowledge articles, plan descriptions). */
  source: string
  className?: string
}

/** Sanitised Markdown → HTML. Shows a skeleton while the renderer loads. */
export function Markdown({ source, className }: MarkdownProps) {
  const [html, setHtml] = useState<string | null>(null)

  useEffect(() => {
    let alive = true
    loadRenderer()
      .then((render) => {
        if (alive) setHtml(render(source))
      })
      .catch(() => {
        if (alive) setHtml(null)
      })
    return () => {
      alive = false
    }
  }, [source])

  if (html === null) {
    return (
      <div className={cx('flex flex-col gap-2', className)} aria-busy="true">
        <div className="h-4 w-3/4 animate-pulse rounded bg-fill-tertiary" />
        <div className="h-4 w-full animate-pulse rounded bg-fill-tertiary" />
        <div className="h-4 w-2/3 animate-pulse rounded bg-fill-tertiary" />
      </div>
    )
  }
  // Sanitised by DOMPurify above.
  return <div className={cx('markdown', className)} dangerouslySetInnerHTML={{ __html: html }} />
}

export default Markdown
