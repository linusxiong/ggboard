import type { AnyFieldMeta } from '@tanstack/react-form'

/** Normalises TanStack Form errors (strings, Standard Schema issues, objects) to messages. */
export function errorText(errors: readonly unknown[]): string | undefined {
  for (const error of errors) {
    if (!error) continue
    if (typeof error === 'string') return error
    if (typeof error === 'object' && 'message' in error && typeof error.message === 'string') return error.message
  }
  return undefined
}

/** First visible error for a field (validators + server errors set by `applyServerErrors`). */
export function fieldErrorMessage(meta: AnyFieldMeta): string | undefined {
  return errorText(meta.errors)
}
