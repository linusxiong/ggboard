/**
 * Extra TanStack Form field components for the admin console (bind with `form.AppField` +
 * `useFieldContext`, like `@/components/form` fields):
 *   <form.AppField name="expired_at">{() => <DateTimeField label="到期时间" />}</form.AppField>
 */
import {
  Button,
  Description,
  FieldError,
  Input,
  Label,
  ListBox,
  Select,
  TextField as HeroTextField,
} from '@heroui/react'
import type { ReactNode } from 'react'
import { useFieldContext } from '@/components/form'
import { fieldErrorMessage } from '@/components/form/fieldError'
import type { SelectOption } from '@/components/form'
import type { AnyFieldApi } from '@tanstack/react-form'

function clearServerError(field: AnyFieldApi) {
  if (field.state.meta.errorMap.onServer) {
    field.setMeta((meta) => ({ ...meta, errorMap: { ...meta.errorMap, onServer: undefined } }))
  }
}

interface DateTimeFieldProps {
  label: ReactNode
  description?: ReactNode
  /** Label of the "clear" button (value `''` = null, e.g. "永不过期"). Omit to hide the button. */
  clearLabel?: string
  isDisabled?: boolean
  className?: string
}

/** Native `datetime-local` input in HeroUI TextField chrome. Value: `YYYY-MM-DDTHH:mm` or `''`. */
export function DateTimeField({ label, description, clearLabel, isDisabled, className }: DateTimeFieldProps) {
  const field = useFieldContext<string>()
  const error = fieldErrorMessage(field.state.meta)
  const value = field.state.value ?? ''
  return (
    <HeroTextField
      name={field.name}
      id={field.name}
      value={value}
      onChange={(v) => {
        field.handleChange(v)
        clearServerError(field)
      }}
      onBlur={field.handleBlur}
      isInvalid={Boolean(error)}
      isDisabled={isDisabled}
      validationBehavior="aria"
      fullWidth
      className={className}
    >
      <Label>{label}</Label>
      <div className="flex items-center gap-2">
        <Input type="datetime-local" className="w-full min-w-0 flex-1" />
        {clearLabel ? (
          <Button
            size="sm"
            variant="tertiary"
            isDisabled={isDisabled || value === ''}
            onPress={() => {
              field.handleChange('')
              clearServerError(field)
            }}
          >
            {clearLabel}
          </Button>
        ) : null}
      </div>
      {description && !error ? <Description>{description}</Description> : null}
      <FieldError>{error}</FieldError>
    </HeroTextField>
  )
}

interface MultiSelectFieldProps {
  label: ReactNode
  description?: ReactNode
  options: readonly SelectOption[]
  placeholder?: string
  isDisabled?: boolean
  className?: string
}

/** Multiple selection (HeroUI Select `selectionMode="multiple"`). Value: `string[]` of option values. */
export function MultiSelectField({
  label,
  description,
  options,
  placeholder,
  isDisabled,
  className,
}: MultiSelectFieldProps) {
  const field = useFieldContext<string[]>()
  const error = fieldErrorMessage(field.state.meta)
  return (
    <Select
      name={field.name}
      id={field.name}
      selectionMode="multiple"
      value={field.state.value ?? []}
      onChange={(keys) => {
        field.handleChange((Array.isArray(keys) ? keys : []).map(String))
        clearServerError(field)
      }}
      onBlur={field.handleBlur}
      isInvalid={Boolean(error)}
      isDisabled={isDisabled}
      placeholder={placeholder}
      validationBehavior="aria"
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
