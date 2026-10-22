import { useSyncExternalStore } from 'react'

/** Shared minute clock: components that compare dates with "now" re-render once a minute (pure render). */
let now = Date.now()
const listeners = new Set<() => void>()
let timer: number | undefined

function subscribe(cb: () => void) {
  listeners.add(cb)
  if (timer === undefined) {
    timer = window.setInterval(() => {
      now = Date.now()
      listeners.forEach((l) => l())
    }, 60_000)
  }
  return () => {
    listeners.delete(cb)
    if (listeners.size === 0 && timer !== undefined) {
      window.clearInterval(timer)
      timer = undefined
    }
  }
}

export function useNow(): number {
  return useSyncExternalStore(
    subscribe,
    () => now,
    () => now,
  )
}
