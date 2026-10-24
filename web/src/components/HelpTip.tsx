/**
 * Second-level explanations. Pages show only what the user reads or acts on; the why/how text
 * (field hints, page and section explanations, placeholder references) sits behind a small ⓘ
 * button that opens it in a popover, tap- and keyboard-friendly unlike a hover tooltip.
 *
 *   <HelpTip subject="分类">选择已有分类或输入新分类。</HelpTip>
 *   <TitleWithHelp help={t('hint')} subject={t('title')}><h2>{t('title')}</h2></TitleWithHelp>
 *
 * Form fields do this on their own: their `description` becomes the label's ⓘ (`FieldLabel`).
 */
import { Button, Popover } from '@heroui/react'
import { Info } from 'lucide-react'
import type { ReactNode } from 'react'
import { useT } from '@/i18n'
import { cx } from '@/lib/cn'

interface HelpTipProps {
  /** The explanation (plain text, or short rich content; line breaks in strings are kept). */
  children: ReactNode
  /** What it explains, for the button's accessible name ("关于「分类」的说明"). */
  subject?: string
  className?: string
}

export function HelpTip({ children, subject, className }: HelpTipProps) {
  const t = useT('common')
  return (
    <Popover>
      {/* slot={null}: inside a Select / NumberField the button must not pick up the field's own
          trigger or stepper props from React Aria's ButtonContext. */}
      <Button
        slot={null}
        isIconOnly
        size="sm"
        variant="ghost"
        aria-label={subject ? t('help.about', { name: subject }) : t('help.label')}
        className={cx('help-tip', className)}
      >
        <Info className="size-4" aria-hidden="true" />
      </Button>
      <Popover.Content placement="top" className="max-w-[min(20rem,calc(100vw-32px))]">
        <Popover.Dialog className="whitespace-pre-line p-3 text-footnote text-label">
          {children}
        </Popover.Dialog>
      </Popover.Content>
    </Popover>
  )
}

/**
 * A heading (or any title element) followed by its ⓘ. The button sits next to the heading, not inside
 * it, so the heading's accessible name stays the title. Renders the children alone without help.
 */
export function TitleWithHelp({
  children,
  help,
  subject,
  className,
}: {
  children: ReactNode
  help?: ReactNode
  /** What the help is about (usually the title text), for the button's accessible name. */
  subject?: string
  className?: string
}) {
  if (!help) return <>{children}</>
  return (
    <div className={cx('flex min-w-0 items-center gap-1', className)}>
      {children}
      <HelpTip subject={subject}>{help}</HelpTip>
    </div>
  )
}
