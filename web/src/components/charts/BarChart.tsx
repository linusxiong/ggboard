/**
 * BarChart — dependency-free SVG column chart (grouped or stacked), for non-negative values.
 *
 *   <BarChart aria-label="节点流量（近 7 天）" labels={dates} stacked
 *     series={[{ id: 'u', label: '上行', values: up }, { id: 'd', label: '下行', values: down }]}
 *     formatValue={(v) => formatBytes(v)} />
 *
 * Marks follow the dataviz spec: ≤ 24px columns, 4px rounded data end, square base, a 2px
 * surface gap between stacked segments / adjacent columns. The hovered band lifts with a wash;
 * the tooltip lists every series. Hidden fallback table + focusable plot as in LineChart.
 */
import type { PointerEvent } from 'react'
import { cx } from '@/lib/cn'
import {
  type BaseChartProps,
  axisGutter,
  identity,
  plainNumber,
  useActiveIndex,
  useChartWidth,
} from './chart-utils'
import { ChartDataTable, ChartEmpty, ChartLegend, ChartTooltip } from './parts'
import { bandIndex, columnPath, linear, niceTicksIn, sampleIndexes, seriesColor } from './scale'

export interface BarChartProps extends BaseChartProps {
  /** Stack the series in one column per label (default: grouped side by side). */
  stacked?: boolean
}

const PAD = { top: 12, right: 8, bottom: 28 }
const GAP = 2
const MAX_BAR = 24

export function BarChart(props: BarChartProps) {
  const {
    labels,
    series,
    formatValue = plainNumber,
    formatTick,
    formatLabel = identity,
    labelHeader = '—',
    height = 240,
    isFetching = false,
    emptyLabel = '—',
    stacked = false,
    tickUnit = 1,
    className,
  } = props
  const label = props['aria-label']
  const [ref, width] = useChartWidth()
  const count = labels.length
  const { active, setActive, focusProps } = useActiveIndex(count)

  const totals = labels.map((_, i) =>
    stacked
      ? series.reduce((sum, s) => sum + Math.max(0, s.values[i] ?? 0), 0)
      : Math.max(0, ...series.map((s) => s.values[i] ?? 0)),
  )
  const ticks = niceTicksIn(0, Math.max(1, ...totals), tickUnit)
  const tickText = ticks.map(formatTick ?? formatValue)
  const left = axisGutter(tickText)
  const plotW = Math.max(1, width - left - PAD.right)
  const plotH = Math.max(1, height - PAD.top - PAD.bottom)
  const baseline = PAD.top + plotH
  const y = linear([0, ticks.at(-1) ?? 1], [baseline, PAD.top])
  const band = plotW / Math.max(1, count)
  const groups = stacked ? 1 : Math.max(1, series.length)
  const barW = Math.max(2, Math.min(MAX_BAR, (band * 0.72 - GAP * (groups - 1)) / groups))
  const groupW = barW * groups + GAP * (groups - 1)
  const bandX = (i: number) => left + i * band
  const xLabels = sampleIndexes(count, Math.max(2, Math.floor(plotW / 84)))

  const onPointerMove = (event: PointerEvent<SVGSVGElement>) => {
    const rect = event.currentTarget.getBoundingClientRect()
    const scale = rect.width > 0 ? width / rect.width : 1
    setActive(bandIndex((event.clientX - rect.left) * scale, left, plotW, count))
  }

  return (
    <figure className={cx('flex flex-col gap-2', className)}>
      <div ref={ref} className="relative w-full" style={{ height }}>
        {count === 0 ? (
          <ChartEmpty height={height} label={emptyLabel} />
        ) : (
          <>
            <svg
              role="img"
              aria-label={label}
              width={width}
              height={height}
              viewBox={`0 0 ${width} ${height}`}
              className={cx(
                'block touch-pan-y overflow-visible rounded-[8px] outline-offset-2 transition-opacity duration-200 focus-visible:outline-2 focus-visible:outline-focus',
                isFetching && 'opacity-60',
              )}
              onPointerMove={onPointerMove}
              onPointerLeave={() => setActive(null)}
              {...focusProps}
            >
              <g aria-hidden="true">
                {active !== null ? (
                  <rect
                    x={bandX(active)}
                    y={PAD.top}
                    width={band}
                    height={plotH}
                    rx={6}
                    className="fill-fill-quaternary"
                  />
                ) : null}
                {ticks.map((tick, i) => (
                  <g key={tick}>
                    <line
                      x1={left}
                      x2={left + plotW}
                      y1={y(tick)}
                      y2={y(tick)}
                      className="stroke-hairline"
                      strokeWidth={1}
                    />
                    <text
                      x={left - 6}
                      y={y(tick)}
                      textAnchor="end"
                      dominantBaseline="middle"
                      className="fill-label-secondary text-caption2 tabular"
                    >
                      {tickText[i]}
                    </text>
                  </g>
                ))}
                {xLabels.map((i) => (
                  <text
                    key={i}
                    x={bandX(i) + band / 2}
                    y={height - 8}
                    textAnchor="middle"
                    className="fill-label-secondary text-caption2"
                  >
                    {formatLabel(labels[i] ?? '')}
                  </text>
                ))}
                {labels.map((_, i) => {
                  const x0 = bandX(i) + (band - groupW) / 2
                  if (stacked) {
                    let top = baseline
                    const visible = series
                      .map((s, si) => ({ si, v: Math.max(0, s.values[i] ?? 0) }))
                      .filter((seg) => seg.v > 0)
                    return (
                      <g key={i}>
                        {visible.map((seg, k) => {
                          const h = baseline - y(seg.v)
                          const segTop = top - h
                          const isTop = k === visible.length - 1
                          // 2px surface gap below every segment that sits on another one
                          const drawH = Math.max(0, h - (k > 0 ? GAP : 0))
                          top = segTop
                          return (
                            <path
                              key={seg.si}
                              d={columnPath(x0, segTop, barW, drawH, isTop ? 4 : 0)}
                              fill="currentColor"
                              className={seriesColor(seg.si)}
                            />
                          )
                        })}
                      </g>
                    )
                  }
                  return (
                    <g key={i}>
                      {series.map((s, si) => {
                        const v = Math.max(0, s.values[i] ?? 0)
                        const h = baseline - y(v)
                        return (
                          <path
                            key={s.id}
                            d={columnPath(x0 + si * (barW + GAP), baseline - h, barW, h)}
                            fill="currentColor"
                            className={seriesColor(si)}
                          />
                        )
                      })}
                    </g>
                  )
                })}
                <line
                  x1={left}
                  x2={left + plotW}
                  y1={baseline}
                  y2={baseline}
                  className="stroke-hairline-opaque"
                  strokeWidth={1}
                />
              </g>
            </svg>
            {active !== null ? (
              <ChartTooltip
                x={bandX(active) + band / 2}
                containerWidth={width}
                title={formatLabel(labels[active] ?? '')}
                rows={series.map((s, si) => ({
                  id: s.id,
                  label: s.label,
                  value: formatValue(s.values[active] ?? 0),
                  colorIndex: si,
                }))}
              />
            ) : null}
          </>
        )}
      </div>
      <ChartLegend series={series} shape="rect" />
      <ChartDataTable
        caption={label}
        labels={labels}
        series={series}
        formatValue={formatValue}
        formatLabel={formatLabel}
        labelHeader={labelHeader}
      />
    </figure>
  )
}

export default BarChart
