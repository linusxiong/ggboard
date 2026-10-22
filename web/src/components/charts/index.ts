/**
 * Dependency-free SVG charts (no chart library): `LineChart`, `BarChart`, `Sparkline`.
 * Theme-token colours, hover/focus tooltips, `role="img"` + `aria-label`, and a visually hidden
 * data table per chart. Import them from admin/user pages; they are small enough to ship with
 * the page chunk.
 */
export { LineChart } from './LineChart'
export type { LineChartProps } from './LineChart'
export { BarChart } from './BarChart'
export type { BarChartProps } from './BarChart'
export { Sparkline } from './Sparkline'
export type { SparklineProps } from './Sparkline'
export type { ChartSeries, BaseChartProps } from './chart-utils'
export { byteUnit, niceTicks, niceTicksIn, SERIES_COLORS } from './scale'
