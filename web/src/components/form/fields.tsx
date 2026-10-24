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
  FieldError,
  Input,
  InputGroup,
  ListBox,
  NumberField as HeroNumberField,
  Select,
  Switch,
  TextArea,
  TextField as HeroTextField,
} from '@heroui/react'
import type { ReactNode } from 'react'
import type { AnyFieldApi } from '@tanstack/react-form'
import { cx } from '@/lib/cn'
import { useFieldContext, useFormContext } from './context'
import { fieldErrorMessage } from './fieldError'
import { TagInput, type TagInputProps } from './TagInput'
import { FieldAffix, FieldHint, FieldLabel, SelectItemContent, SwitchHelp } from './parts'
import { labelWithUnit } from './unit'

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
      <FieldLabel label={label} help={description} />
      {grouped ? (
        <InputGroup fullWidth>
          {startContent ? <InputGroup.Prefix>{startContent}</InputGroup.Prefix> : null}
          <InputGroup.Input {...inputProps} />
          {endContent ? <InputGroup.Suffix>{endContent}</InputGroup.Suffix> : null}
        </InputGroup>
      ) : (
        <Input {...inputProps} />
      )}
      {error ? null : <FieldHint>{description}</FieldHint>}
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
      <FieldLabel label={label} help={description} />
      <TextArea placeholder={placeholder} rows={rows} maxLength={maxLength} className="w-full" />
      {error ? null : <FieldHint>{description}</FieldHint>}
      <FieldError>{error}</FieldError>
    </HeroTextField>
  )
}

export interface NumberFieldProps extends BaseFieldProps {
  minValue?: number
  maxValue?: number
  step?: number
  formatOptions?: Intl.NumberFormatOptions
  /** Unit shown inside the field, after the number (GB, Mbps, 天, %). Also read out with the label. */
  unit?: ReactNode
  /** Shown before the number (currency symbol: `¥` → `¥ 15.00`). */
  prefix?: ReactNode
  placeholder?: string
  /** Show −/+ steppers (default false — plain numeric input, arrow keys still step). */
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
  unit,
  prefix,
  placeholder,
  steppers = false,
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
      <FieldLabel
        label={labelWithUnit(label, unit)}
        subject={typeof label === 'string' ? label : undefined}
        help={description}
      />
      <HeroNumberField.Group>
        {steppers ? <HeroNumberField.DecrementButton /> : null}
        {prefix ? <FieldAffix start>{prefix}</FieldAffix> : null}
        <HeroNumberField.Input className="w-full" placeholder={placeholder} />
        {unit ? <FieldAffix>{unit}</FieldAffix> : null}
        {steppers ? <HeroNumberField.IncrementButton /> : null}
      </HeroNumberField.Group>
      {error ? null : <FieldHint>{description}</FieldHint>}
      <FieldError>{error}</FieldError>
    </HeroNumberField>
  )
}

export interface SelectOption {
  value: string
  label: ReactNode
  /** Second line in the list (what the option does). */
  description?: ReactNode
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
      <FieldLabel label={label} help={description} />
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      {error ? null : <FieldHint>{description}</FieldHint>}
      <FieldError>{error}</FieldError>
      <Select.Popover>
        <ListBox>
          {options.map((option) => (
            <ListBox.Item
              key={option.value}
              id={option.value}
              textValue={option.textValue ?? (typeof option.label === 'string' ? option.label : option.value)}
            >
              <SelectItemContent label={option.label} description={option.description} />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

export interface SwitchFieldProps extends Omit<BaseFieldProps, 'isRequired'> {
  /** `row` (default): title + caption left, switch right (Settings style). `inline`: switch first. */
  layout?: 'row' | 'inline'
}

export function SwitchField({ label, description, isDisabled, className, layout = 'row' }: SwitchFieldProps) {
  const field = useFieldContext<boolean>()
  const error = fieldErrorMessage(field.state.meta)
  return (
    <SwitchHelp
      help={description}
      label={label}
      layout={layout}
      className={description ? className : undefined}
    >
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
        className={cx(layout === 'row' && 'switch-row', !description && className)}
      >
        {/* HeroUI ≥ 3.2 anatomy: Control inside Content, plain-text label, Description/FieldError as siblings */}
        <Switch.Content>
          <Switch.Control>
            <Switch.Thumb />
          </Switch.Control>
          {label}
        </Switch.Content>
        <FieldHint>{description}</FieldHint>
        <FieldError>{error}</FieldError>
      </Switch>
    </SwitchHelp>
  )
}

export type TagsFieldProps = Omit<TagInputProps, 'value' | 'onChange' | 'name' | 'error' | 'onBlur'>

/** String-list field (chips + input): Enter/comma adds, paste splits, Backspace removes. */
export function TagsField(props: TagsFieldProps) {
  const field = useFieldContext<string[]>()
  const error = fieldErrorMessage(field.state.meta)
  return (
    <TagInput
      {...props}
      name={field.name}
      id={field.name}
      value={field.state.value ?? []}
      onChange={(next) => {
        field.handleChange(next)
        clearServerError(field)
      }}
      onBlur={field.handleBlur}
      error={error}
    />
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
