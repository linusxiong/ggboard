/**
 * Sparkline — tiny trend line for stat tiles. The line wears the de-emphasis grey, the latest
 * point the accent. `role="img"` + `aria-label` (summarise the trend in the label); values are
 * not individually interactive, so pair it with a visible number.
 */
import { cx } from '@/lib/cn'
import { linear, linePath, type Point } from './scale'

export interface SparklineProps {
  values: readonly number[]
  'aria-label': string
  width?: number
  height?: number
  className?: string
}

export function Sparkline({ values, width = 96, height = 28, className, ...rest }: SparklineProps) {
  const pad = 3
  const min = values.length ? Math.min(...values) : 0
  const max = values.length ? Math.max(...values) : 0
  const y = linear([min, max === min ? min + 1 : max], [height - pad, pad])
  const x = linear([0, Math.max(1, values.length - 1)], [pad, width - pad])
  const points: Point[] = values.map((v, i) => [x(i), max === min ? height / 2 : y(v)])
  const last = points.at(-1)
  return (
    <svg
      role="img"
      aria-label={rest['aria-label']}
      width={width}
      height={height}
      viewBox={`0 0 ${width} ${height}`}
      className={cx('block shrink-0 overflow-visible', className)}
    >
      {points.length > 1 ? (
        <path
          d={linePath(points)}
          fill="none"
          stroke="currentColor"
          strokeWidth={1.5}
          strokeLinejoin="round"
          strokeLinecap="round"
          className="text-system-gray"
        />
      ) : null}
      {last ? (
        <circle cx={last[0]} cy={last[1]} r={2.75} fill="currentColor" className="text-system-blue" />
      ) : null}
    </svg>
  )
}

export default Sparkline
