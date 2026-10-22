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

/** `93784` → "1d 2h 3m" (narrow Intl units, up to three parts). */
export function formatDuration(seconds: number | null, locale: Locale): string {
  if (seconds === null) return '—'
  const parts: Array<[Intl.NumberFormatOptions['unit'], number]> = []
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
  return parts
    .slice(0, 3)
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
