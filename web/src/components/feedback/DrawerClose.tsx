import { Drawer } from '@heroui/react'
import { useT } from '@/i18n'

/** `Drawer.CloseTrigger` with a localized label (HeroUI hard-codes `aria-label="Close"`). */
export function DrawerClose() {
  const t = useT('common')
  return <Drawer.CloseTrigger aria-label={t('actions.close')} />
}
