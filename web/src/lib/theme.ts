/**
 * Applies the theme preference to `<html>`:
 *   system → no `data-theme` (CSS follows `prefers-color-scheme`)
 *   light/dark → `data-theme="light|dark"` (forced)
 * and keeps `<meta name="theme-color">` in sync for forced themes.
 */
import { useSyncExternalStore } from 'react'
import { type ThemePreference, useUiStore } from '@/stores/ui'

const THEME_COLOR = { light: '#F2F2F7', dark: '#000000' } as const
const DARK_QUERY = '(prefers-color-scheme: dark)'

export function applyTheme(theme: ThemePreference, root: HTMLElement = document.documentElement) {
  if (theme === 'system') root.removeAttribute('data-theme')
  else root.setAttribute('data-theme', theme)
  for (const meta of document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')) {
    const media = meta.dataset.media ?? meta.getAttribute('media') ?? ''
    if (!meta.dataset.media) meta.dataset.media = media
    if (theme === 'system') {
      meta.setAttribute('media', media)
      meta.content = media.includes('dark') ? THEME_COLOR.dark : THEME_COLOR.light
    } else {
      meta.removeAttribute('media')
      meta.content = THEME_COLOR[theme]
    }
  }
}

/** Call once at startup; returns an unsubscribe function. */
export function syncTheme(): () => void {
  applyTheme(useUiStore.getState().theme)
  return useUiStore.subscribe((s, prev) => {
    if (s.theme !== prev.theme) applyTheme(s.theme)
  })
}

function subscribeScheme(cb: () => void) {
  const mql = window.matchMedia?.(DARK_QUERY)
  mql?.addEventListener('change', cb)
  return () => mql?.removeEventListener('change', cb)
}
const systemIsDark = () => window.matchMedia?.(DARK_QUERY).matches ?? false

/** The effective scheme (`light` | `dark`) after resolving `system`. */
export function useResolvedTheme(): 'light' | 'dark' {
  const theme = useUiStore((s) => s.theme)
  const dark = useSyncExternalStore(subscribeScheme, systemIsDark, () => false)
  return theme === 'system' ? (dark ? 'dark' : 'light') : theme
}
