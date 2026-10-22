/**
 * LineChart — dependency-free SVG line/area chart.
 *
 *   <LineChart aria-label="收入（近 30 天）" labels={dates} series={[{ id: 'income', label: '收入', values }]}
 *     formatValue={(v) => formatMoney(v)} formatLabel={(d) => formatDate(d, locale, 'short')} />
 *
 * - Colours come from theme tokens (`text-system-*` + `currentColor`), never hex.
 * - Crosshair + tooltip on hover; the plot is focusable and ←/→/Home/End move the readout.
 * - A visually hidden `<table>` lists every value (accessible fallback); the SVG is `role="img"`.
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
import {
  type Point,
  areaPath,
  linear,
  linePath,
  nearestIndex,
  niceTicksIn,
  sampleIndexes,
  seriesColor,
} from './scale'

export interface LineChartProps extends BaseChartProps {
  /** Fill a 10% wash under each line (default true). */
  area?: boolean
}

const PAD = { top: 12, right: 12, bottom: 28 }

export function LineChart(props: LineChartProps) {
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
    area = true,
    tickUnit = 1,
    className,
  } = props
  const label = props['aria-label']
  const [ref, width] = useChartWidth()
  const count = labels.length
  const { active, setActive, focusProps } = useActiveIndex(count)

  const values = series.flatMap((s) => s.values)
  const ticks = niceTicksIn(
    values.length ? Math.min(...values) : 0,
    values.length ? Math.max(...values) : 1,
    tickUnit,
  )
  const tickText = ticks.map(formatTick ?? formatValue)
  const left = axisGutter(tickText)
  const plotW = Math.max(1, width - left - PAD.right)
  const plotH = Math.max(1, height - PAD.top - PAD.bottom)
  const baseline = PAD.top + plotH
  const y = linear([ticks[0] ?? 0, ticks.at(-1) ?? 1], [baseline, PAD.top])
  const x = (i: number) => (count <= 1 ? left + plotW / 2 : left + (i / (count - 1)) * plotW)
  const xLabels = sampleIndexes(count, Math.max(2, Math.floor(plotW / 84)))

  const onPointerMove = (event: PointerEvent<SVGSVGElement>) => {
    const rect = event.currentTarget.getBoundingClientRect()
    const scale = rect.width > 0 ? width / rect.width : 1
    setActive(nearestIndex((event.clientX - rect.left) * scale, left, plotW, count))
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
                    x={x(i)}
                    y={height - 8}
                    textAnchor={
                      count > 1 && i === 0 ? 'start' : count > 1 && i === count - 1 ? 'end' : 'middle'
                    }
                    className="fill-label-secondary text-caption2"
                  >
                    {formatLabel(labels[i] ?? '')}
                  </text>
                ))}
                {series.map((s, si) => {
                  const points: Point[] = s.values.map((v, i) => [x(i), y(v)])
                  return (
                    <g key={s.id} className={seriesColor(si)}>
                      {area && points.length > 1 ? (
                        <path d={areaPath(points, baseline)} fill="currentColor" fillOpacity={0.1} />
                      ) : null}
                      {points.length > 1 ? (
                        <path
                          d={linePath(points)}
                          fill="none"
                          stroke="currentColor"
                          strokeWidth={2}
                          strokeLinejoin="round"
                          strokeLinecap="round"
                        />
                      ) : null}
                      {points.length === 1 || active === null ? (
                        <circle
                          cx={points.at(-1)?.[0]}
                          cy={points.at(-1)?.[1]}
                          r={4}
                          fill="currentColor"
                          className="stroke-elevated"
                          strokeWidth={2}
                        />
                      ) : null}
                    </g>
                  )
                })}
                {active !== null ? (
                  <g>
                    <line
                      x1={x(active)}
                      x2={x(active)}
                      y1={PAD.top}
                      y2={baseline}
                      className="stroke-label-tertiary"
                      strokeWidth={1}
                    />
                    {series.map((s, si) => (
                      <circle
                        key={s.id}
                        cx={x(active)}
                        cy={y(s.values[active] ?? 0)}
                        r={4.5}
                        fill="currentColor"
                        className={cx(seriesColor(si), 'stroke-elevated')}
                        strokeWidth={2}
                      />
                    ))}
                  </g>
                ) : null}
              </g>
            </svg>
            {active !== null ? (
              <ChartTooltip
                x={x(active)}
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
      <ChartLegend series={series} shape="line" />
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

export default LineChart
