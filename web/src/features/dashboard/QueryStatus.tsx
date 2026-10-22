import { Button } from '@heroui/react'
import { CircleAlert } from 'lucide-react'
import type { ReactNode } from 'react'
import { useT } from '@/i18n'
import { cx } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'

interface QueryLike<T> {
  data: T | undefined
  isPending: boolean
  isError: boolean
  error: unknown
  refetch: () => unknown
}

/**
 * Renders `children(data)` once a query has data; a fixed-height placeholder while pending (CLS 0) and an
 * inline error with retry on failure. Keeps cached data visible during background refetches.
 */
export function QueryStatus<T>({
  query,
  minHeight,
  className,
  children,
}: {
  query: QueryLike<T>
  /** Placeholder height in px (match the loaded content). */
  minHeight: number
  className?: string
  children: (data: T) => ReactNode
}) {
  const t = useT('common')
  if (query.data !== undefined) return <>{children(query.data)}</>
  if (query.isError) {
    return (
      <div
        role="alert"
        className={cx(
          'surface-card flex flex-col items-center justify-center gap-3 p-6 text-center',
          className,
        )}
        style={{ minHeight }}
      >
        <CircleAlert className="size-6 text-system-red" aria-hidden="true" />
        <p className="text-subhead text-label-secondary">{errorMessage(query.error)}</p>
        <Button size="sm" variant="secondary" onPress={() => void query.refetch()}>
          {t('actions.retry')}
        </Button>
      </div>
    )
  }
  return (
    <div aria-busy="true" className={cx('surface-card animate-pulse', className)} style={{ minHeight }}>
      <span className="sr-only">{t('states.loading')}</span>
    </div>
  )
}
