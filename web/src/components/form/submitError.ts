import type { AnyFormApi } from '@tanstack/react-form'
import { isApiError } from '@/lib/api/client'
import { errorMessage } from '@/lib/errors'
import { applyServerErrors } from './index'

/**
 * Standard `onSubmit` catch for `useAppForm` forms: field errors go onto fields
 * (`errorMap.onServer`), everything else becomes one form-level message — localized for
 * transport errors, the server's text for business errors.
 *
 *   catch (e) { setFormError(submitError(formApi, e)) }
 */
export function submitError(form: AnyFormApi, error: unknown): string | undefined {
  if (isApiError(error) && (error.isNetwork || error.code === 'internal' || error.code === 'rate_limited'))
    return errorMessage(error)
  if (isApiError(error) && Object.keys(error.errors).length > 0) return applyServerErrors(form, error)
  return applyServerErrors(form, error) ?? errorMessage(error)
}
