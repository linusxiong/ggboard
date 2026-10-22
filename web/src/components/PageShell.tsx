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
 * Standard page frame for user/admin pages: Large Title header, optional description and
 * actions, then content. Renders the page's single `<h1>`.
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
    <div className={cx('mx-auto flex w-full max-w-6xl flex-col gap-6', className)}>
      <header className="flex flex-wrap items-end justify-between gap-4">
        <div className="min-w-0">
          <h1 className="text-title1 sm:text-large-title text-label">{title}</h1>
          {description ? <p className="mt-1 text-subhead text-label-secondary">{description}</p> : null}
        </div>
        {actions ? <div className="flex flex-wrap items-center gap-2">{actions}</div> : null}
      </header>
      {children}
    </div>
  )
}
