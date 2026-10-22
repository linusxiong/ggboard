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

/**
 * Empty / placeholder state in the style of Apple's ContentUnavailableView: a large secondary
 * symbol, a title, a short explanation and an optional action.
 */
export function EmptyState({
  title,
  description,
  icon,
  action,
  className,
  compact = false,
}: EmptyStateProps) {
  return (
    <div
      className={cx(
        'flex flex-col items-center justify-center text-center',
        compact ? 'gap-1.5 px-4 py-10' : 'gap-2 px-6 py-16',
        className,
      )}
    >
      <div
        className={cx(
          'mb-1 flex items-center justify-center text-system-gray [&_svg]:stroke-[1.5]',
          compact ? '[&_svg]:size-8' : '[&_svg]:size-11',
        )}
        aria-hidden="true"
      >
        {icon ?? <Inbox />}
      </div>
      <p className={cx('text-label', compact ? 'text-headline' : 'text-title3')}>{title}</p>
      {description ? <p className="max-w-sm text-subhead text-label-secondary">{description}</p> : null}
      {action ? <div className={compact ? 'mt-2' : 'mt-3'}>{action}</div> : null}
    </div>
  )
}
