import { Button } from '@heroui/react'
import type { ReactNode } from 'react'
import { cx } from '@/lib/cn'

interface SaveBarProps {
  dirty: boolean
  saving?: boolean
  /** Status text while there are unsaved changes / when everything is saved. */
  dirtyLabel: ReactNode
  cleanLabel?: ReactNode
  saveLabel: ReactNode
  discardLabel: ReactNode
  onDiscard: () => void
  /** Without it the save button is `type="submit"` (put the bar inside the `<form>`). */
  onSave?: () => void
  /** Extra hint on wide screens (e.g. "⌘S to save"). */
  hint?: ReactNode
  className?: string
}

/**
 * Sticky save bar for long forms with dirty tracking: a floating glass capsule pinned to the bottom
 * of the viewport (above the phone tab bar), status on the left, Discard + Save on the right.
 */
export function SaveBar({
  dirty,
  saving = false,
  dirtyLabel,
  cleanLabel,
  saveLabel,
  discardLabel,
  onDiscard,
  onSave,
  hint,
  className,
}: SaveBarProps) {
  return (
    <div
      className={cx(
        'sticky bottom-[calc(env(safe-area-inset-bottom)+4.25rem)] z-20 md:bottom-4',
        'glass flex items-center gap-2 rounded-[16px] py-2.5 ps-4 pe-2.5 shadow-float',
        // Phones: only surface the bar when there is something to save (it would cover content).
        !dirty && 'max-md:hidden',
        className,
      )}
      data-dirty={dirty || undefined}
    >
      <p
        className="me-auto flex min-w-0 items-center gap-2 text-footnote text-label-secondary"
        aria-live="polite"
      >
        <span
          className={cx('size-2 shrink-0 rounded-full', dirty ? 'bg-system-orange' : 'bg-system-green')}
          aria-hidden="true"
        />
        <span className="truncate">{dirty ? dirtyLabel : cleanLabel}</span>
        {dirty && hint ? <span className="hidden lg:inline">· {hint}</span> : null}
      </p>
      <Button variant="tertiary" isDisabled={!dirty || saving} onPress={onDiscard}>
        {discardLabel}
      </Button>
      <Button type={onSave ? 'button' : 'submit'} onPress={onSave} isDisabled={!dirty} isPending={saving}>
        {saveLabel}
      </Button>
    </div>
  )
}
