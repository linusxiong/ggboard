import { useLocale, useT } from '@/i18n'
import { formatBytes, formatPercent } from '@/lib/format'
import { cx } from '@/lib/cn'

interface TrafficRingProps {
  /** Bytes used (u + d). */
  used: number
  /** Bytes total (`transfer_enable`). 0 = no quota. */
  total: number
  /** Diameter in px. */
  size?: number
  className?: string
}

/**
 * Activity-ring style usage gauge (ARIA meter): blue < 70 %, orange < 90 %, red ≥ 90 %.
 * The percentage sits in the middle; the track is the grey fill color.
 */
export function TrafficRing({ used, total, size = 112, className }: TrafficRingProps) {
  const t = useT('common')
  const locale = useLocale()
  const ratio = total > 0 ? Math.min(1, Math.max(0, used / total)) : 0
  const stroke = Math.max(8, Math.round(size / 9))
  const r = (size - stroke) / 2
  const c = 2 * Math.PI * r
  const tone =
    ratio >= 0.9 ? 'stroke-system-red' : ratio >= 0.7 ? 'stroke-system-orange' : 'stroke-system-blue'
  const usedText = formatBytes(used, locale)
  const totalText = total > 0 ? formatBytes(total, locale) : t('values.unlimited')
  const percent = total > 0 ? formatPercent(ratio, locale) : '—'

  return (
    <div
      role="meter"
      aria-label={t('traffic.label')}
      aria-valuemin={0}
      aria-valuemax={100}
      aria-valuenow={Math.round(ratio * 100)}
      aria-valuetext={`${t('traffic.used', { used: usedText, total: totalText })} (${percent})`}
      className={cx('relative shrink-0', className)}
      style={{ width: size, height: size }}
    >
      <svg
        width={size}
        height={size}
        viewBox={`0 0 ${size} ${size}`}
        aria-hidden="true"
        className="-rotate-90"
      >
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          strokeWidth={stroke}
          className="stroke-fill-tertiary"
        />
        <circle
          cx={size / 2}
          cy={size / 2}
          r={r}
          fill="none"
          strokeWidth={stroke}
          strokeLinecap="round"
          strokeDasharray={c}
          strokeDashoffset={c * (1 - ratio)}
          className={cx(
            tone,
            'transition-[stroke-dashoffset] duration-700 ease-apple-out',
            ratio === 0 && 'opacity-0',
          )}
        />
      </svg>
      <div className="absolute inset-0 flex flex-col items-center justify-center" aria-hidden="true">
        <span className="text-title3 font-semibold tabular text-label">{percent}</span>
        <span className="text-caption text-label-secondary">{t('traffic.usedLabel')}</span>
      </div>
    </div>
  )
}
