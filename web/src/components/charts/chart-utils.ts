/** Chart types and hooks shared by LineChart / BarChart (no components here, for react-refresh). */
import { type KeyboardEvent, type RefObject, useLayoutEffect, useRef, useState } from 'react'

export interface ChartSeries {
  id: string
  /** Legend / tooltip / table header. */
  label: string
  /** One value per `labels` entry. */
  values: readonly number[]
}

export interface BaseChartProps {
  /** Accessible name of the chart image and caption of the fallback table. */
  'aria-label': string
  /** x categories (e.g. ISO dates), one per value. */
  labels: readonly string[]
  series: readonly ChartSeries[]
  /** Tooltip / table value format (money, bytes…). */
  formatValue?: (value: number) => string
  /** Compact y-axis tick format (defaults to `formatValue`). */
  formatTick?: (value: number) => string
  /** Make ticks round in this unit (e.g. `byteUnit(max)` for traffic). Default 1. */
  tickUnit?: number
  /** x label format (tooltip, axis, table). */
  formatLabel?: (label: string) => string
  /** Header of the label column in the fallback table (default "—"). */
  labelHeader?: string
  /** Plot height in px (fixed → no layout shift). */
  height?: number
  /** Dim the chart while a refetch is in flight (keeps the frame, no skeleton). */
  isFetching?: boolean
  /** Text shown when there is no data. */
  emptyLabel?: string
  className?: string
}

export const identity = (v: string) => v
export const plainNumber = (v: number) => String(v)

/** Measures the container width (ResizeObserver); `fallback` until measured (and in jsdom). */
export function useChartWidth(fallback = 640): [RefObject<HTMLDivElement | null>, number] {
  const ref = useRef<HTMLDivElement | null>(null)
  const [width, setWidth] = useState(fallback)
  useLayoutEffect(() => {
    const el = ref.current
    if (!el) return
    const measure = () => {
      const w = Math.floor(el.clientWidth)
      if (w > 0) setWidth(w)
    }
    measure()
    if (typeof ResizeObserver === 'undefined') return
    const ro = new ResizeObserver(measure)
    ro.observe(el)
    return () => ro.disconnect()
  }, [])
  return [ref, width]
}

/** Keyboard handling for the focusable plot: ←/→/Home/End move the readout, Esc hides it. */
export function useActiveIndex(count: number) {
  const [active, setActive] = useState<number | null>(null)
  const onKeyDown = (event: KeyboardEvent) => {
    if (count === 0) return
    const current = active ?? count - 1
    let next: number | null = null
    if (event.key === 'ArrowLeft') next = Math.max(0, current - 1)
    else if (event.key === 'ArrowRight') next = Math.min(count - 1, current + 1)
    else if (event.key === 'Home') next = 0
    else if (event.key === 'End') next = count - 1
    else if (event.key === 'Escape') {
      setActive(null)
      return
    }
    if (next !== null) {
      event.preventDefault()
      setActive(next)
    }
  }
  return {
    active,
    setActive,
    focusProps: {
      tabIndex: count > 0 ? 0 : -1,
      onKeyDown,
      onFocus: () => setActive((a) => a ?? (count > 0 ? count - 1 : null)),
      onBlur: () => setActive(null),
    },
  }
}

/** Y-axis gutter wide enough for the longest tick label (≈ 7px per glyph at 12px). */
export function axisGutter(ticks: readonly string[]): number {
  const longest = ticks.reduce((n, t) => Math.max(n, t.length), 0)
  return Math.max(28, Math.min(96, longest * 7 + 10))
}
