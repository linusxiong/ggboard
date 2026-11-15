/** The viewer's calendar day as `YYYY-MM-DD`. `toISOString()` gives the UTC day: yesterday, before 08:00 in UTC+8. */
export function localDay(date: Date): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`
}
