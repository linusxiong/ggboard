/**
 * Controlled HeroUI fields (value/onChange/error) for metadata-driven forms (settings, gateway
 * config) where the set of fields is only known at runtime. Same anatomy as `@/components/form`:
 * visible `<Label>`, `name`/`id`, `isInvalid` + `<FieldError>`, description hidden while invalid.
 */
import {
  Description,
  FieldError,
  Input,
  InputGroup,
  Label,
  ListBox,
  NumberField,
  Select,
  Switch,
  TextArea,
  TextField,
} from '@heroui/react'
import type { ReactNode } from 'react'
import { cx } from '@/lib/cn'

interface BaseProps {
  label: ReactNode
  name?: string
  description?: ReactNode
  error?: string
  isRequired?: boolean
  isDisabled?: boolean
  className?: string
}

export interface TextInputProps extends BaseProps {
  value: string
  onChange: (value: string) => void
  type?: 'text' | 'email' | 'url' | 'date' | 'datetime-local' | 'search' | 'tel'
  placeholder?: string
  monospace?: boolean
  inputMode?: 'text' | 'decimal' | 'numeric' | 'email' | 'url'
  autoComplete?: string
  startContent?: ReactNode
  endContent?: ReactNode
  maxLength?: number
  autoFocus?: boolean
  isReadOnly?: boolean
}

export function TextInput({
  label,
  name,
  description,
  error,
  isRequired,
  isDisabled,
  className,
  value,
  onChange,
  type = 'text',
  placeholder,
  monospace,
  inputMode,
  autoComplete = 'off',
  startContent,
  endContent,
  maxLength,
  autoFocus,
  isReadOnly,
}: TextInputProps) {
  const inputProps = {
    type,
    placeholder,
    inputMode,
    autoComplete,
    maxLength,
    className: cx('w-full', monospace && 'font-mono'),
  }
  return (
    <TextField
      name={name}
      id={name}
      value={value}
      onChange={onChange}
      isInvalid={Boolean(error)}
      isRequired={isRequired}
      isDisabled={isDisabled}
      isReadOnly={isReadOnly}
      autoFocus={autoFocus}
      validationBehavior="aria"
      fullWidth
      className={className}
    >
      <Label>{label}</Label>
      {startContent || endContent ? (
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
    </TextField>
  )
}

export interface TextAreaInputProps extends BaseProps {
  value: string
  onChange: (value: string) => void
  rows?: number
  placeholder?: string
  monospace?: boolean
}

export function TextAreaInput({
  label,
  name,
  description,
  error,
  isRequired,
  isDisabled,
  className,
  value,
  onChange,
  rows = 4,
  placeholder,
  monospace,
}: TextAreaInputProps) {
  return (
    <TextField
      name={name}
      id={name}
      value={value}
      onChange={onChange}
      isInvalid={Boolean(error)}
      isRequired={isRequired}
      isDisabled={isDisabled}
      validationBehavior="aria"
      fullWidth
      className={className}
    >
      <Label>{label}</Label>
      <TextArea
        rows={rows}
        placeholder={placeholder}
        spellCheck={monospace ? false : undefined}
        className={cx('w-full', monospace && 'font-mono text-footnote')}
      />
      {description && !error ? <Description>{description}</Description> : null}
      <FieldError>{error}</FieldError>
    </TextField>
  )
}

export interface NumberInputProps extends BaseProps {
  value: number | null
  onChange: (value: number | null) => void
  minValue?: number
  maxValue?: number
  step?: number
  formatOptions?: Intl.NumberFormatOptions
  steppers?: boolean
}

export function NumberInput({
  label,
  name,
  description,
  error,
  isRequired,
  isDisabled,
  className,
  value,
  onChange,
  minValue,
  maxValue,
  step,
  formatOptions,
  steppers = true,
}: NumberInputProps) {
  return (
    <NumberField
      name={name}
      id={name}
      value={value ?? Number.NaN}
      onChange={(v) => onChange(Number.isNaN(v) ? null : v)}
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
      <NumberField.Group>
        {steppers ? <NumberField.DecrementButton /> : null}
        <NumberField.Input className="w-full" />
        {steppers ? <NumberField.IncrementButton /> : null}
      </NumberField.Group>
      {description && !error ? <Description>{description}</Description> : null}
      <FieldError>{error}</FieldError>
    </NumberField>
  )
}

export interface SelectInputOption {
  value: string
  label: ReactNode
  textValue?: string
}

export interface SelectInputProps extends BaseProps {
  value: string | null
  onChange: (value: string | null) => void
  options: readonly SelectInputOption[]
  placeholder?: string
}

export function SelectInput({
  label,
  name,
  description,
  error,
  isRequired,
  isDisabled,
  className,
  value,
  onChange,
  options,
  placeholder,
}: SelectInputProps) {
  return (
    <Select
      name={name}
      id={name}
      value={value}
      onChange={(key) => onChange(key === null ? null : String(key))}
      isInvalid={Boolean(error)}
      isRequired={isRequired}
      isDisabled={isDisabled}
      placeholder={placeholder}
      validationBehavior="aria"
      className={className}
      fullWidth
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
          {options.map((o) => (
            <ListBox.Item
              key={o.value}
              id={o.value}
              textValue={o.textValue ?? (typeof o.label === 'string' ? o.label : o.value)}
            >
              {o.label}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

export interface MultiSelectInputProps extends BaseProps {
  value: readonly string[]
  onChange: (value: string[]) => void
  options: readonly SelectInputOption[]
  placeholder?: string
}

export function MultiSelectInput({
  label,
  name,
  description,
  error,
  isDisabled,
  className,
  value,
  onChange,
  options,
  placeholder,
}: MultiSelectInputProps) {
  return (
    <Select
      name={name}
      id={name}
      selectionMode="multiple"
      value={[...value]}
      onChange={(keys) => onChange(Array.isArray(keys) ? keys.map((k) => String(k)) : [])}
      isInvalid={Boolean(error)}
      isDisabled={isDisabled}
      placeholder={placeholder}
      validationBehavior="aria"
      className={className}
      fullWidth
    >
      <Label>{label}</Label>
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      {description && !error ? <Description>{description}</Description> : null}
      <FieldError>{error}</FieldError>
      <Select.Popover>
        <ListBox selectionMode="multiple">
          {options.map((o) => (
            <ListBox.Item
              key={o.value}
              id={o.value}
              textValue={o.textValue ?? (typeof o.label === 'string' ? o.label : o.value)}
            >
              {o.label}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

export interface SwitchInputProps extends Omit<BaseProps, 'isRequired'> {
  value: boolean
  onChange: (value: boolean) => void
}

export function SwitchInput({
  label,
  name,
  description,
  error,
  isDisabled,
  className,
  value,
  onChange,
}: SwitchInputProps) {
  return (
    <Switch
      name={name}
      id={name}
      isSelected={value}
      onChange={onChange}
      isInvalid={Boolean(error)}
      isDisabled={isDisabled}
      className={className}
    >
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
