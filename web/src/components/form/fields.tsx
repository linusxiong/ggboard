/**
 * HeroUI v3 ↔ TanStack Form bridge. Used through `useAppForm` (see ./index.ts):
 *
 *   <form.AppField name="email" validators={{ onBlur: ({ value }) => (!value ? t('required') : undefined) }}>
 *     {(field) => <field.TextField label={t('email')} type="email" autoComplete="email" />}
 *   </form.AppField>
 *
 * Every field: visible `<Label>`, `name`/`id` for autofill, `isInvalid` + `<FieldError>` from the
 * field meta, `onBlur` → `field.handleBlur()`. Server errors set via `applyServerErrors()` clear
 * as soon as the user edits the field.
 */
import {
  Button,
  Description,
  FieldError,
  Input,
  InputGroup,
  Label,
  ListBox,
  NumberField as HeroNumberField,
  Select,
  Switch,
  TextArea,
  TextField as HeroTextField,
} from '@heroui/react'
import type { ReactNode } from 'react'
import type { AnyFieldApi } from '@tanstack/react-form'
import { useFieldContext, useFormContext } from './context'
import { fieldErrorMessage } from './fieldError'

/** Remove a server-side error once the user changes the value. */
function clearServerError(field: AnyFieldApi) {
  if (field.state.meta.errorMap.onServer) {
    field.setMeta((meta) => ({ ...meta, errorMap: { ...meta.errorMap, onServer: undefined } }))
  }
}

interface BaseFieldProps {
  label: ReactNode
  description?: ReactNode
  isRequired?: boolean
  isDisabled?: boolean
  className?: string
}

export interface TextFieldProps extends BaseFieldProps {
  type?: 'text' | 'email' | 'password' | 'url' | 'tel' | 'search'
  placeholder?: string
  autoComplete?: string
  inputMode?: 'text' | 'email' | 'numeric' | 'decimal' | 'tel' | 'url' | 'search'
  autoFocus?: boolean
  maxLength?: number
  /** Content rendered inside the input group, after the input (e.g. a "send code" button). */
  endContent?: ReactNode
  startContent?: ReactNode
}

export function TextField({
  label,
  description,
  isRequired,
  isDisabled,
  className,
  type = 'text',
  placeholder,
  autoComplete,
  inputMode,
  autoFocus,
  maxLength,
  endContent,
  startContent,
}: TextFieldProps) {
  const field = useFieldContext<string>()
  const error = fieldErrorMessage(field.state.meta)
  const grouped = Boolean(endContent || startContent)
  const inputProps = {
    placeholder,
    autoComplete,
    inputMode,
    maxLength,
    className: 'w-full',
  }
  return (
    <HeroTextField
      name={field.name}
      id={field.name}
      type={type}
      value={field.state.value ?? ''}
      onChange={(value) => {
        field.handleChange(value)
        clearServerError(field)
      }}
      onBlur={field.handleBlur}
      isInvalid={Boolean(error)}
      isRequired={isRequired}
      isDisabled={isDisabled}
      autoFocus={autoFocus}
      validationBehavior="aria"
      fullWidth
      className={className}
    >
      <Label>{label}</Label>
      {grouped ? (
        <InputGroup fullWidth>
          {startContent ? <InputGroup.Prefix>{startContent}</InputGroup.Prefix> : null}
          <InputGroup.Input {...inputProps} />
          {endContent ? <InputGroup.Suffix>{endContent}</InputGroup.Suffix> : null}
        </InputGroup>
      ) : (
        <Input {...inputProps} />
      )}
      {description && !error ? <Description>{description}</Description> : null}
      <FieldError>{error}</FieldError>
    </HeroTextField>
  )
}

export interface TextAreaFieldProps extends BaseFieldProps {
  placeholder?: string
  rows?: number
  maxLength?: number
}

export function TextAreaField({
  label,
  description,
  isRequired,
  isDisabled,
  className,
  placeholder,
  rows = 4,
  maxLength,
}: TextAreaFieldProps) {
  const field = useFieldContext<string>()
  const error = fieldErrorMessage(field.state.meta)
  return (
    <HeroTextField
      name={field.name}
      id={field.name}
      value={field.state.value ?? ''}
      onChange={(value) => {
        field.handleChange(value)
        clearServerError(field)
      }}
      onBlur={field.handleBlur}
      isInvalid={Boolean(error)}
      isRequired={isRequired}
      isDisabled={isDisabled}
      validationBehavior="aria"
      fullWidth
      className={className}
    >
      <Label>{label}</Label>
      <TextArea placeholder={placeholder} rows={rows} maxLength={maxLength} className="w-full" />
      {description && !error ? <Description>{description}</Description> : null}
      <FieldError>{error}</FieldError>
    </HeroTextField>
  )
}

export interface NumberFieldProps extends BaseFieldProps {
  minValue?: number
  maxValue?: number
  step?: number
  formatOptions?: Intl.NumberFormatOptions
  /** Show −/+ steppers (default true). */
  steppers?: boolean
}

export function NumberField({
  label,
  description,
  isRequired,
  isDisabled,
  className,
  minValue,
  maxValue,
  step,
  formatOptions,
  steppers = true,
}: NumberFieldProps) {
  const field = useFieldContext<number | null>()
  const error = fieldErrorMessage(field.state.meta)
  return (
    <HeroNumberField
      name={field.name}
      id={field.name}
      value={field.state.value ?? Number.NaN}
      onChange={(value) => {
        field.handleChange(Number.isNaN(value) ? null : value)
        clearServerError(field)
      }}
      onBlur={field.handleBlur}
      isInvalid={Boolean(error)}
      isRequired={isRequired}
      isDisabled={isDisabled}
      minValue={minValue}
      maxValue={maxValue}
      step={step}
      formatOptions={formatOptions}
      validationBehavior="aria"
      className={className}
    >
      <Label>{label}</Label>
      <HeroNumberField.Group>
        {steppers ? <HeroNumberField.DecrementButton /> : null}
        <HeroNumberField.Input className="w-full" />
        {steppers ? <HeroNumberField.IncrementButton /> : null}
      </HeroNumberField.Group>
      {description && !error ? <Description>{description}</Description> : null}
      <FieldError>{error}</FieldError>
    </HeroNumberField>
  )
}

export interface SelectOption {
  value: string
  label: ReactNode
  /** Plain text for type-ahead when `label` is not a string. */
  textValue?: string
  isDisabled?: boolean
}

export interface SelectFieldProps extends BaseFieldProps {
  options: readonly SelectOption[]
  placeholder?: string
}

export function SelectField({
  label,
  description,
  isRequired,
  isDisabled,
  className,
  options,
  placeholder,
}: SelectFieldProps) {
  const field = useFieldContext<string | null>()
  const error = fieldErrorMessage(field.state.meta)
  return (
    <Select
      name={field.name}
      id={field.name}
      value={field.state.value ?? null}
      onChange={(key) => {
        field.handleChange(key === null ? null : String(key))
        clearServerError(field)
      }}
      onBlur={field.handleBlur}
      isInvalid={Boolean(error)}
      isRequired={isRequired}
      isDisabled={isDisabled}
      placeholder={placeholder}
      validationBehavior="aria"
      disabledKeys={options.filter((o) => o.isDisabled).map((o) => o.value)}
      className={className}
    >
      <Label>{label}</Label>
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      {description && !error ? <Description>{description}</Description> : null}
      <FieldError>{error}</FieldError>
      <Select.Popover>
        <ListBox>
          {options.map((option) => (
            <ListBox.Item
              key={option.value}
              id={option.value}
              textValue={option.textValue ?? (typeof option.label === 'string' ? option.label : option.value)}
            >
              {option.label}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

export type SwitchFieldProps = Omit<BaseFieldProps, 'isRequired'>

export function SwitchField({ label, description, isDisabled, className }: SwitchFieldProps) {
  const field = useFieldContext<boolean>()
  const error = fieldErrorMessage(field.state.meta)
  return (
    <Switch
      name={field.name}
      id={field.name}
      isSelected={Boolean(field.state.value)}
      onChange={(value) => {
        field.handleChange(value)
        clearServerError(field)
      }}
      onBlur={field.handleBlur}
      isInvalid={Boolean(error)}
      isDisabled={isDisabled}
      className={className}
    >
      {/* HeroUI ≥ 3.2 anatomy: Control inside Content, plain-text label, Description/FieldError as siblings */}
      <Switch.Content>
        <Switch.Control>
          <Switch.Thumb />
        </Switch.Control>
        {label}
      </Switch.Content>
      {description ? <Description>{description}</Description> : null}
      <FieldError>{error}</FieldError>
    </Switch>
  )
}

// ---------------------------------------------------------------- form-level components

export interface SubmitButtonProps {
  children: ReactNode
  className?: string
  fullWidth?: boolean
  size?: 'sm' | 'md' | 'lg'
  variant?: 'primary' | 'secondary' | 'danger'
}

/** Disabled while submitting / when the form cannot submit; shows a spinner while pending. */
export function SubmitButton({
  children,
  className,
  fullWidth,
  size = 'lg',
  variant = 'primary',
}: SubmitButtonProps) {
  const form = useFormContext()
  return (
    <form.Subscribe selector={(s) => [s.isSubmitting, s.canSubmit] as const}>
      {([isSubmitting, canSubmit]) => (
        <Button
          type="submit"
          size={size}
          variant={variant}
          fullWidth={fullWidth}
          isPending={isSubmitting}
          isDisabled={!canSubmit && !isSubmitting}
          className={className}
        >
          {children}
        </Button>
      )}
    </form.Subscribe>
  )
}
