import { Modal } from '@heroui/react'
import { useT } from '@/i18n'

/** `Modal.CloseTrigger` with a localized label (HeroUI hard-codes `aria-label="Close"`). */
export function ModalClose() {
  const t = useT('common')
  return <Modal.CloseTrigger aria-label={t('actions.close')} />
}
