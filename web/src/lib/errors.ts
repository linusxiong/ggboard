import { isApiError } from '@/lib/api/client'
import { t } from '@/i18n'

/**
 * Human message for any thrown value. Server messages win for validation/conflict errors
 * (they are specific); otherwise a localized message per error code (`common:errors.*`).
 */
export function errorMessage(error: unknown): string {
  if (isApiError(error)) {
    if (
      (error.code === 'validation_failed' || error.code === 'conflict' || error.code === 'forbidden') &&
      error.message
    )
      return error.message
    const key = `errors.${error.code}`
    const text = t('core', key)
    if (text !== key) return text
    return error.message || t('core', 'errors.unknown')
  }
  if (error instanceof Error && error.message) return error.message
  return t('core', 'errors.unknown')
}
