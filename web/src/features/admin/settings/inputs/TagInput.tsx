import { Description, FieldError, Input, Label, Tag, TagGroup, TextField } from '@heroui/react'
import type { Key } from '@heroui/react'
import { type ReactNode, useId, useState } from 'react'
import { useT } from '@/i18n'
import { splitTags } from './tags'

export interface TagInputProps {
  label: ReactNode
  value: readonly string[]
  onChange: (value: string[]) => void
  name?: string
  description?: ReactNode
  error?: string
  placeholder?: string
  isDisabled?: boolean
  /** Returns an error message for an item that must not be added. */
  validate?: (item: string) => string | undefined
  /** Monospace tags (URLs, IPs, codes). */
  monospace?: boolean
  className?: string
}

/**
 * Editor for `strings` settings and tag lists: removable tags (keyboard: Backspace/Delete on a
 * focused tag) plus an input that adds on Enter or comma (and splits pasted lists).
 * Needs the `admin-settings` i18n namespace.
 */
export function TagInput({
  label,
  value,
  onChange,
  name,
  description,
  error,
  placeholder,
  isDisabled,
  validate,
  monospace,
  className,
}: TagInputProps) {
  const t = useT('admin-settings')
  const id = useId()
  const [draft, setDraft] = useState('')
  const [draftError, setDraftError] = useState<string>()

  const add = (text: string) => {
    const items = splitTags(text)
    if (items.length === 0) return
    const next = [...value]
    for (const item of items) {
      const problem = validate?.(item)
      if (problem) {
        setDraftError(problem)
        setDraft(item)
        return
      }
      if (!next.includes(item)) next.push(item)
    }
    setDraftError(undefined)
    setDraft('')
    onChange(next)
  }

  const remove = (keys: Set<Key>) => onChange(value.filter((v) => !keys.has(v)))
  const shownError = draftError ?? error

  return (
    <div className={className ?? 'flex flex-col gap-2'}>
      <TagGroup aria-labelledby={`${id}-label`} onRemove={isDisabled ? undefined : remove}>
        <Label id={`${id}-label`}>{label}</Label>
        <TagGroup.List
          items={value.map((v) => ({ id: v }))}
          renderEmptyState={() => (
            <span className="text-footnote text-label-secondary">{t('inputs.empty')}</span>
          )}
        >
          {(item) => (
            <Tag id={item.id} textValue={item.id} className={monospace ? 'font-mono' : undefined}>
              {item.id}
            </Tag>
          )}
        </TagGroup.List>
      </TagGroup>
      <TextField
        name={name}
        aria-label={t('inputs.addTo', { label: typeof label === 'string' ? label : '' })}
        value={draft}
        onChange={(v) => {
          if (/[,;\n，；]/.test(v)) add(v)
          else {
            setDraft(v)
            setDraftError(undefined)
          }
        }}
        onKeyDown={(e) => {
          if (e.key === 'Enter') {
            e.preventDefault()
            add(draft)
          } else if (e.key === 'Backspace' && draft === '' && value.length > 0) {
            onChange(value.slice(0, -1))
          }
        }}
        onBlur={() => add(draft)}
        isInvalid={Boolean(shownError)}
        isDisabled={isDisabled}
        validationBehavior="aria"
        fullWidth
      >
        <Input
          placeholder={placeholder ?? t('inputs.addPlaceholder')}
          className={monospace ? 'w-full font-mono' : 'w-full'}
          autoComplete="off"
        />
        {description && !shownError ? <Description>{description}</Description> : null}
        <FieldError>{shownError}</FieldError>
      </TextField>
    </div>
  )
}
