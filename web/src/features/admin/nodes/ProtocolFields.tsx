/**
 * Inputs bound to a `protocol_settings` path through `ProtocolContext` (value, setter and the
 * server/client error for that path). HeroUI components with visible labels.
 */
import {
  Description,
  FieldError,
  Input,
  Label,
  ListBox,
  NumberField,
  Select,
  Switch,
  TextArea,
  TextField,
} from '@heroui/react'
import { type ReactNode, useState } from 'react'
import { cx } from '@/lib/cn'
import { useSetting } from './protocol-context'

interface BaseProps {
  path: string
  label: ReactNode
  description?: ReactNode
  className?: string
}

export function PText({
  path,
  label,
  description,
  className,
  placeholder,
  mono,
  isRequired,
  end,
}: BaseProps & { placeholder?: string; mono?: boolean; isRequired?: boolean; end?: ReactNode }) {
  const { value, set, error, id } = useSetting(path)
  return (
    <TextField
      id={id}
      name={id}
      value={typeof value === 'string' ? value : typeof value === 'number' ? String(value) : ''}
      onChange={set}
      isInvalid={Boolean(error)}
      isRequired={isRequired}
      validationBehavior="aria"
      fullWidth
      className={className}
    >
      <Label>{label}</Label>
      <div className="flex items-center gap-2">
        <Input
          placeholder={placeholder}
          className={cx('w-full min-w-0 flex-1', mono && 'font-mono text-footnote')}
          spellCheck={false}
          autoComplete="off"
        />
        {end}
      </div>
      {description && !error ? <Description>{description}</Description> : null}
      <FieldError>{error}</FieldError>
    </TextField>
  )
}

export function PNumber({
  path,
  label,
  description,
  className,
  minValue = 0,
  maxValue,
  step = 1,
}: BaseProps & { minValue?: number; maxValue?: number; step?: number }) {
  const { value, set, error, id } = useSetting(path)
  return (
    <NumberField
      id={id}
      name={id}
      value={typeof value === 'number' ? value : Number.NaN}
      onChange={(v) => set(Number.isNaN(v) ? undefined : v)}
      minValue={minValue}
      maxValue={maxValue}
      step={step}
      isInvalid={Boolean(error)}
      validationBehavior="aria"
      className={className}
    >
      <Label>{label}</Label>
      <NumberField.Group>
        <NumberField.Input className="w-full" />
      </NumberField.Group>
      {description && !error ? <Description>{description}</Description> : null}
      <FieldError>{error}</FieldError>
    </NumberField>
  )
}

export function PSwitch({ path, label, description, className }: BaseProps) {
  const { value, set, error, id } = useSetting(path)
  return (
    <Switch
      id={id}
      name={id}
      isSelected={value === true}
      onChange={set}
      isInvalid={Boolean(error)}
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

export interface POption {
  value: string | number
  label: string
}

/** Collection keys must be non-empty: `''` options (e.g. "no flow") use a sentinel key. */
const keyOf = (v: string | number) => (v === '' ? '__empty__' : String(v))

export function PSelect({
  path,
  label,
  description,
  className,
  options,
  fallback = '',
}: BaseProps & {
  options: readonly POption[]
  /** Option shown while the key is absent. */ fallback?: string | number
}) {
  const { value, set, error, id } = useSetting(path)
  const selected = options.find((o) => o.value === (value ?? fallback))
  return (
    <Select
      id={id}
      name={id}
      value={selected ? keyOf(selected.value) : null}
      onChange={(key) => {
        const option = options.find((o) => keyOf(o.value) === String(key))
        if (option) set(option.value)
      }}
      isInvalid={Boolean(error)}
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
          {options.map((o) => (
            <ListBox.Item key={keyOf(o.value)} id={keyOf(o.value)} textValue={o.label}>
              {o.label}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

const toList = (v: unknown): string[] => (Array.isArray(v) ? v.map(String) : [])

/**
 * `string[]` edited as text: one entry per line (`separator="line"`) or comma separated.
 * Keeps the raw text locally so typing a separator does not jump the caret.
 */
export function PList({
  path,
  label,
  description,
  className,
  separator = 'comma',
  rows = 4,
  placeholder,
  end,
}: BaseProps & { separator?: 'comma' | 'line'; rows?: number; placeholder?: string; end?: ReactNode }) {
  const { value, set, error, id } = useSetting(path)
  const list = toList(value)
  const join = (items: string[]) => items.join(separator === 'line' ? '\n' : ', ')
  const parse = (text: string) =>
    text
      .split(separator === 'line' ? '\n' : /[,，\n]/)
      .map((s) => s.trim())
      .filter(Boolean)
  const [text, setText] = useState(() => join(list))
  const serialized = JSON.stringify(list)
  const [seen, setSeen] = useState(serialized)
  if (seen !== serialized) {
    // external change (e.g. "use default" button): resync the text during render
    setSeen(serialized)
    if (JSON.stringify(parse(text)) !== serialized) setText(join(list))
  }
  const onChange = (next: string) => {
    setText(next)
    set(parse(next))
  }
  return (
    <TextField
      id={id}
      name={id}
      value={text}
      onChange={onChange}
      isInvalid={Boolean(error)}
      validationBehavior="aria"
      fullWidth
      className={className}
    >
      <div className="flex items-center justify-between gap-2">
        <Label>{label}</Label>
        {end}
      </div>
      {separator === 'line' ? (
        <TextArea
          rows={rows}
          placeholder={placeholder}
          className="w-full font-mono text-footnote"
          spellCheck={false}
        />
      ) : (
        <Input placeholder={placeholder} className="w-full" spellCheck={false} autoComplete="off" />
      )}
      {description && !error ? <Description>{description}</Description> : null}
      <FieldError>{error}</FieldError>
    </TextField>
  )
}

/** JSON object editor for one path (e.g. xhttp `extra`); invalid JSON keeps the last valid value. */
export function PJson({
  path,
  label,
  description,
  className,
  rows = 6,
  invalidLabel,
}: BaseProps & { rows?: number; invalidLabel: string }) {
  const { value, set, error, id } = useSetting(path)
  const [text, setText] = useState(() => (value === undefined ? '' : JSON.stringify(value, null, 2)))
  const [parseError, setParseError] = useState<string>()
  const message = parseError ?? error
  return (
    <TextField
      id={id}
      name={id}
      value={text}
      onChange={(next) => {
        setText(next)
        if (!next.trim()) {
          setParseError(undefined)
          set(undefined)
          return
        }
        try {
          const parsed: unknown = JSON.parse(next)
          if (parsed === null || typeof parsed !== 'object' || Array.isArray(parsed))
            throw new Error('not an object')
          setParseError(undefined)
          set(parsed)
        } catch {
          setParseError(invalidLabel)
        }
      }}
      isInvalid={Boolean(message)}
      validationBehavior="aria"
      fullWidth
      className={className}
    >
      <Label>{label}</Label>
      <TextArea rows={rows} className="w-full font-mono text-footnote" spellCheck={false} />
      {description && !message ? <Description>{description}</Description> : null}
      <FieldError>{message}</FieldError>
    </TextField>
  )
}
