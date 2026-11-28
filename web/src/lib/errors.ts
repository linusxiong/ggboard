import { isApiError } from '@/lib/api/client'
import { t } from '@/i18n'

/** Codes whose server message says more than the generic text (the server writes it in the UI language). */
const SPECIFIC = new Set(['validation_failed', 'conflict', 'forbidden', 'payment_failed', 'rate_limited'])

/**
 * Human message for any thrown value. The server's message wins for business errors (it is specific and
 * localized from `Accept-Language`); otherwise a localized message per error code (`core:errors.*`).
 */
export function errorMessage(error: unknown): string {
  if (isApiError(error)) {
    if (SPECIFIC.has(error.code) && error.message) return error.message
    const key = `errors.${error.code}`
    const text = t('core', key)
    if (text !== key) return text
    return error.message || t('core', 'errors.unknown')
  }
  if (error instanceof Error && error.message) return error.message
  return t('core', 'errors.unknown')
}
