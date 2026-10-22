import { Drawer } from '@heroui/react'
import type { FormEvent, ReactNode } from 'react'
import { cx } from '@/lib/cn'
import { DrawerClose } from '@/components/feedback/DrawerClose'

interface FormDrawerProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  /** Small text under the heading. */
  subtitle?: ReactNode
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

/** Right-hand sheet used for create/edit forms and detail panels. */
export function FormDrawer({
  isOpen,
  onOpenChange,
  title,
  subtitle,
  children,
  footer,
  size = 'lg',
  isDismissable = true,
  onSubmit,
}: FormDrawerProps) {
  const content = (
    <>
      <Drawer.Body>{children}</Drawer.Body>
      {footer ? <Drawer.Footer>{footer}</Drawer.Footer> : null}
    </>
  )
  return (
    <Drawer.Backdrop isOpen={isOpen} onOpenChange={onOpenChange} isDismissable={isDismissable}>
      <Drawer.Content placement="right">
        <Drawer.Dialog className={cx('w-full max-w-full', WIDTH[size])}>
          <DrawerClose />
          <Drawer.Header>
            <Drawer.Heading>{title}</Drawer.Heading>
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
