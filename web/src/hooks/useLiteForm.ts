/**
 * Minimal form state for the public auth pages (no TanStack Form there — first-load budget).
 * Feature pages should use `useAppForm` from `@/components/form`.
 *
 *   const form = useLiteForm({ email: '', password: '' }, { email: (v) => validateEmail(v) })
 *   <NativeField {...form.field('email')} label="Email" />
 *   <form onSubmit={form.handleSubmit(async (values) => { … })}>
 */
import { type FormEvent, useCallback, useEffect, useRef, useState } from 'react'

type Values = Record<string, string>
type Validator<V extends Values> = (value: string, values: V) => string | undefined
export type LiteValidators<V extends Values> = Partial<Record<keyof V, Validator<V>>>

export function useLiteForm<V extends Values>(initial: V, validators: LiteValidators<V> = {}) {
  const [values, setValues] = useState<V>(initial)
  const [errors, setErrors] = useState<Partial<Record<keyof V, string>>>({})
  const [submitting, setSubmitting] = useState(false)
  const valuesRef = useRef(values)
  const validatorsRef = useRef(validators)
  useEffect(() => {
    valuesRef.current = values
    validatorsRef.current = validators
  })

  const validateField = useCallback((name: keyof V): string | undefined => {
    const error = validatorsRef.current[name]?.(valuesRef.current[name] ?? '', valuesRef.current)
    setErrors((prev) => ({ ...prev, [name]: error }))
    return error
  }, [])

  const setError = useCallback((name: keyof V, error: string | undefined) => {
    setErrors((prev) => ({ ...prev, [name]: error }))
  }, [])

  const field = (name: keyof V & string) => ({
    name,
    value: values[name] ?? '',
    error: errors[name],
    onChange: (event: { currentTarget: { value: string } }) => {
      const value = event.currentTarget.value
      valuesRef.current = { ...valuesRef.current, [name]: value }
      setValues((prev) => ({ ...prev, [name]: value }))
      if (errors[name]) setErrors((prev) => ({ ...prev, [name]: undefined }))
    },
    onBlur: () => {
      if (valuesRef.current[name]) validateField(name)
    },
  })

  const validateAll = (): boolean => {
    let ok = true
    const next: Partial<Record<keyof V, string>> = {}
    for (const name of Object.keys(validatorsRef.current) as (keyof V)[]) {
      const error = validatorsRef.current[name]?.(valuesRef.current[name] ?? '', valuesRef.current)
      next[name] = error
      if (error) ok = false
    }
    setErrors(next)
    return ok
  }

  const handleSubmit = (onValid: (values: V) => Promise<void> | void) => async (event: FormEvent) => {
    event.preventDefault()
    if (submitting) return
    const formEl = event.currentTarget as HTMLFormElement
    if (!validateAll()) {
      // Focus the first invalid control once the error state has rendered.
      requestAnimationFrame(() => formEl.querySelector<HTMLElement>('[aria-invalid="true"]')?.focus())
      return
    }
    setSubmitting(true)
    try {
      await onValid(valuesRef.current)
    } finally {
      setSubmitting(false)
    }
  }

  return { values, errors, submitting, field, setError, validateField, handleSubmit, setValues }
}
