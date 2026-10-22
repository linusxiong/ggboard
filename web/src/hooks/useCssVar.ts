import { useMemo } from 'react'
import { useResolvedTheme } from '@/lib/theme'

/** Reads a CSS custom property from `<html>`; re-reads when the resolved theme changes. */
export function useCssVar(name: string, fallback = ''): string {
  const theme = useResolvedTheme()
  return useMemo(() => {
    if (typeof document === 'undefined') return fallback
    // `theme` is a dependency on purpose: the value differs per scheme.
    void theme
    return getComputedStyle(document.documentElement).getPropertyValue(name).trim() || fallback
  }, [name, fallback, theme])
}
