import { ComboBox, FieldError, Input, ListBox } from '@heroui/react'
import type { ReactNode } from 'react'
import { FieldHint, FieldLabel } from '@/components/form/parts'

export interface ComboOption {
  value: string
  /** Shown in the list (defaults to `value`). */
  label?: ReactNode
  /** Secondary text in the list. */
  hint?: ReactNode
}

export interface ComboInputProps {
  label: ReactNode
  value: string
  onChange: (value: string) => void
  /** Suggestions; any other text is accepted too. */
  options: readonly (ComboOption | string)[]
  name?: string
  description?: ReactNode
  error?: string
  placeholder?: string
  isRequired?: boolean
  isDisabled?: boolean
  monospace?: boolean
  className?: string
}

const showAll = () => true

/**
 * Free-text input with suggestions (HeroUI ComboBox + `allowsCustomValue`). Used where the backend
 * accepts any string but common values exist: EPay `type`, Epusdt `currency_type`, knowledge
 * language/category, rule-set mirror prefix. The description sits behind the label's ⓘ.
 */
export function ComboInput({
  label,
  value,
  onChange,
  options,
  name,
  description,
  error,
  placeholder,
  isRequired,
  isDisabled,
  monospace,
  className,
}: ComboInputProps) {
  const items = options.map((o) => (typeof o === 'string' ? { value: o } : o))
  return (
    <ComboBox
      name={name}
      id={name}
      allowsCustomValue
      allowsEmptyCollection
      inputValue={value}
      onInputChange={onChange}
      defaultFilter={showAll}
      isInvalid={Boolean(error)}
      isRequired={isRequired}
      isDisabled={isDisabled}
      validationBehavior="aria"
      fullWidth
      className={className}
    >
      <FieldLabel label={label} help={description} />
      <ComboBox.InputGroup>
        <Input placeholder={placeholder} className={monospace ? 'font-mono' : undefined} autoComplete="off" />
        <ComboBox.Trigger />
      </ComboBox.InputGroup>
      {error ? null : <FieldHint>{description}</FieldHint>}
      <FieldError>{error}</FieldError>
      <ComboBox.Popover>
        <ListBox>
          {items.map((o) => (
            <ListBox.Item key={o.value} id={o.value} textValue={o.value}>
              <div className="flex min-w-0 flex-col">
                <span className={monospace && !o.label ? 'truncate font-mono' : 'truncate'}>
                  {o.label ?? o.value}
                </span>
                {o.hint ? (
                  <span className="truncate text-footnote text-label-secondary">{o.hint}</span>
                ) : null}
              </div>
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </ComboBox.Popover>
    </ComboBox>
  )
}
