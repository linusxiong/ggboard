import { Drawer } from '@heroui/react'
import { type FormEvent, type ReactNode, useCallback } from 'react'
import { TitleWithHelp } from '@/components/HelpTip'
import { cx } from '@/lib/cn'
import { DrawerClose } from '@/components/feedback/DrawerClose'
import { useIsDesktop } from '@/hooks/useMediaQuery'

interface FormDrawerProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  /** Small text under the heading: short facts only (counts, ids). Explanations go in `help`. */
  subtitle?: ReactNode
  /** What the sheet is for, behind an ⓘ next to the heading. */
  help?: ReactNode
  children: ReactNode
  footer?: ReactNode
  /** Panel width on ≥ sm screens. */
  size?: 'md' | 'lg' | 'xl'
  /** Keep the drawer open on outside clicks (forms with unsaved input). */
  isDismissable?: boolean
  /** Wrap body + footer in a `<form>` (so a submit button in the footer submits it). */
  onSubmit?: () => void
}

const WIDTH = { md: 'sm:w-[28rem]', lg: 'sm:w-[36rem]', xl: 'sm:w-[48rem]' }

/**
 * Sheet for create/edit forms and detail panels: a floating inspector on the right on desktop, a
 * bottom sheet with a grabber on phones. Title 20/600, footer pinned with a hairline.
 */
export function FormDrawer({
  isOpen,
  onOpenChange,
  title,
  subtitle,
  help,
  children,
  footer,
  size = 'lg',
  isDismissable = true,
  onSubmit,
}: FormDrawerProps) {
  const desktop = useIsDesktop()
  // Forms on desktop: put the caret in the first field once the sheet has opened (phones skip it —
  // the keyboard would cover the sheet).
  const isForm = Boolean(onSubmit)
  const autoFocusFirst = useCallback(
    (node: HTMLDivElement | null) => {
      if (!node || !isForm || !desktop) return
      const timer = window.setTimeout(() => {
        if (node.contains(document.activeElement) && document.activeElement !== node) return
        node
          .querySelector<HTMLElement>(
            'input:not([type="hidden"]):not([disabled]):not([readonly]), textarea:not([disabled])',
          )
          ?.focus({ preventScroll: true })
      }, 80)
      return () => window.clearTimeout(timer)
    },
    [isForm, desktop],
  )
  const content = (
    <>
      <Drawer.Body>
        <div ref={autoFocusFirst}>{children}</div>
      </Drawer.Body>
      {footer ? <Drawer.Footer>{footer}</Drawer.Footer> : null}
    </>
  )
  return (
    <Drawer.Backdrop isOpen={isOpen} onOpenChange={onOpenChange} isDismissable={isDismissable}>
      <Drawer.Content placement={desktop ? 'right' : 'bottom'}>
        <Drawer.Dialog className={cx('w-full max-w-full', WIDTH[size])}>
          {desktop ? null : <Drawer.Handle />}
          <DrawerClose />
          <Drawer.Header>
            <TitleWithHelp help={help} subject={typeof title === 'string' ? title : undefined}>
              <Drawer.Heading>{title}</Drawer.Heading>
            </TitleWithHelp>
            {subtitle ? <p className="text-footnote text-label-secondary">{subtitle}</p> : null}
          </Drawer.Header>
          {onSubmit ? (
            <form
              noValidate
              className="contents"
              onSubmit={(event: FormEvent) => {
                event.preventDefault()
                onSubmit()
              }}
            >
              {content}
            </form>
          ) : (
            content
          )}
        </Drawer.Dialog>
      </Drawer.Content>
    </Drawer.Backdrop>
  )
}
