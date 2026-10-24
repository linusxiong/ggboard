/**
 * Small controlled HeroUI inputs for the rule builder (not bound to a form library: the draft is local state).
 * Explanations (`description`, `help`, `hint`) sit behind an ⓘ next to the label or title (`HelpTip`).
 */
import { FieldError, ListBox, Select, Switch, ToggleButton, ToggleButtonGroup } from '@heroui/react'
import { type ReactNode, useId } from 'react'
import { FieldHint, FieldLabel, SwitchHelp } from '@/components/form/parts'
import { HelpTip, TitleWithHelp } from '@/components/HelpTip'
import { cx } from '@/lib/cn'

export interface Option {
  value: string
  label: string
}

export function OptionSelect({
  label,
  hideLabel,
  description,
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
  /** Explanation behind the label's ⓘ (still the trigger's accessible description). */
  description?: ReactNode
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
      {hideLabel ? null : <FieldLabel label={label} help={description} />}
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      {error ? <FieldError>{error}</FieldError> : <FieldHint>{description}</FieldHint>}
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
  help,
  className,
  'aria-label': ariaLabel,
}: {
  isSelected: boolean
  onChange: (value: boolean) => void
  isDisabled?: boolean
  children?: ReactNode
  /** Explanation behind an ⓘ after the label (outside the switch's `<label>`). */
  help?: ReactNode
  className?: string
  'aria-label'?: string
}) {
  const control = (
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
  return (
    <SwitchHelp
      help={help}
      label={typeof children === 'string' ? children : (ariaLabel ?? '')}
      layout="inline"
    >
      {control}
    </SwitchHelp>
  )
}

/** Section header used inside the builder card; `hint` (what the section does) goes behind the title's ⓘ. */
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
      <TitleWithHelp help={hint} subject={title} className="min-w-0">
        <h3 id={id} className="text-headline text-label">
          {title}
        </h3>
      </TitleWithHelp>
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
      <ChipLabel id={id} label={label} hint={hint} />
      <ToggleButtonGroup
        aria-labelledby={id}
        aria-describedby={hint ? `${id}-hint` : undefined}
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

/**
 * Label of a chip / tag group (not a form `<label>`) with its explanation behind an ⓘ; the explanation stays
 * in the DOM as `${id}-hint` (sr-only) for the group's `aria-describedby`.
 */
export function ChipLabel({ id, label, hint }: { id: string; label: string; hint?: ReactNode }) {
  return (
    <div className="flex min-w-0 items-center gap-0.5">
      <span id={id} className="text-subhead font-medium text-label">
        {label}
      </span>
      {hint ? (
        <>
          <HelpTip subject={label}>{hint}</HelpTip>
          <span id={`${id}-hint`} className="sr-only">
            {hint}
          </span>
        </>
      ) : null}
    </div>
  )
}
