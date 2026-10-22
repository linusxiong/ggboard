/** Security query, error mapping and code formats (non-component helpers for the profile → security panels). */
import { queryOptions, useQueryClient } from '@tanstack/react-query'
import { useLocale, useT } from '@/i18n'
import { isApiError } from '@/lib/api/client'
import { qk } from '@/lib/api/queries'
import type { Locale } from '@/lib/api/types'
import { security } from '@/lib/api/user'
import { errorMessage } from '@/lib/errors'
import { waitText } from './wait'

export const securityKey = () => [...qk.user.all, 'security'] as const

export const securityQuery = () =>
  queryOptions({ queryKey: securityKey(), queryFn: ({ signal }) => security(signal) })

type T = (key: string, params?: Record<string, string | number>) => string

/**
 * Maps a security-endpoint error to field messages (`code`, `password`, `old_password`, `name`) and a
 * form-level message. Server texts are English, so known fields get localized messages.
 */
export function securityError(
  error: unknown,
  t: T,
  locale: Locale,
): {
  fields: Partial<Record<'code' | 'password' | 'old_password' | 'new_password' | 'name' | 'form', string>>
} {
  if (!isApiError(error)) return { fields: { form: errorMessage(error) } }
  if (error.status === 429) {
    return {
      fields: {
        form:
          error.retryAfter !== null
            ? t('errors.wait', { wait: waitText(error.retryAfter, locale) })
            : t('errors.tooMany'),
      },
    }
  }
  if (error.status === 409) return { fields: { form: t('errors.conflict') } }
  const fields: ReturnType<typeof securityError>['fields'] = {}
  if (error.fieldError('code')) fields.code = t('errors.codeWrong')
  if (error.fieldError('password')) fields.password = t('errors.passwordWrong')
  if (error.fieldError('old_password')) fields.old_password = t('errors.passwordWrong')
  const newPassword = error.fieldError('new_password')
  if (newPassword) fields.new_password = newPassword
  const name = error.fieldError('name')
  if (name) fields.name = name
  if (Object.keys(fields).length === 0) fields.form = errorMessage(error)
  return { fields }
}

/** `securityError` bound to the current locale; a 409 also refetches `/user/security` (stale state). */
export function useSecurityError() {
  const t = useT(['security', 'common'])
  const locale = useLocale()
  const queryClient = useQueryClient()
  return (error: unknown) => {
    if (isApiError(error) && error.status === 409)
      void queryClient.invalidateQueries({ queryKey: securityKey() })
    return securityError(error, t, locale).fields
  }
}

export const TOTP_RE = /^\d{6}$/
export const CODE_OR_BACKUP_RE = /^(\d{6}|[a-z0-9]{5}-?[a-z0-9]{5})$/i
