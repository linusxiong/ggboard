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
import {
  NumberField,
  SelectField,
  SubmitButton,
  SwitchField,
  TagsField,
  TextAreaField,
  TextField,
} from './fields'
import { focusFirstInvalid } from './focus'

const hook = createFormHook({
  fieldContext,
  formContext,
  fieldComponents: { TextField, TextAreaField, NumberField, SelectField, SwitchField, TagsField },
  formComponents: { SubmitButton },
})

/**
 * `createFormHook`'s `useAppForm`, plus inline validation UX: when a submit fails validation, the
 * first invalid field is scrolled into view and focused (override with your own `onSubmitInvalid`).
 */
export const useAppForm: typeof hook.useAppForm = (options) =>
  hook.useAppForm({ onSubmitInvalid: () => focusFirstInvalid(), ...options })
export const { withForm, withFieldGroup } = hook

export { FormError } from '@/components/FormError'
export type { SelectOption } from './fields'
export { FieldAffix, SelectItemContent } from './parts'
export { labelWithUnit } from './unit'
export { TagInput } from './TagInput'
export { splitTagText } from './tags'
export { focusFirstInvalid } from './focus'
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
  let matched = false
  for (const [name, messages] of entries) {
    const message = messages[0]
    if (!message) continue
    if (name in (form.state.values as Record<string, unknown>)) {
      form.setFieldMeta(name, (meta) => ({
        ...meta,
        isTouched: true,
        errorMap: { ...meta.errorMap, onServer: message },
      }))
      matched = true
    } else {
      unmatched ??= message
    }
  }
  if (matched) focusFirstInvalid()
  return unmatched
}
