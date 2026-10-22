/** Small controlled HeroUI inputs for the rule builder (not bound to a form library: the draft is local state). */
import { FieldError, Label, ListBox, Select, Switch, ToggleButton, ToggleButtonGroup } from '@heroui/react'
import { type ReactNode, useId } from 'react'
import { cx } from '@/lib/cn'

export interface Option {
  value: string
  label: string
}

export function OptionSelect({
  label,
  hideLabel,
  value,
  options,
  onChange,
  isDisabled,
  error,
  className,
  variant,
}: {
  label: string
  hideLabel?: boolean
  value: string
  options: readonly Option[]
  onChange: (value: string) => void
  isDisabled?: boolean
  error?: string | undefined
  className?: string
  variant?: 'primary' | 'secondary'
}) {
  // Keep an unknown current value selectable (e.g. a custom mirror prefix) instead of showing nothing.
  const list = options.some((o) => o.value === value) ? options : [...options, { value, label: value }]
  return (
    <Select
      value={value}
      onChange={(key) => {
        if (key !== null) onChange(String(key))
      }}
      isDisabled={isDisabled}
      isInvalid={Boolean(error)}
      aria-label={hideLabel ? label : undefined}
      className={cx('min-w-0', className)}
      variant={variant}
    >
      {hideLabel ? null : <Label>{label}</Label>}
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      {error ? <FieldError>{error}</FieldError> : null}
      <Select.Popover>
        <ListBox>
          {list.map((option) => (
            <ListBox.Item key={option.value} id={option.value} textValue={option.label}>
              {option.label}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

export function Toggle({
  isSelected,
  onChange,
  isDisabled,
  children,
  className,
  'aria-label': ariaLabel,
}: {
  isSelected: boolean
  onChange: (value: boolean) => void
  isDisabled?: boolean
  children?: ReactNode
  className?: string
  'aria-label'?: string
}) {
  return (
    <Switch
      isSelected={isSelected}
      onChange={onChange}
      isDisabled={isDisabled}
      className={className}
      aria-label={ariaLabel}
    >
      <Switch.Content>
        <Switch.Control>
          <Switch.Thumb />
        </Switch.Control>
        {children}
      </Switch.Content>
    </Switch>
  )
}

/** Section header used inside the builder card. */
export function SectionTitle({
  id,
  title,
  hint,
  end,
}: {
  id: string
  title: string
  hint?: ReactNode
  end?: ReactNode
}) {
  return (
    <div className="flex flex-wrap items-end justify-between gap-2">
      <div className="min-w-0">
        <h3 id={id} className="text-headline text-label">
          {title}
        </h3>
        {hint ? <p className="mt-0.5 text-footnote text-label-secondary">{hint}</p> : null}
      </div>
      {end}
    </div>
  )
}

/** FE-S: multi-select chips (node types, regions); an empty selection means "all". */
export function ChipSelect({
  label,
  hint,
  value,
  options,
  onChange,
  isDisabled,
}: {
  label: string
  hint?: ReactNode
  value: readonly string[]
  options: ReadonlyArray<{ value: string; label: ReactNode; textValue?: string }>
  onChange: (value: string[]) => void
  isDisabled?: boolean
}) {
  const id = useId()
  return (
    <div className="flex min-w-0 flex-col gap-2">
      <div>
        <span id={id} className="text-subhead font-medium text-label">
          {label}
        </span>
        {hint ? <p className="text-footnote text-label-secondary">{hint}</p> : null}
      </div>
      <ToggleButtonGroup
        aria-labelledby={id}
        selectionMode="multiple"
        selectedKeys={value}
        onSelectionChange={(keys) => {
          const picked = new Set([...keys].map(String))
          // Keep the option order (stable payloads) and values without an option (stale saved entries).
          onChange([
            ...options.filter((o) => picked.has(o.value)).map((o) => o.value),
            ...value.filter((v) => picked.has(v) && !options.some((o) => o.value === v)),
          ])
        }}
        isDisabled={isDisabled}
        isDetached
        size="sm"
        className="flex-wrap justify-start"
      >
        {options.map((option) => (
          <ToggleButton
            key={option.value}
            id={option.value}
            aria-label={option.textValue}
            className="rounded-full"
          >
            {option.label}
          </ToggleButton>
        ))}
      </ToggleButtonGroup>
    </div>
  )
}
