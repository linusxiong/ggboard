/**
 * `<input type="datetime-local">` ↔ RFC3339 (no date library: the browser parses a
 * `YYYY-MM-DDTHH:mm` string without offset as local time).
 *
 * There is deliberately no "today as `YYYY-MM-DD`" helper: statistics and traffic days are the
 * server's calendar days (`today.date` of `GET /admin/stats/overview`), not the browser's.
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

/**
 * 00:00 in the browser's zone of the calendar day `date` (`YYYY-MM-DD`) shifted by `days`, as RFC3339
 * UTC. Date filters on instants (`created_at`) send these bounds, so a picked day means the day the
 * list displays (browser time), not the server's calendar day. 23- and 25-hour DST days come out right.
 */
export function localDayStartIso(date: string, days = 0): string {
  const [y = 1970, m = 1, d = 1] = date.split('-').map(Number)
  return new Date(y, m - 1, d + days).toISOString().replace(/\.\d{3}Z$/, 'Z')
}
