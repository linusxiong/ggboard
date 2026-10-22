import { lazy, type ReactNode, Suspense } from 'react'
import { useAnimationsReady } from '@/lib/motion'
import { cx } from '@/lib/cn'

const CountUp = lazy(() => import('@/components/reactbits/CountUp'))

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

/** KPI tile used on dashboards. */
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
    <section className={cx('surface-card flex flex-col gap-3 p-5', className)}>
      <div className="flex items-center justify-between gap-3">
        <h3 className="text-subhead text-label-secondary">{label}</h3>
        {icon ? (
          <span
            className={cx('flex size-8 items-center justify-center rounded-[10px] text-white', accent)}
            aria-hidden="true"
          >
            {icon}
          </span>
        ) : null}
      </div>
      <p className="text-title2 tabular text-label">
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
