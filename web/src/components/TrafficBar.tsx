import { useId } from 'react'
import { useLocale, useT } from '@/i18n'
import { formatBytes, formatPercent } from '@/lib/format'
import { cx } from '@/lib/cn'

interface TrafficBarProps {
  /** Bytes used (u + d). */
  used: number
  /** Bytes total (`transfer_enable`). 0 = no quota. */
  total: number
  /** Visible label above the bar (defaults to "流量使用"). */
  label?: string
  showLabel?: boolean
  className?: string
}

/** Usage meter: blue < 70%, orange < 90%, red ≥ 90%. Exposed as an ARIA meter. */
export function TrafficBar({ used, total, label, showLabel = true, className }: TrafficBarProps) {
  const t = useT('common')
  const locale = useLocale()
  const id = useId()
  const ratio = total > 0 ? Math.min(1, Math.max(0, used / total)) : 0
  const tone = ratio >= 0.9 ? 'bg-system-red' : ratio >= 0.7 ? 'bg-system-orange' : 'bg-system-blue'
  const usedText = formatBytes(used, locale)
  const totalText = total > 0 ? formatBytes(total, locale) : t('values.unlimited')
  const valueText = t('traffic.used', { used: usedText, total: totalText })

  return (
    <div className={cx('flex flex-col gap-1.5', className)}>
      {showLabel ? (
        <div className="flex items-baseline justify-between gap-2 text-footnote">
          <span id={id} className="text-label-secondary">
            {label ?? t('traffic.label')}
          </span>
          <span className="tabular text-label">
            {usedText} / {totalText}
          </span>
        </div>
      ) : null}
      <div
        role="meter"
        aria-labelledby={showLabel ? id : undefined}
        aria-label={showLabel ? undefined : (label ?? t('traffic.label'))}
        aria-valuemin={0}
        aria-valuemax={100}
        aria-valuenow={Math.round(ratio * 100)}
        aria-valuetext={`${valueText} (${formatPercent(ratio, locale)})`}
        className="h-2 w-full overflow-hidden rounded-full bg-fill-tertiary"
      >
        <div
          className={cx('h-full rounded-full transition-[width] duration-300 ease-out', tone)}
          style={{ width: `${(ratio * 100).toFixed(2)}%` }}
        />
      </div>
    </div>
  )
}
