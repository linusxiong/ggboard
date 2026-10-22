import { isApiError } from '@/lib/api/client'
import { errorMessage } from '@/lib/errors'

/**
 * Splits an auth API error into per-field messages (`errors` map) and a form-level message.
 * Transport errors get localized text; business errors keep the server's message.
 */
export function splitAuthError(error: unknown): {
  fields: Record<string, string>
  message: string | undefined
} {
  const fields: Record<string, string> = {}
  if (!isApiError(error)) return { fields, message: errorMessage(error) }
  for (const [name, messages] of Object.entries(error.errors)) if (messages[0]) fields[name] = messages[0]
  if (error.isNetwork || error.code === 'internal' || error.code === 'rate_limited')
    return { fields, message: errorMessage(error) }
  return {
    fields,
    message: Object.keys(fields).length > 0 ? undefined : error.message || errorMessage(error),
  }
}
