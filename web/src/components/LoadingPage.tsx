import { cx } from '@/lib/cn'

/** Apple-style activity indicator (pure CSS, no JS deps). */
export function Spinner({
  size = 20,
  className,
  label,
}: {
  size?: number
  className?: string
  label?: string
}) {
  return (
    <span
      role={label ? 'status' : undefined}
      aria-label={label}
      aria-hidden={label ? undefined : true}
      className={cx(
        'inline-block shrink-0 animate-spin rounded-full border-2 border-fill border-t-label-secondary',
        className,
      )}
      style={{ width: size, height: size }}
    />
  )
}

/**
 * Full-area loading state for route `pendingComponent` / Suspense fallbacks.
 * Keeps layout stable (min-height) to avoid CLS. Text is static (no i18n dependency) on purpose:
 * it may render before dictionaries load.
 */
export function LoadingPage({
  fullscreen = false,
  label = 'Loading…',
}: {
  fullscreen?: boolean
  label?: string
}) {
  return (
    <div
      className={cx('flex w-full items-center justify-center', fullscreen ? 'min-h-dvh' : 'min-h-[40vh]')}
      role="status"
      aria-live="polite"
    >
      <Spinner size={24} />
      <span className="sr-only">{label}</span>
    </div>
  )
}
