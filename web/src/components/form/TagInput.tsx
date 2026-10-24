/**
 * Token field for string lists (tags, email suffixes, URLs, chat ids): chips + an inline input.
 * Enter, comma, semicolon or newline adds (paste splits a whole list); Backspace on an empty input
 * selects the last chip, a second Backspace removes it; every chip has its own remove button.
 * Duplicates are ignored. Optional `suggestions` render as "+ item" chips under the field.
 *
 *   <TagInput label="允许的邮箱后缀" value={list} onChange={setList}
 *     normalize={(s) => s.replace(/^@/, '').toLowerCase()} suggestions={['gmail.com', 'outlook.com']} />
 *
 * Form-bound version: `<f.TagsField …/>` (`@/components/form`).
 */
import { type ClipboardEvent, type KeyboardEvent, type ReactNode, useId, useRef, useState } from 'react'
import { HelpTip } from '@/components/HelpTip'
import { cx } from '@/lib/cn'
import { splitTagText } from './tags'

export interface TagInputProps {
  label: ReactNode
  value: readonly string[]
  onChange: (value: string[]) => void
  name?: string
  id?: string
  description?: ReactNode
  /** External error (validation / server). */
  error?: string
  placeholder?: string
  isDisabled?: boolean
  isRequired?: boolean
  /** Returns an error message for an item that must not be added. */
  validate?: (item: string) => string | undefined
  /** Cleans an item before validation (trim is always applied). */
  normalize?: (item: string) => string
  /** Quick-add chips shown under the field (hidden once added). */
  suggestions?: readonly string[]
  /** Monospace chips (URLs, IPs, codes). */
  monospace?: boolean
  /** Accessible label for a chip's remove button. Default: `Remove {item}`. */
  removeLabel?: (item: string) => string
  /** Prefix of the suggestion chips' accessible label. Default: `Add`. */
  addLabel?: string
  className?: string
  onBlur?: () => void
}

export function TagInput({
  label,
  value,
  onChange,
  name,
  id,
  description,
  error,
  placeholder,
  isDisabled,
  isRequired,
  validate,
  normalize,
  suggestions,
  monospace,
  removeLabel = (item) => `Remove ${item}`,
  addLabel = 'Add',
  className,
  onBlur,
}: TagInputProps) {
  const auto = useId()
  const inputId = id ?? `${name ?? 'tags'}-${auto}`
  const labelId = `${inputId}-label`
  const descId = `${inputId}-desc`
  const errId = `${inputId}-err`
  const inputRef = useRef<HTMLInputElement>(null)
  const [draft, setDraft] = useState('')
  const [draftError, setDraftError] = useState<string>()
  const [armed, setArmed] = useState(false)

  const shownError = draftError ?? error
  const invalid = Boolean(shownError)

  /** Adds every item in `text`; keeps the first invalid one in the input. Returns true if all were added. */
  const add = (text: string) => {
    const items = splitTagText(text)
    if (items.length === 0) {
      setDraft('')
      return true
    }
    const next = [...value]
    for (const raw of items) {
      const item = normalize ? normalize(raw.trim()) : raw.trim()
      if (!item) continue
      const problem = validate?.(item)
      if (problem) {
        if (next.length !== value.length) onChange(next)
        setDraft(raw)
        setDraftError(problem)
        return false
      }
      if (!next.includes(item)) next.push(item)
    }
    setDraft('')
    setDraftError(undefined)
    if (next.length !== value.length) onChange(next)
    return true
  }

  const remove = (item: string) => {
    onChange(value.filter((v) => v !== item))
    setArmed(false)
    inputRef.current?.focus()
  }

  const onKeyDown = (e: KeyboardEvent<HTMLInputElement>) => {
    if (e.key === 'Enter' || e.key === ',' || e.key === '，' || e.key === ';' || e.key === '；') {
      if (e.nativeEvent.isComposing) return
      if (draft.trim() === '' && e.key === 'Enter') return // let Enter submit the form
      e.preventDefault()
      add(draft)
    } else if (e.key === 'Backspace' && draft === '' && value.length > 0) {
      e.preventDefault()
      if (armed) remove(value[value.length - 1]!)
      else setArmed(true)
    } else if (e.key === 'Escape' && armed) {
      setArmed(false)
    } else if (armed) {
      setArmed(false)
    }
  }

  const onPaste = (e: ClipboardEvent<HTMLInputElement>) => {
    const text = e.clipboardData.getData('text')
    if (!/[,;\n\r，；、]/.test(text)) return
    e.preventDefault()
    add(`${draft}${text}`)
  }

  const describedBy =
    cx(description && !invalid ? descId : undefined, invalid ? errId : undefined) || undefined
  const pending = (suggestions ?? []).filter((s) => !value.includes(s))

  return (
    <div
      className={cx('gg-field', className)}
      data-invalid={invalid || undefined}
      data-disabled={isDisabled || undefined}
    >
      {description ? (
        <span className="field-label-row">
          <label id={labelId} htmlFor={inputId} className="label">
            {label}
          </label>
          <HelpTip subject={typeof label === 'string' ? label : undefined}>{description}</HelpTip>
        </span>
      ) : (
        <label id={labelId} htmlFor={inputId} className="label">
          {label}
        </label>
      )}
      <div
        role="group"
        aria-labelledby={labelId}
        className="gg-field__control gg-tags"
        data-invalid={invalid || undefined}
        onClick={(e) => {
          if (e.target === e.currentTarget) inputRef.current?.focus()
        }}
      >
        {value.length > 0 ? (
          <ul className="gg-tags__list" aria-labelledby={labelId}>
            {value.map((item, i) => (
              <li
                key={item}
                className={cx(
                  'gg-tag',
                  monospace && 'font-mono',
                  armed && i === value.length - 1 && 'gg-tag--armed',
                )}
              >
                <span className="min-w-0 truncate">{item}</span>
                {isDisabled ? null : (
                  <button
                    type="button"
                    className="gg-tag__remove"
                    aria-label={removeLabel(item)}
                    onClick={() => remove(item)}
                  >
                    <svg viewBox="0 0 12 12" aria-hidden="true">
                      <path
                        d="M3.5 3.5l5 5M8.5 3.5l-5 5"
                        stroke="currentColor"
                        strokeWidth="1.5"
                        strokeLinecap="round"
                      />
                    </svg>
                  </button>
                )}
              </li>
            ))}
          </ul>
        ) : null}
        <input
          ref={inputRef}
          id={inputId}
          name={name}
          type="text"
          value={draft}
          disabled={isDisabled}
          required={isRequired && value.length === 0}
          aria-invalid={invalid || undefined}
          aria-describedby={describedBy}
          autoComplete="off"
          autoCapitalize="none"
          spellCheck={false}
          enterKeyHint="done"
          placeholder={value.length === 0 ? placeholder : undefined}
          className={cx('gg-tags__input', monospace && 'font-mono')}
          onChange={(e) => {
            setDraft(e.target.value)
            setDraftError(undefined)
            setArmed(false)
          }}
          onKeyDown={onKeyDown}
          onPaste={onPaste}
          onBlur={() => {
            setArmed(false)
            add(draft)
            onBlur?.()
          }}
        />
      </div>
      {pending.length > 0 && !isDisabled ? (
        <div className="flex flex-wrap gap-1.5 pt-0.5">
          {pending.map((s) => (
            <button
              key={s}
              type="button"
              className="gg-suggestion"
              aria-label={`${addLabel} ${s}`}
              onClick={() => onChange([...value, s])}
            >
              <span aria-hidden="true">+</span>
              {s}
            </button>
          ))}
        </div>
      ) : null}
      {description && !invalid ? (
        <p id={descId} className="sr-only">
          {description}
        </p>
      ) : null}
      {invalid ? (
        <p id={errId} className="field-error" data-visible="true" role="alert">
          {shownError}
        </p>
      ) : null}
    </div>
  )
}
