/**
 * Tiny scale/path helpers for the SVG charts (no d3). Pure functions, unit-tested.
 */

/** Categorical series colours in fixed order (validated: blue/pink pass CVD + contrast in light and dark). */
export const SERIES_COLORS = ['text-system-blue', 'text-system-pink'] as const

export function seriesColor(index: number): string {
  return SERIES_COLORS[index % SERIES_COLORS.length] ?? SERIES_COLORS[0]
}

/** "Nice" step for `span / count` (1, 2, 2.5, 5 × 10^n). */
function niceStep(span: number, count: number): number {
  const raw = span / Math.max(1, count)
  const power = 10 ** Math.floor(Math.log10(raw))
  const fraction = raw / power
  const nice = fraction <= 1 ? 1 : fraction <= 2 ? 2 : fraction <= 2.5 ? 2.5 : fraction <= 5 ? 5 : 10
  return nice * power
}

/**
 * Rounded y-axis ticks from 0 (or the negative minimum) to at least `max`.
 * `niceTicks(0, 87)` → `[0, 25, 50, 75, 100]`.
 */
export function niceTicks(min: number, max: number, count = 4, minStep = 0): number[] {
  const lo = Math.min(0, min)
  let hi = Math.max(0, max)
  if (hi === lo) hi = lo + 1
  const step = Math.max(minStep, niceStep(hi - lo, count))
  const start = Math.floor(lo / step) * step
  const ticks: number[] = []
  for (let v = start; v < hi + step / 2; v += step) {
    ticks.push(Number(v.toPrecision(12)))
    if (ticks.length > 20) break
  }
  if ((ticks.at(-1) ?? 0) < hi) ticks.push(Number(((ticks.at(-1) ?? 0) + step).toPrecision(12)))
  return ticks
}

/**
 * Ticks that are round in `unit` (e.g. 1024³ for traffic so the axis reads 50 GB, 100 GB…),
 * returned in raw units. Raw values are whole numbers (bytes, cents, counts), so a step is never below one
 * raw unit: an all-zero series gets `[0, 1]` instead of 0.25-steps that format as "0 B, 0 B, 1 B, 1 B, 1 B".
 */
export function niceTicksIn(min: number, max: number, unit = 1, count = 4): number[] {
  const u = unit > 0 ? unit : 1
  return niceTicks(min / u, max / u, count, 1 / u).map((v) => v * u)
}

/** Largest power of 1024 not above `max` (≥ 1): the unit `formatBytes` will display. */
export function byteUnit(max: number): number {
  if (!(max >= 1024)) return 1
  return 1024 ** Math.min(5, Math.floor(Math.log(max) / Math.log(1024)))
}

/** Linear mapping `domain → range`. A zero-width domain maps to the range start. */
export function linear(domain: readonly [number, number], range: readonly [number, number]) {
  const [d0, d1] = domain
  const [r0, r1] = range
  const span = d1 - d0
  return (value: number) => (span === 0 ? r0 : r0 + ((value - d0) / span) * (r1 - r0))
}

export type Point = readonly [number, number]

const round = (n: number) => Math.round(n * 100) / 100

/** `M x y L x y …` polyline. */
export function linePath(points: readonly Point[]): string {
  return points.map(([x, y], i) => `${i === 0 ? 'M' : 'L'}${round(x)} ${round(y)}`).join(' ')
}

/** Closed area under a polyline down to `baseline`. */
export function areaPath(points: readonly Point[], baseline: number): string {
  if (points.length === 0) return ''
  const first = points[0]!
  const last = points[points.length - 1]!
  return `${linePath(points)} L${round(last[0])} ${round(baseline)} L${round(first[0])} ${round(baseline)} Z`
}

/**
 * Column path with a rounded data end (top) and a square base (dataviz mark spec: 4px radius).
 * `y` is the top edge, `height` grows down to the baseline.
 */
export function columnPath(x: number, y: number, width: number, height: number, radius = 4): string {
  if (height <= 0 || width <= 0) return ''
  const r = Math.min(radius, width / 2, height)
  const right = x + width
  const bottom = y + height
  return [
    `M${round(x)} ${round(bottom)}`,
    `L${round(x)} ${round(y + r)}`,
    `Q${round(x)} ${round(y)} ${round(x + r)} ${round(y)}`,
    `L${round(right - r)} ${round(y)}`,
    `Q${round(right)} ${round(y)} ${round(right)} ${round(y + r)}`,
    `L${round(right)} ${round(bottom)}`,
    'Z',
  ].join(' ')
}

/** Index of the category band containing `x` (0-based), clamped. */
export function bandIndex(x: number, left: number, width: number, count: number): number {
  if (count <= 1 || width <= 0) return 0
  const step = width / count
  return Math.max(0, Math.min(count - 1, Math.floor((x - left) / step)))
}

/** Index of the point closest to `x` for points spread evenly over `[left, left + width]`. */
export function nearestIndex(x: number, left: number, width: number, count: number): number {
  if (count <= 1 || width <= 0) return 0
  const step = width / (count - 1)
  return Math.max(0, Math.min(count - 1, Math.round((x - left) / step)))
}

/** Picks at most `max` evenly spaced indexes (always including the first and last). */
export function sampleIndexes(count: number, max: number): number[] {
  if (count <= 0) return []
  if (count <= max) return Array.from({ length: count }, (_, i) => i)
  const out = new Set<number>()
  const step = (count - 1) / (max - 1)
  for (let i = 0; i < max; i++) out.add(Math.round(i * step))
  return [...out]
}
