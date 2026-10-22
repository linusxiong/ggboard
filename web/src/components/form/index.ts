/**
 * App form hook: TanStack Form + HeroUI field components.
 *
 *   const form = useAppForm({
 *     defaultValues: { email: '', password: '' },
 *     onSubmit: async ({ value }) => { … },
 *   })
 *   <form onSubmit={(e) => { e.preventDefault(); void form.handleSubmit() }} noValidate>
 *     <form.AppField name="email">{(f) => <f.TextField label="Email" type="email" />}</form.AppField>
 *     <form.AppForm><form.SubmitButton>Save</form.SubmitButton></form.AppForm>
 *   </form>
 */
import { createFormHook } from '@tanstack/react-form'
import type { AnyFormApi } from '@tanstack/react-form'
import { isApiError } from '@/lib/api/client'
import { fieldContext, formContext } from './context'
import { NumberField, SelectField, SubmitButton, SwitchField, TextAreaField, TextField } from './fields'

export const { useAppForm, withForm, withFieldGroup } = createFormHook({
  fieldContext,
  formContext,
  fieldComponents: { TextField, TextAreaField, NumberField, SelectField, SwitchField },
  formComponents: { SubmitButton },
})

export { FormError } from '@/components/FormError'
export type { SelectOption } from './fields'
export { useFieldContext, useFormContext } from './context'

/**
 * Maps an `ApiError` with `errors: {field: [msg]}` onto form fields (`errorMap.onServer`).
 * Returns the first message that did not match a field (show it with `<FormError>`), or the
 * error's own message when there were no field errors.
 */
export function applyServerErrors(form: AnyFormApi, error: unknown): string | undefined {
  if (!isApiError(error)) return error instanceof Error ? error.message : undefined
  const entries = Object.entries(error.errors)
  if (entries.length === 0) return error.message
  let unmatched: string | undefined
  for (const [name, messages] of entries) {
    const message = messages[0]
    if (!message) continue
    if (name in (form.state.values as Record<string, unknown>)) {
      form.setFieldMeta(name, (meta) => ({ ...meta, isTouched: true, errorMap: { ...meta.errorMap, onServer: message } }))
    } else {
      unmatched ??= message
    }
  }
  return unmatched
}
