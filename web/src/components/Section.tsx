import type { ReactNode } from 'react'
import { useId } from 'react'
import { TitleWithHelp } from '@/components/HelpTip'
import { cx } from '@/lib/cn'

interface SectionProps {
  title?: ReactNode
  /** 13 px caption under the title: short facts only. Explanations go in `help`. */
  caption?: ReactNode
  /** What the section does, behind an ⓘ next to the title. */
  help?: ReactNode
  /** Right-aligned header content (buttons, links). */
  actions?: ReactNode
  children: ReactNode
  /** Wrap the children in a card (`surface-card`, 20–24 px padding). Default true. */
  card?: boolean
  className?: string
  /** Extra classes for the card body. */
  bodyClassName?: string
  id?: string
}

/**
 * Titled page section: Title3-sized header + caption, then a card. Use inside `<PageShell>` (which
 * puts 32 px between sections); content inside the card is spaced 16 px.
 *
 *   <Section title="通知" caption="何时给你发邮件">…</Section>
 */
export function Section({
  title,
  caption,
  help,
  actions,
  children,
  card = true,
  className,
  bodyClassName,
  id,
}: SectionProps) {
  const auto = useId()
  const headingId = `${id ?? auto}-title`
  return (
    <section
      id={id}
      aria-labelledby={title ? headingId : undefined}
      className={cx('flex flex-col gap-3', className)}
    >
      {title || actions ? (
        <header className="flex flex-wrap items-end justify-between gap-x-4 gap-y-2 px-1">
          <div className="min-w-0">
            {title ? (
              <TitleWithHelp help={help} subject={typeof title === 'string' ? title : undefined}>
                <h2 id={headingId} className="text-title3 text-label">
                  {title}
                </h2>
              </TitleWithHelp>
            ) : null}
            {caption ? <p className="mt-0.5 text-footnote text-label-secondary">{caption}</p> : null}
          </div>
          {actions ? <div className="flex items-center gap-2">{actions}</div> : null}
        </header>
      ) : null}
      {card ? (
        <div className={cx('surface-card flex flex-col gap-4 p-5 sm:p-6', bodyClassName)}>{children}</div>
      ) : (
        children
      )}
    </section>
  )
}
