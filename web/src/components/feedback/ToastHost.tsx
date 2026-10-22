import { Toast } from '@heroui/react/toast'
import { I18nProvider } from 'react-aria-components'
import { useLocale } from '@/i18n'

/**
 * Lazily mounted by `ToastMount` (root route, outside the shells' `AriaProviders`): the region's React Aria
 * strings ("1 notification.", "Close") follow the app locale, not the browser's.
 */
export default function ToastHost() {
  const locale = useLocale()
  return (
    <I18nProvider locale={locale}>
      <Toast.Provider placement="top" maxVisibleToasts={3} />
    </I18nProvider>
  )
}
