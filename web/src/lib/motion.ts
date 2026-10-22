/** Idle scheduling + reduced-motion helpers (AGENTS.md §6: start animations after idle). */
import { useEffect, useState, useSyncExternalStore } from 'react'

type IdleHandle = number
const ric: (cb: () => void, opts?: { timeout: number }) => IdleHandle =
  typeof window !== 'undefined' && 'requestIdleCallback' in window
    ? (cb, opts) => window.requestIdleCallback(cb, opts)
    : (cb) => window.setTimeout(cb, 200)
const cic: (handle: IdleHandle) => void =
  typeof window !== 'undefined' && 'cancelIdleCallback' in window
    ? (h) => window.cancelIdleCallback(h)
    : (h) => window.clearTimeout(h)

/** Runs `cb` when the main thread is idle (after load). Returns a cancel function. */
export function onIdle(cb: () => void, timeout = 2000): () => void {
  let handle: IdleHandle | undefined
  const schedule = () => {
    handle = ric(cb, { timeout })
  }
  if (document.readyState === 'complete') schedule()
  else window.addEventListener('load', schedule, { once: true })
  return () => {
    window.removeEventListener('load', schedule)
    if (handle !== undefined) cic(handle)
  }
}

/** `true` once the page has loaded and the browser reported idle time. */
export function useIdle(timeout = 2000): boolean {
  const [idle, setIdle] = useState(false)
  useEffect(() => onIdle(() => setIdle(true), timeout), [timeout])
  return idle
}

const REDUCED = '(prefers-reduced-motion: reduce)'
function subscribeReduced(cb: () => void) {
  const mql = window.matchMedia?.(REDUCED)
  mql?.addEventListener('change', cb)
  return () => mql?.removeEventListener('change', cb)
}
export const prefersReducedMotion = () => window.matchMedia?.(REDUCED).matches ?? false

/** Live `prefers-reduced-motion: reduce`. */
export function useReducedMotion(): boolean {
  return useSyncExternalStore(subscribeReduced, prefersReducedMotion, () => false)
}

/** `true` when decorative animation may start: idle reached and motion allowed. */
export function useAnimationsReady(timeout = 2000): boolean {
  const idle = useIdle(timeout)
  const reduced = useReducedMotion()
  return idle && !reduced
}
