/** Small field validators returning an error message or `undefined` (TanStack Form style). */
import { t } from '@/i18n'

export const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]{2,}$/
export const PASSWORD_MIN = 8

export function validateEmail(value: string, whitelist: readonly string[] = []): string | undefined {
  const email = value.trim()
  if (!email) return t('auth', 'validation.emailRequired')
  if (!EMAIL_RE.test(email)) return t('auth', 'validation.emailInvalid')
  if (whitelist.length > 0) {
    const domain = email.slice(email.lastIndexOf('@') + 1).toLowerCase()
    if (!whitelist.some((suffix) => domain === suffix.toLowerCase().replace(/^@/, '')))
      return t('auth', 'validation.emailSuffix')
  }
  return undefined
}

export function validatePassword(value: string, min = PASSWORD_MIN): string | undefined {
  if (!value) return t('auth', 'validation.passwordRequired')
  if (value.length < min) return t('auth', 'validation.passwordMin', { min })
  return undefined
}

export function validateRequired(
  value: string | null | undefined,
  messageKey = 'validation.codeRequired',
): string | undefined {
  return value && value.trim() ? undefined : t('auth', messageKey)
}
