/**
 * Tiny localStorage persistence for the always-loaded stores (auth, ui). Same on-disk shape as
 * zustand's `persist` (`{"state": …, "version": n}`) but ~1 KB lighter in the first-load
 * bundle. Feature stores in lazy chunks may use `zustand/middleware` `persist` directly.
 */
import type { StoreApi } from 'zustand'

function read(key: string): unknown {
  try {
    const raw = globalThis.localStorage?.getItem(key)
    if (!raw) return undefined
    return (JSON.parse(raw) as { state?: unknown }).state
  } catch {
    return undefined
  }
}

function write(key: string, state: unknown, version: number) {
  try {
    globalThis.localStorage?.setItem(key, JSON.stringify({ state, version }))
  } catch {
    /* private mode / quota: ignore */
  }
}

export interface PersistOptions<T> {
  key: string
  version?: number
  /** Subset of state to save. */
  pick: (state: T) => Partial<T>
  /** Validate/merge persisted data into the current state. */
  merge: (persisted: unknown, current: T) => Partial<T>
}

/**
 * Hydrates `store` synchronously from localStorage, saves on every change and re-hydrates when
 * another tab writes the same key. Returns a `rehydrate()` function.
 */
export function persistStore<T>(store: StoreApi<T>, { key, version = 1, pick, merge }: PersistOptions<T>) {
  const rehydrate = () => {
    const persisted = read(key)
    if (persisted !== undefined) store.setState(merge(persisted, store.getState()))
  }
  rehydrate()
  let last = JSON.stringify(pick(store.getState()))
  store.subscribe((state) => {
    const next = JSON.stringify(pick(state))
    if (next === last) return
    last = next
    write(key, JSON.parse(next), version)
  })
  if (typeof window !== 'undefined') {
    window.addEventListener('storage', (event) => {
      if (event.key === key) rehydrate()
    })
  }
  return rehydrate
}
