import type { ReactNode } from 'react'
import { Inbox } from '@/components/icons/inline'
import { cx } from '@/lib/cn'

interface EmptyStateProps {
  title: ReactNode
  description?: ReactNode
  icon?: ReactNode
  /** Action area (button/link). */
  action?: ReactNode
  className?: string
  /** Compact variant for table bodies / cards. */
  compact?: boolean
}

/** Friendly empty/placeholder state (also used for "coming soon" pages). */
export function EmptyState({ title, description, icon, action, className, compact = false }: EmptyStateProps) {
  return (
    <div
      className={cx(
        'flex flex-col items-center justify-center text-center',
        compact ? 'gap-2 px-4 py-8' : 'gap-3 px-6 py-16',
        className,
      )}
    >
      <div
        className={cx(
          'flex items-center justify-center rounded-full bg-fill-tertiary text-label-secondary',
          compact ? 'size-10' : 'size-14',
        )}
        aria-hidden="true"
      >
        {icon ?? <Inbox className={compact ? 'size-5' : 'size-7'} strokeWidth={1.75} />}
      </div>
      <p className={cx('text-label', compact ? 'text-subhead font-semibold' : 'text-headline')}>{title}</p>
      {description ? (
        <p className="max-w-sm text-subhead text-label-secondary">{description}</p>
      ) : null}
      {action ? <div className="mt-2">{action}</div> : null}
    </div>
  )
}
