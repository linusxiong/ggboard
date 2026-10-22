import { useRouter } from '@tanstack/react-router'
import type { ReactNode } from 'react'
import { I18nProvider, RouterProvider } from 'react-aria-components'
import { useLocale } from '@/i18n'

/**
 * React Aria context for HeroUI v3 (it has no provider of its own):
 * - RouterProvider: `href` on HeroUI Link / Button / Dropdown.Item navigates client-side,
 * - I18nProvider: number/date fields and calendars follow the app locale.
 * Mounted by the authenticated shells (public pages use native elements only).
 */
export function AriaProviders({ children }: { children: ReactNode }) {
  const router = useRouter()
  const locale = useLocale()
  return (
    <RouterProvider
      navigate={(to) => void router.navigate({ to: to as '/' })}
      useHref={(to) => router.buildLocation({ to: to as '/' }).href}
    >
      <I18nProvider locale={locale}>{children}</I18nProvider>
    </RouterProvider>
  )
}
