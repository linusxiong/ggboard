import type { Locale } from '@/lib/api/types'
import { formatNumber } from '@/lib/format'

const UNIT_SECONDS: Record<string, number> = { h: 3600, m: 60, s: 1, ms: 1e-3, us: 1e-6, µs: 1e-6, ns: 1e-9 }

/** Go `time.Duration.String()` (e.g. `72h3m0.5s`) or plain seconds → seconds. `null` if unparseable. */
export function toSeconds(value: number | string | null | undefined): number | null {
  if (typeof value === 'number') return Number.isFinite(value) ? value : null
  if (typeof value !== 'string' || value.trim() === '') return null
  const s = value.trim()
  if (/^\d+(\.\d+)?$/.test(s)) return Number(s)
  const re = /(\d+(?:\.\d+)?)(h|ms|us|µs|ns|m|s)/g
  let total = 0
  let matched = ''
  for (const m of s.matchAll(re)) {
    total += Number(m[1]) * (UNIT_SECONDS[m[2] as string] ?? 0)
    matched += m[0]
  }
  return matched === s.replace(/^-/, '') ? (s.startsWith('-') ? -total : total) : null
}

type DurationUnit = 'day' | 'hour' | 'minute' | 'second'
/** `Intl.DurationFormat` as far as this file uses it (Chrome 129, Firefox 136, Safari 16.4; not in the ES2023 lib). */
type DurationFormat = new (
  locale: string,
  options: { style: 'narrow'; secondsDisplay?: 'always' },
) => { format(duration: Partial<Record<`${DurationUnit}s`, number>>): string }

/**
 * `93784` → "1d 2h 3m" / "1天2小时3分钟": days, hours and minutes, seconds only below a day (up to three parts).
 * `Intl.DurationFormat` joins the parts the way the locale writes a duration (no spaces in Chinese); older
 * browsers format each unit on its own and join with a space.
 */
export function formatDuration(seconds: number | null, locale: Locale): string {
  if (seconds === null) return '—'
  const parts: Array<[DurationUnit, number]> = []
  let rest = Math.max(0, Math.floor(seconds))
  const d = Math.floor(rest / 86_400)
  rest %= 86_400
  const h = Math.floor(rest / 3600)
  rest %= 3600
  const m = Math.floor(rest / 60)
  const s = rest % 60
  if (d) parts.push(['day', d])
  if (h) parts.push(['hour', h])
  if (m) parts.push(['minute', m])
  if (!d && (s || parts.length === 0)) parts.push(['second', s])

  const Native = (Intl as typeof Intl & { DurationFormat?: DurationFormat }).DurationFormat
  if (Native) {
    const duration: Partial<Record<`${DurationUnit}s`, number>> = {}
    for (const [unit, n] of parts) duration[`${unit}s`] = n
    // A zero duration would format as an empty string: keep "0s".
    const zero = parts.every(([, n]) => n === 0)
    return new Native(locale, { style: 'narrow', ...(zero ? { secondsDisplay: 'always' } : {}) }).format(
      duration,
    )
  }
  return parts
    .map(([unit, n]) => formatNumber(n, locale, { style: 'unit', unit, unitDisplay: 'narrow' }))
    .join(' ')
}

/** Log timestamp with milliseconds in the viewer's time zone (`2026-10-22 08:01:02.345`). */
export function formatLogTime(iso: string): string {
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return iso
  const p = (n: number, w = 2) => String(n).padStart(w, '0')
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())} ${p(d.getHours())}:${p(d.getMinutes())}:${p(d.getSeconds())}.${p(d.getMilliseconds(), 3)}`
}
