import { type ReactNode, useEffect } from 'react'
import { cx } from '@/lib/cn'

interface PageShellProps {
  title: string
  description?: ReactNode
  /** Right-aligned header actions (buttons, filters). */
  actions?: ReactNode
  children?: ReactNode
  className?: string
  /** Set `document.title` to "<title> · <suffix>". Default true. */
  setDocumentTitle?: boolean
  titleSuffix?: string
}

/**
 * Standard page frame for user/admin pages: Large Title (34/700) header, 15 px secondary subtitle,
 * optional actions, then content — 1120 px max width, 32 px between sections. Renders the page's
 * single `<h1>`. Group content inside with `<Section>` (`@/components/Section`).
 */
export function PageShell({
  title,
  description,
  actions,
  children,
  className,
  setDocumentTitle = true,
  titleSuffix = 'GG Bond',
}: PageShellProps) {
  useEffect(() => {
    if (setDocumentTitle) document.title = `${title} · ${titleSuffix}`
  }, [title, titleSuffix, setDocumentTitle])

  return (
    <div className={cx('mx-auto flex w-full max-w-page flex-col gap-8', className)}>
      <header className="flex flex-wrap items-end justify-between gap-x-6 gap-y-4">
        <div className="min-w-0 flex-1 basis-72">
          <h1 className="text-large-title text-label">{title}</h1>
          {description ? (
            <p className="mt-1.5 max-w-3xl text-subhead text-label-secondary">{description}</p>
          ) : null}
        </div>
        {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
      </header>
      {children}
    </div>
  )
}
