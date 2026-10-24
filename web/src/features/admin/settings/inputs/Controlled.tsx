/**
 * Controlled HeroUI fields (value/onChange/error) for metadata-driven forms (settings, gateway
 * config) where the set of fields is only known at runtime. Same anatomy as `@/components/form`:
 * visible `<Label>`, `name`/`id`, `isInvalid` + `<FieldError>`, description behind the label's ⓘ.
 */
import {
  FieldError,
  Input,
  InputGroup,
  ListBox,
  NumberField,
  Select,
  Switch,
  TextArea,
  TextField,
} from '@heroui/react'
import type { ReactNode } from 'react'
import { FieldAffix, FieldHint, FieldLabel, SelectItemContent, SwitchHelp } from '@/components/form/parts'
import { labelWithUnit } from '@/components/form/unit'
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
      <FieldLabel label={label} help={description} />
      {startContent || endContent ? (
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
      <FieldLabel label={label} help={description} />
      <TextArea
        rows={rows}
        placeholder={placeholder}
        spellCheck={monospace ? false : undefined}
        className={cx('w-full', monospace && 'font-mono text-footnote')}
      />
      {error ? null : <FieldHint>{description}</FieldHint>}
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
  /** Unit inside the field after the number (分钟, GB, %). */
  unit?: ReactNode
  /** Shown before the number (currency symbol). */
  prefix?: ReactNode
  placeholder?: string
  /** −/+ steppers; off by default (plain numeric input). */
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
  unit,
  prefix,
  placeholder,
  steppers = false,
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
      <FieldLabel
        label={labelWithUnit(label, unit)}
        subject={typeof label === 'string' ? label : undefined}
        help={description}
      />
      <NumberField.Group>
        {steppers ? <NumberField.DecrementButton /> : null}
        {prefix ? <FieldAffix start>{prefix}</FieldAffix> : null}
        <NumberField.Input className="w-full" placeholder={placeholder} />
        {unit ? <FieldAffix>{unit}</FieldAffix> : null}
        {steppers ? <NumberField.IncrementButton /> : null}
      </NumberField.Group>
      {error ? null : <FieldHint>{description}</FieldHint>}
      <FieldError>{error}</FieldError>
    </NumberField>
  )
}

export interface SelectInputOption {
  value: string
  label: ReactNode
  /** Second line in the list. */
  description?: ReactNode
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
      <FieldLabel label={label} help={description} />
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      {error ? null : <FieldHint>{description}</FieldHint>}
      <FieldError>{error}</FieldError>
      <Select.Popover>
        <ListBox>
          {options.map((o) => (
            <ListBox.Item
              key={o.value}
              id={o.value}
              textValue={o.textValue ?? (typeof o.label === 'string' ? o.label : o.value)}
            >
              <SelectItemContent label={o.label} description={o.description} />
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
      <FieldLabel label={label} help={description} />
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      {error ? null : <FieldHint>{description}</FieldHint>}
      <FieldError>{error}</FieldError>
      <Select.Popover>
        <ListBox selectionMode="multiple">
          {options.map((o) => (
            <ListBox.Item
              key={o.value}
              id={o.value}
              textValue={o.textValue ?? (typeof o.label === 'string' ? o.label : o.value)}
            >
              <SelectItemContent label={o.label} description={o.description} />
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
  /** `row` (default): title + caption left, switch right. `inline`: switch first. */
  layout?: 'row' | 'inline'
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
  layout = 'row',
}: SwitchInputProps) {
  return (
    <SwitchHelp
      help={description}
      label={label}
      layout={layout}
      className={description ? className : undefined}
    >
      <Switch
        name={name}
        id={name}
        isSelected={value}
        onChange={onChange}
        isInvalid={Boolean(error)}
        isDisabled={isDisabled}
        className={cx(layout === 'row' && 'switch-row', !description && className)}
      >
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
