/**
 * Markdown for knowledge articles. Same lazy `marked` + `dompurify` chunks as `@/components/Markdown`,
 * but with its own DOMPurify instance that also keeps proxy-client deep links (`clash://install-config?…`,
 * `sing-box://import-remote-profile?…`, `shadowrocket://add/…`) so "one-click import" links in articles
 * work as plain links. Scripts, styles, frames, forms and `javascript:`/`data:` URLs are still removed.
 * `.knowledge-no-access` blocks (content gated behind a plan) are styled as a callout.
 */
import { useEffect, useState } from 'react'
import { cx } from '@/lib/cn'

/** http(s)/mailto/tel, relative URLs and well-known client import schemes. */
const ALLOWED_URI =
  /^(?:(?:https?|mailto|tel|clash|clashx|clash-verge|clashmeta|mihomo|stash|shadowrocket|sing-box|sn|surge|surge3|quantumult-x|quantumult|loon|hiddify|v2rayng|v2rayn|v2box|nekobox|nekoray|karing|streisand|flclash|surfboard):|[^a-z]|[a-z+.-]+(?:[^a-z+.\-:]|$))/i

type Renderer = (source: string) => string
let rendererPromise: Promise<Renderer> | null = null

function loadRenderer(): Promise<Renderer> {
  rendererPromise ??= Promise.all([import('marked'), import('dompurify')]).then(
    ([{ marked }, { default: createDOMPurify }]) => {
      const purify = createDOMPurify(window)
      purify.addHook('afterSanitizeAttributes', (node) => {
        if (node.tagName !== 'A') return
        const href = node.getAttribute('href') ?? ''
        if (/^https?:\/\//i.test(href)) {
          node.setAttribute('target', '_blank')
          node.setAttribute('rel', 'noopener noreferrer')
        }
      })
      return (source: string) =>
        purify.sanitize(marked.parse(source, { async: false, gfm: true, breaks: true }), {
          USE_PROFILES: { html: true },
          FORBID_TAGS: ['style', 'iframe', 'form', 'input', 'button', 'textarea', 'select'],
          ALLOWED_URI_REGEXP: ALLOWED_URI,
        })
    },
  )
  return rendererPromise
}

/** Utility classes for the gated-content callout (`<div class="knowledge-no-access">`). */
const NO_ACCESS = cx(
  '[&_.knowledge-no-access]:my-4 [&_.knowledge-no-access]:rounded-[14px] [&_.knowledge-no-access]:border-[0.5px]',
  '[&_.knowledge-no-access]:border-hairline [&_.knowledge-no-access]:bg-fill-quaternary [&_.knowledge-no-access]:px-4',
  '[&_.knowledge-no-access]:py-3 [&_.knowledge-no-access]:text-subhead [&_.knowledge-no-access]:text-label-secondary',
)

export default function KnowledgeMarkdown({ source, className }: { source: string; className?: string }) {
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
        <div className="h-4 w-5/6 animate-pulse rounded bg-fill-tertiary" />
        <div className="h-4 w-2/3 animate-pulse rounded bg-fill-tertiary" />
      </div>
    )
  }
  // Sanitised by DOMPurify above.
  return <div className={cx('markdown', NO_ACCESS, className)} dangerouslySetInnerHTML={{ __html: html }} />
}
