/**
 * 7-day traffic column chart in plain SVG (no chart library). Single series → no legend (the card title
 * names it); per-column hover tooltip with the upload/download split; an sr-only table carries the data.
 */
import { useLayoutEffect, useRef, useState } from 'react'
import { useLocale, useT } from '@/i18n'
import { formatBytes } from '@/lib/format'
import type { DayTraffic } from './traffic'
import { niceByteMax } from './traffic'

const HEIGHT = 180
const PAD = { top: 12, right: 4, bottom: 24, left: 52 }
const FALLBACK_WIDTH = 560

function useWidth() {
  const ref = useRef<HTMLDivElement>(null)
  const [width, setWidth] = useState(0)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    setWidth(el.clientWidth)
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver((entries) => {
      const w = entries[0]?.contentRect.width
      if (w) setWidth(Math.round(w))
    })
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  return [ref, width || FALLBACK_WIDTH] as const
}

/** Column with a 4px rounded data-end, square at the baseline. */
function columnPath(x: number, y: number, w: number, h: number): string {
  if (h <= 0) return ''
  const r = Math.min(4, h, w / 2)
  return `M${x},${y + h}V${y + r}Q${x},${y} ${x + r},${y}H${x + w - r}Q${x + w},${y} ${x + w},${y + r}V${y + h}Z`
}

export function TrafficChart({ days }: { days: readonly DayTraffic[] }) {
  const t = useT(['dashboard', 'common'])
  const locale = useLocale()
  const [ref, width] = useWidth()
  const [hover, setHover] = useState<number | null>(null)

  const max = niceByteMax(Math.max(0, ...days.map((d) => d.total)))
  const plotW = Math.max(0, width - PAD.left - PAD.right)
  const plotH = HEIGHT - PAD.top - PAD.bottom
  const band = days.length > 0 ? plotW / days.length : 0
  const barW = Math.max(4, Math.min(24, band * 0.56))
  const y = (v: number) => PAD.top + plotH - (v / max) * plotH
  const ticks = [0, max / 2, max]
  const dayLabel = new Intl.DateTimeFormat(locale, { month: 'numeric', day: 'numeric' })
  const label = (date: string) => {
    const [yy = 0, mm = 1, dd = 1] = date.split('-').map(Number)
    return dayLabel.format(new Date(yy, mm - 1, dd))
  }
  const empty = days.every((d) => d.total === 0)
  const active = hover === null ? undefined : days[hover]

  return (
    <div ref={ref} className="relative w-full" style={{ height: HEIGHT }}>
      <svg
        width={width}
        height={HEIGHT}
        className="block overflow-visible"
        aria-hidden="true"
        focusable="false"
      >
        {ticks.map((tick) => (
          <g key={tick}>
            <line
              x1={PAD.left}
              x2={width - PAD.right}
              y1={y(tick)}
              y2={y(tick)}
              className="stroke-hairline"
              strokeWidth={1}
              shapeRendering="crispEdges"
            />
            <text
              x={PAD.left - 8}
              y={y(tick)}
              dy="0.32em"
              textAnchor="end"
              className="fill-label-secondary text-[11px] tabular"
            >
              {formatBytes(tick, locale, { digits: 0 })}
            </text>
          </g>
        ))}
        {days.map((day, i) => {
          const cx = PAD.left + band * i + band / 2
          const top = y(day.total)
          return (
            <g key={day.date}>
              <path
                d={columnPath(cx - barW / 2, top, barW, PAD.top + plotH - top)}
                className={
                  hover === null || hover === i
                    ? 'fill-system-blue transition-opacity duration-200'
                    : 'fill-system-blue opacity-40 transition-opacity duration-200'
                }
              />
              <text
                x={cx}
                y={HEIGHT - 6}
                textAnchor="middle"
                className={
                  hover === i ? 'fill-label text-[11px] tabular' : 'fill-label-secondary text-[11px] tabular'
                }
              >
                {label(day.date)}
              </text>
              {/* Hit target: the whole column band, bigger than the mark. */}
              <rect
                x={PAD.left + band * i}
                y={PAD.top}
                width={band}
                height={plotH}
                fill="transparent"
                onPointerEnter={() => setHover(i)}
                onPointerLeave={() => setHover((h) => (h === i ? null : h))}
              />
            </g>
          )
        })}
      </svg>

      {empty ? (
        <p className="pointer-events-none absolute inset-x-0 top-1/2 -translate-y-1/2 pl-[52px] text-center text-footnote text-label-secondary">
          {t('traffic.empty')}
        </p>
      ) : null}

      {active && hover !== null ? (
        <div
          role="presentation"
          className="pointer-events-none absolute z-10 min-w-36 -translate-x-1/2 rounded-[10px] bg-overlay px-3 py-2 text-footnote shadow-overlay"
          style={{
            left: Math.min(Math.max(PAD.left + band * hover + band / 2, 80), Math.max(80, width - 80)),
            top: Math.max(0, y(active.total) - 76),
          }}
        >
          <p className="font-semibold text-label">{label(active.date)}</p>
          <p className="tabular text-label">{formatBytes(active.total, locale)}</p>
          <p className="tabular text-label-secondary">
            ↑ {formatBytes(active.u, locale)} · ↓ {formatBytes(active.d, locale)}
          </p>
        </div>
      ) : null}

      <table className="sr-only">
        <caption>{t('traffic.title')}</caption>
        <thead>
          <tr>
            <th scope="col">{t('traffic.date')}</th>
            <th scope="col">{t('traffic.upload')}</th>
            <th scope="col">{t('traffic.download')}</th>
            <th scope="col">{t('traffic.total')}</th>
          </tr>
        </thead>
        <tbody>
          {days.map((day) => (
            <tr key={day.date}>
              <th scope="row">{day.date}</th>
              <td>{formatBytes(day.u, locale)}</td>
              <td>{formatBytes(day.d, locale)}</td>
              <td>{formatBytes(day.total, locale)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  )
}
