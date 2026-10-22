/** `commission_withdraw_method` is not exposed to users; these match its default (docs/API.md). */
export const WITHDRAW_METHOD_SUGGESTIONS = ['USDT', 'Alipay'] as const

/** `<app_url>/register?code=<code>` — the registration page reads `?code=` (web/README.md). */
export function buildInviteLink(appUrl: string, code: string): string {
  const base = appUrl.trim().replace(/\/+$/, '')
  return `${base}/register?code=${encodeURIComponent(code)}`
}

/** Parses a decimal currency amount ("12.5") into cents; `null` when invalid or not positive. */
export function parseAmount(value: number | null | undefined): number | null {
  if (value === null || value === undefined || !Number.isFinite(value)) return null
  const cents = Math.round(value * 100)
  return cents > 0 ? cents : null
}
