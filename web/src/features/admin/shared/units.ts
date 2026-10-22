/**
 * Unit conversions for admin forms. API units: money = integer minor units ("cents"),
 * traffic = bytes, except `Plan.transfer_enable` (GB as entered). Forms edit human units.
 */

/** Traffic uses binary GB (1024³), like `lib/format.ts` `gbToBytes`. */
export const GIB = 1024 ** 3

/** Bytes → GB rounded to 2 decimals (form display). */
export function bytesToGb(bytes: number): number {
  return Math.round((bytes / GIB) * 100) / 100
}

/** GB (possibly fractional) → whole bytes. */
export function gbToBytes(gb: number): number {
  return Math.round(gb * GIB)
}

/** Keep in sync with `ZERO_DECIMAL` in `lib/format.ts` (`formatMoney` divides the rest by 100). */
const ZERO_DECIMAL = new Set(['JPY', 'KRW', 'VND', 'IDR', 'CLP', 'TWD'])

export function minorFactor(currency = 'CNY'): number {
  return ZERO_DECIMAL.has(currency.toUpperCase()) ? 1 : 100
}

/** Cents → currency units (12345 → 123.45). */
export function centsToUnits(cents: number, currency = 'CNY'): number {
  return cents / minorFactor(currency)
}

/** Currency units → cents, rounded (123.455 → 12346). */
export function unitsToCents(units: number, currency = 'CNY'): number {
  return Math.round(units * minorFactor(currency))
}

/** "a, b\nc" → ["a", "b", "c"] (trimmed, empty and duplicate entries dropped). */
export function parseList(text: string): string[] {
  const out: string[] = []
  for (const part of text.split(/[\n,，]/)) {
    const value = part.trim()
    if (value && !out.includes(value)) out.push(value)
  }
  return out
}

/** Inverse of `parseList` for single-line inputs. */
export function joinList(list: readonly string[] | null | undefined): string {
  return (list ?? []).join(', ')
}

/** Non-empty lines of a textarea (trimmed, order kept, duplicates kept). */
export function parseLines(text: string): string[] {
  return text
    .split('\n')
    .map((line) => line.trim())
    .filter(Boolean)
}
