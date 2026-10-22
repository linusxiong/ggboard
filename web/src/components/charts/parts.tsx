/**
 * Shared chart pieces: legend, tooltip and the visually hidden data table that keeps every value
 * reachable without a pointer (dataviz: tooltips enhance, they never gate).
 */
import type { ReactNode } from 'react'
import type { ChartSeries } from './chart-utils'
import { seriesColor } from './scale'

/** Legend (≥ 2 series only — a single series is named by the chart title). */
export function ChartLegend({ series, shape }: { series: readonly ChartSeries[]; shape: 'line' | 'rect' }) {
  if (series.length < 2) return null
  return (
    <ul className="flex flex-wrap items-center gap-x-4 gap-y-1 text-footnote text-label-secondary">
      {series.map((s, i) => (
        <li key={s.id} className="flex items-center gap-1.5">
          <svg aria-hidden="true" width="14" height="10" className={seriesColor(i)}>
            {shape === 'line' ? (
              <line
                x1="1"
                y1="5"
                x2="13"
                y2="5"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
              />
            ) : (
              <rect x="2" y="1" width="10" height="8" rx="2" fill="currentColor" />
            )}
          </svg>
          {s.label}
        </li>
      ))}
    </ul>
  )
}

export interface TooltipRow {
  id: string
  label: string
  value: string
  colorIndex: number
}

/** Floating readout: value first (strong), series name second, keyed by a short line. */
export function ChartTooltip({
  x,
  containerWidth,
  title,
  rows,
}: {
  x: number
  containerWidth: number
  title: ReactNode
  rows: readonly TooltipRow[]
}) {
  // Sit beside the crosshair (never on top of the hovered point); flip left near the right edge.
  const flip = x > containerWidth - 196
  return (
    <div
      aria-hidden="true"
      data-testid="chart-tooltip"
      className="pointer-events-none absolute top-1 z-10 min-w-36 rounded-[12px] bg-elevated px-3 py-2 shadow-float"
      style={flip ? { left: x - 12, transform: 'translateX(-100%)' } : { left: x + 12 }}
    >
      <p className="text-caption text-label-secondary">{title}</p>
      <ul className="mt-1 flex flex-col gap-0.5">
        {rows.map((row) => (
          <li key={row.id} className="flex items-center gap-2 text-footnote">
            <svg aria-hidden="true" width="12" height="4" className={seriesColor(row.colorIndex)}>
              <line
                x1="1"
                y1="2"
                x2="11"
                y2="2"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
              />
            </svg>
            <span className="tabular font-semibold text-label">{row.value}</span>
            <span className="text-label-secondary">{row.label}</span>
          </li>
        ))}
      </ul>
    </div>
  )
}

/** Visually hidden table with every value (screen readers, copy/paste, no-pointer users). */
export function ChartDataTable({
  caption,
  labels,
  series,
  formatValue,
  formatLabel,
  labelHeader,
}: {
  caption: string
  labels: readonly string[]
  series: readonly ChartSeries[]
  formatValue: (value: number) => string
  formatLabel: (label: string) => string
  labelHeader: string
}) {
  return (
    <table className="sr-only">
      <caption>{caption}</caption>
      <thead>
        <tr>
          <th scope="col">{labelHeader}</th>
          {series.map((s) => (
            <th key={s.id} scope="col">
              {s.label}
            </th>
          ))}
        </tr>
      </thead>
      <tbody>
        {labels.map((label, i) => (
          <tr key={`${label}-${i}`}>
            <th scope="row">{formatLabel(label)}</th>
            {series.map((s) => (
              <td key={s.id}>{formatValue(s.values[i] ?? 0)}</td>
            ))}
          </tr>
        ))}
      </tbody>
    </table>
  )
}

export function ChartEmpty({ height, label }: { height: number; label: string }) {
  return (
    <div
      className="flex items-center justify-center rounded-[12px] bg-fill-quaternary text-subhead text-label-secondary"
      style={{ height }}
    >
      {label}
    </div>
  )
}
