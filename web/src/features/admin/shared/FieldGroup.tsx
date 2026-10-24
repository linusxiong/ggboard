import { type ReactNode, useId } from 'react'
import { HelpTip } from '@/components/HelpTip'

/**
 * A titled group of form fields (`<fieldset>` + `<legend>`) for admin sheets. `help` puts an ⓘ in
 * the legend; the group's accessible name stays the title.
 */
export function FieldGroup({
  title,
  help,
  children,
}: {
  title: string
  help?: ReactNode
  children: ReactNode
}) {
  const titleId = useId()
  return (
    <fieldset className="flex min-w-0 flex-col gap-4" aria-labelledby={help ? titleId : undefined}>
      <legend className="mb-3 flex items-center gap-1 text-headline text-label">
        <span id={titleId}>{title}</span>
        {help ? <HelpTip subject={title}>{help}</HelpTip> : null}
      </legend>
      {children}
    </fieldset>
  )
}
