import { useCallback, useSyncExternalStore } from 'react'

/**
 * Live `matchMedia` result (`fallback` on the server / without matchMedia).
 *
 *   const desktop = useMediaQuery('(min-width: 768px)')
 */
export function useMediaQuery(query: string, fallback = true): boolean {
  const subscribe = useCallback(
    (cb: () => void) => {
      const mql = window.matchMedia?.(query)
      mql?.addEventListener('change', cb)
      return () => mql?.removeEventListener('change', cb)
    },
    [query],
  )
  return useSyncExternalStore(
    subscribe,
    () => window.matchMedia?.(query).matches ?? fallback,
    () => fallback,
  )
}

/** `md` and up (the sidebar layout); phones get sheets and tab bars. */
export const useIsDesktop = () => useMediaQuery('(min-width: 768px)')
