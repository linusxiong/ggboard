import { SearchField } from '@heroui/react'
import { Label } from '@heroui/react'
import { useEffect, useState } from 'react'

interface DebouncedSearchProps {
  label: string
  /** Committed value (from the URL). */
  value: string | undefined
  onCommit: (value: string | undefined) => void
  placeholder?: string
  delay?: number
  className?: string
  /** Hide the visible label (keeps it for screen readers). */
  hideLabel?: boolean
  monospace?: boolean
}

/**
 * Search box that commits to the URL after `delay` ms of inactivity (or on Enter / clear).
 * External changes (reset, back/forward) replace the draft.
 */
export function DebouncedSearch({
  label,
  value,
  onCommit,
  placeholder,
  delay = 400,
  className,
  hideLabel,
  monospace,
}: DebouncedSearchProps) {
  const [draft, setDraft] = useState(value ?? '')
  const [synced, setSynced] = useState(value)
  if (value !== synced) {
    setSynced(value)
    setDraft(value ?? '')
  }

  useEffect(() => {
    const next = draft.trim() || undefined
    if (next === value) return
    const timer = window.setTimeout(() => onCommit(next), delay)
    return () => window.clearTimeout(timer)
  }, [draft, value, delay, onCommit])

  return (
    <SearchField
      value={draft}
      onChange={setDraft}
      onSubmit={(v) => onCommit(v.trim() || undefined)}
      onClear={() => onCommit(undefined)}
      className={className}
      fullWidth
    >
      <Label className={hideLabel ? 'sr-only' : undefined}>{label}</Label>
      <SearchField.Group>
        <SearchField.SearchIcon />
        <SearchField.Input placeholder={placeholder} className={monospace ? 'font-mono' : undefined} />
        <SearchField.ClearButton />
      </SearchField.Group>
    </SearchField>
  )
}
