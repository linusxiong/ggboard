/** `<input type="datetime-local">` ↔ RFC3339 (the input works in the browser's local time zone). */

const pad = (n: number) => String(n).padStart(2, '0')

/** RFC3339 → `YYYY-MM-DDTHH:mm` (local). Empty for null/invalid. */
export function toLocalInput(iso: string | null | undefined): string {
  if (!iso) return ''
  const d = new Date(iso)
  if (Number.isNaN(d.getTime())) return ''
  return `${d.getFullYear()}-${pad(d.getMonth() + 1)}-${pad(d.getDate())}T${pad(d.getHours())}:${pad(d.getMinutes())}`
}

/** `YYYY-MM-DDTHH:mm` (local) → RFC3339 UTC, or `null` when empty/invalid. */
export function fromLocalInput(local: string): string | null {
  if (!local) return null
  const d = new Date(local)
  return Number.isNaN(d.getTime()) ? null : d.toISOString().replace(/\.\d{3}Z$/, 'Z')
}

export type Validity = 'upcoming' | 'active' | 'expired'

export function validity(startedAt: string, endedAt: string, now = Date.now()): Validity {
  if (Date.parse(startedAt) > now) return 'upcoming'
  if (Date.parse(endedAt) < now) return 'expired'
  return 'active'
}
