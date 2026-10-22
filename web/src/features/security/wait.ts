/**
 * "15 分钟后" / "in 15 minutes" for a `Retry-After` delay. Tiny on purpose: the login page uses it and
 * must stay inside the first-load budget (no `lib/format` there).
 */
export function waitText(seconds: number, locale: string): string {
  const rtf = new Intl.RelativeTimeFormat(locale, { numeric: 'always' })
  if (seconds < 60) return rtf.format(Math.max(1, Math.ceil(seconds)), 'second')
  if (seconds < 3600) return rtf.format(Math.ceil(seconds / 60), 'minute')
  return rtf.format(Math.ceil(seconds / 3600), 'hour')
}
