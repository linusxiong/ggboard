/**
 * Small presentational pieces shared by the form fields and the controlled admin inputs
 * (no TanStack Form dependency, so settings pages don't pull the form runtime).
 */
import { Description, Label, ListBox } from '@heroui/react'
import type { ReactNode } from 'react'
import { HelpTip } from '@/components/HelpTip'
import { cx } from '@/lib/cn'

const subjectOf = (label: ReactNode) => (typeof label === 'string' ? label : undefined)

/**
 * Field label with the field's description behind an ⓘ (explanations stay off the page's first level).
 * Pair it with `<FieldHint>` so the description is still the input's `aria-describedby`.
 */
export function FieldLabel({
  label,
  help,
  subject,
}: {
  label: ReactNode
  help?: ReactNode
  subject?: string
}) {
  if (!help) return <Label>{label}</Label>
  return (
    <span className="field-label-row">
      <Label>{label}</Label>
      <HelpTip subject={subject ?? subjectOf(label)}>{help}</HelpTip>
    </span>
  )
}

/** The description as screen-reader text (sighted users open it from the label's ⓘ). */
export function FieldHint({ children }: { children?: ReactNode }) {
  return children ? <Description className="sr-only">{children}</Description> : null
}

/**
 * Puts a switch's description behind an ⓘ. A switch is a `<label>`, so the button cannot go inside
 * it: in the `row` layout it sits before the switch at the trailing edge (as in iOS Settings), in the
 * `inline` layout after the label. Without help it returns the switch unchanged.
 */
export function SwitchHelp({
  children,
  help,
  label,
  layout = 'row',
  className,
}: {
  children: ReactNode
  help?: ReactNode
  label: ReactNode
  layout?: 'row' | 'inline'
  className?: string
}) {
  if (!help) return <>{children}</>
  return (
    <div className={cx('switch-help', `switch-help--${layout}`, className)}>
      {children}
      <HelpTip subject={subjectOf(label)}>{help}</HelpTip>
    </div>
  )
}

/** Unit / currency adornment inside a field (visual only; the label carries the unit for screen readers). */
export function FieldAffix({ children, start }: { children: ReactNode; start?: boolean }) {
  return (
    <span className={cx('field-affix', start && 'field-affix--start')} aria-hidden="true">
      {children}
    </span>
  )
}

/** List row for Select popovers: title + optional description + check mark. */
export function SelectItemContent({ label, description }: { label: ReactNode; description?: ReactNode }) {
  return description ? (
    <>
      <span className="flex min-w-0 flex-col py-1">
        <Label>{label}</Label>
        <Description>{description}</Description>
      </span>
      <ListBox.ItemIndicator />
    </>
  ) : (
    <>
      {label}
      <ListBox.ItemIndicator />
    </>
  )
}
