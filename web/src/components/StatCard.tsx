import { lazy, type ReactNode, Suspense } from 'react'
import { useAnimationsReady } from '@/lib/motion'
import { cx } from '@/lib/cn'

const CountUp = lazy(() => import('@/components/effects/CountUp'))

interface StatCardProps {
  label: ReactNode
  /** A number animates with CountUp (after idle, motion permitting) via `format`. */
  value: ReactNode | number
  /** Formats numeric values (money, bytes…). Required for correct final text when animating. */
  format?: (value: number) => string
  icon?: ReactNode
  /** Small text below the value. */
  hint?: ReactNode
  /** Decorative accent for the icon chip, e.g. `bg-system-blue`. */
  accent?: string
  className?: string
  /** Disable the count-up animation. */
  static?: boolean
}

/** KPI tile used on dashboards: symbol badge + label, large tabular value, optional hint. */
export function StatCard({
  label,
  value,
  format,
  icon,
  hint,
  accent = 'bg-system-blue',
  className,
  static: isStatic,
}: StatCardProps) {
  const animate = useAnimationsReady()
  const numeric = typeof value === 'number'
  const text = numeric ? (format ? format(value) : String(value)) : null

  return (
    <section className={cx('surface-card flex flex-col gap-2 p-5', className)}>
      <div className="flex items-center gap-2.5">
        {icon ? (
          <span
            className={cx(
              'flex size-7 shrink-0 items-center justify-center rounded-[8px] text-white [&_svg]:size-4',
              accent,
            )}
            aria-hidden="true"
          >
            {icon}
          </span>
        ) : null}
        <h3 className="min-w-0 truncate text-subhead font-medium text-label-secondary">{label}</h3>
      </div>
      <p className="mt-1 text-title1 font-semibold tabular tracking-tight text-label">
        {numeric && animate && !isStatic ? (
          <Suspense fallback={text}>
            <CountUp to={value} duration={0.8} formatter={format} />
          </Suspense>
        ) : numeric ? (
          text
        ) : (
          value
        )}
      </p>
      {hint ? <p className="text-footnote text-label-secondary">{hint}</p> : null}
    </section>
  )
}
