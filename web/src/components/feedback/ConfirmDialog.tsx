import { AlertDialog, Button } from '@heroui/react'
import { type ReactNode, useState } from 'react'
import { useT } from '@/i18n'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'

export interface ConfirmDialogProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  title: ReactNode
  description?: ReactNode
  confirmLabel?: ReactNode
  cancelLabel?: ReactNode
  /** `danger` → red confirm button + warning icon. */
  tone?: 'primary' | 'danger'
  /**
   * Called on confirm. If it returns a promise the button shows a pending state and the dialog
   * closes only on success (errors are toasted and the dialog stays open).
   */
  onConfirm?: () => unknown
  onCancel?: () => void
}

/**
 * Controlled confirmation dialog (HeroUI AlertDialog).
 *
 *   const [open, setOpen] = useState(false)
 *   <ConfirmDialog isOpen={open} onOpenChange={setOpen} tone="danger"
 *     title={t('deleteTitle')} onConfirm={() => remove.mutateAsync(id)} />
 *
 * For one-off prompts prefer the imperative `confirm()` from `@/components/feedback/confirm`.
 */
export function ConfirmDialog({
  isOpen,
  onOpenChange,
  title,
  description,
  confirmLabel,
  cancelLabel,
  tone = 'primary',
  onConfirm,
  onCancel,
}: ConfirmDialogProps) {
  const t = useT('common')
  const [pending, setPending] = useState(false)

  const handleConfirm = async () => {
    try {
      setPending(true)
      await onConfirm?.()
      onOpenChange(false)
    } catch (error) {
      toast.danger(errorMessage(error))
    } finally {
      setPending(false)
    }
  }

  return (
    <AlertDialog.Backdrop
      isOpen={isOpen}
      onOpenChange={(open) => {
        if (!open && !pending) onCancel?.()
        if (!pending) onOpenChange(open)
      }}
      isDismissable={!pending}
      isKeyboardDismissDisabled={pending}
    >
      <AlertDialog.Container size="sm">
        <AlertDialog.Dialog>
          <AlertDialog.Header>
            <AlertDialog.Icon status={tone === 'danger' ? 'danger' : 'accent'} />
            <AlertDialog.Heading>{title}</AlertDialog.Heading>
          </AlertDialog.Header>
          {description ? (
            <AlertDialog.Body>
              <p className="text-subhead text-label-secondary">{description}</p>
            </AlertDialog.Body>
          ) : null}
          <AlertDialog.Footer>
            <Button variant="tertiary" isDisabled={pending} onPress={() => onOpenChange(false)}>
              {cancelLabel ?? t('actions.cancel')}
            </Button>
            <Button
              variant={tone === 'danger' ? 'danger' : 'primary'}
              isPending={pending}
              onPress={() => void handleConfirm()}
            >
              {confirmLabel ?? t('actions.confirm')}
            </Button>
          </AlertDialog.Footer>
        </AlertDialog.Dialog>
      </AlertDialog.Container>
    </AlertDialog.Backdrop>
  )
}

export default ConfirmDialog
