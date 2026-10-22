/** Animates a number from `from` to `to` with an ease-out curve on requestAnimationFrame, starting when visible.
 *  Original GG Bond component, GPL-3.0. */
import { useEffect, useRef, useState, type CSSProperties } from 'react'
import { useReducedMotion } from '@/lib/motion'
import { useInView } from './useInView'

export interface CountUpProps {
  to: number
  from?: number
  /** Seconds. */
  duration?: number
  /** Seconds before starting. */
  delay?: number
  decimals?: number
  separator?: string
  prefix?: string
  suffix?: string
  /** Start only when true (in addition to being in view). */
  startWhen?: boolean
  /** Custom display formatter (e.g. money or bytes); overrides decimals/separator/prefix/suffix. */
  formatter?: (value: number) => string
  onEnd?: () => void
  className?: string
  style?: CSSProperties
}

const easeOutExpo = (t: number) => (t >= 1 ? 1 : 1 - Math.pow(2, -10 * t))

export default function CountUp({
  to,
  from = 0,
  duration = 1,
  delay = 0,
  decimals = 0,
  separator = ',',
  prefix = '',
  suffix = '',
  startWhen = true,
  formatter,
  onEnd,
  className,
  style,
}: CountUpProps) {
  const ref = useRef<HTMLSpanElement>(null)
  const inView = useInView(ref)
  const reduced = useReducedMotion()
  const active = inView && startWhen
  const immediate = reduced || duration <= 0
  const [value, setValue] = useState(from)
  const onEndRef = useRef(onEnd)
  useEffect(() => {
    onEndRef.current = onEnd
  })

  // Immediate mode: the final value is rendered directly; only the completion callback runs here.
  useEffect(() => {
    if (active && immediate) onEndRef.current?.()
  }, [active, immediate])

  useEffect(() => {
    if (!active || immediate) return
    let frame = 0
    let start = 0
    const total = Math.max(1, duration * 1000)
    const tick = (now: number) => {
      if (!start) start = now
      const t = Math.min(1, (now - start) / total)
      setValue(from + (to - from) * easeOutExpo(t))
      if (t < 1) frame = requestAnimationFrame(tick)
      else onEndRef.current?.()
    }
    const timer = window.setTimeout(() => {
      frame = requestAnimationFrame(tick)
    }, delay * 1000)
    return () => {
      window.clearTimeout(timer)
      cancelAnimationFrame(frame)
    }
  }, [active, immediate, from, to, duration, delay])

  const shown = active && immediate ? to : value
  const display = formatter
    ? formatter(shown)
    : `${prefix}${withSeparator(shown.toFixed(decimals), separator)}${suffix}`
  return (
    <span ref={ref} className={className} style={style}>
      {display}
    </span>
  )
}

function withSeparator(fixed: string, separator: string): string {
  if (!separator) return fixed
  const [int = '', frac] = fixed.split('.')
  const grouped = int.replace(/\B(?=(\d{3})+(?!\d))/g, separator)
  return frac ? `${grouped}.${frac}` : grouped
}
