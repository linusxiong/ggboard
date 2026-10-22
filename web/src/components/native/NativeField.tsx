/**
 * Native `<input>` field with HeroUI v3's TextField markup and CSS (`.textfield`, `.label`,
 * `.input`, `.description`, `.field-error`) but no React Aria JS.
 *
 * Used on the public pages (landing / login / register / forgot) to keep their first-load JS
 * under the 120 KB budget (AGENTS.md §6). Authenticated pages use the full HeroUI components
 * through `@/components/form` instead.
 */
import 'heroui-css:textfield'
import 'heroui-css:label'
import 'heroui-css:input'
import 'heroui-css:description'
import 'heroui-css:field-error'
import { type InputHTMLAttributes, type ReactNode, useId } from 'react'
import { cx } from '@/lib/cn'

export interface NativeFieldProps extends Omit<InputHTMLAttributes<HTMLInputElement>, 'children'> {
  name: string
  label: ReactNode
  description?: ReactNode
  /** Error message; marks the field invalid (`aria-invalid`) and announces it. */
  error?: string | undefined
  /** Rendered to the right of the input (e.g. "send code" button). */
  end?: ReactNode
  className?: string
}

export function NativeField({
  name,
  label,
  description,
  error,
  end,
  required,
  className,
  id,
  ...input
}: NativeFieldProps) {
  const auto = useId()
  const inputId = id ?? `${name}-${auto}`
  const descId = `${inputId}-desc`
  const errId = `${inputId}-err`
  const invalid = Boolean(error)
  const describedBy =
    cx(description && !invalid ? descId : undefined, invalid ? errId : undefined) || undefined

  const control = (
    <input
      {...input}
      id={inputId}
      name={name}
      required={required}
      aria-required={required || undefined}
      aria-invalid={invalid || undefined}
      aria-describedby={describedBy}
      data-slot="input"
      data-invalid={invalid || undefined}
      className={cx('input input--full-width min-w-0', end ? 'flex-1' : undefined)}
    />
  )

  return (
    <div
      className={cx('textfield textfield--full-width', className)}
      data-invalid={invalid || undefined}
      data-required={required || undefined}
    >
      <label htmlFor={inputId} className="label">
        {label}
      </label>
      {end ? (
        <div className="flex items-stretch gap-2">
          {control}
          {end}
        </div>
      ) : (
        control
      )}
      {description && !invalid ? (
        <p id={descId} data-slot="description" className="description px-1">
          {description}
        </p>
      ) : null}
      <p id={errId} className="field-error" data-visible={invalid || undefined} aria-live="polite">
        {error}
      </p>
    </div>
  )
}
