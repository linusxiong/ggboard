/**
 * Small presentational pieces shared by the form fields and the controlled admin inputs
 * (no TanStack Form dependency, so settings pages don't pull the form runtime).
 */
import { Description, Label, ListBox } from '@heroui/react'
import type { ReactNode } from 'react'
import { cx } from '@/lib/cn'

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
