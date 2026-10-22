/**
 * `<input type="datetime-local">` ↔ RFC3339 (no date library: the browser parses a
 * `YYYY-MM-DDTHH:mm` string without offset as local time).
 */
const pad = (n: number) => String(n).padStart(2, '0')

/** RFC3339 → `YYYY-MM-DDTHH:mm` in local time; `''` for null/invalid. */
export function isoToLocalInput(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** `YYYY-MM-DDTHH:mm` (local) → RFC3339 UTC without milliseconds; `null` for empty/invalid. */
export function localInputToIso(value: string | null | undefined): string | null {
  if (!value) return null
  const d = new Date(value)
  if (Number.isNaN(d.getTime())) return null
  return d.toISOString().replace(/\.\d{3}Z$/, 'Z')
}

/** Local calendar date `YYYY-MM-DD`. */
export function localDate(d: Date = new Date()): string {
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}`
}

/** `YYYY-MM-DD` shifted by `days`. */
export function addDays(date: string, days: number): string {
  const [y, m, d] = date.split('-').map(Number)
  return localDate(new Date(y ?? 1970, (m ?? 1) - 1, (d ?? 1) + days))
}
