/** Statistics days are server calendar days (`docs/API.md` › Stats); this names the server's UTC offset. */
const pad = (n: number) => String(n).padStart(2, '0')

/** Seconds east of UTC → `UTC`, `UTC+08:00`, `UTC-03:30` (the server's format for zones without a name). */
export function offsetName(offset: number): string {
  if (offset === 0) return 'UTC'
  const abs = Math.abs(offset)
  return `UTC${offset < 0 ? '-' : '+'}${pad(Math.floor(abs / 3600))}:${pad(Math.floor((abs % 3600) / 60))}`
}
