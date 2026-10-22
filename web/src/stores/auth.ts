/**
 * Session store: JWT + current user, persisted to localStorage (`gg-auth`).
 *
 *   const user = useAuthStore((s) => s.user)
 *   useAuthStore.getState().login(token, user)
 *   useAuthStore.getState().logout()
 *
 * Logging out in one tab logs out the others (storage event).
 */
import { create } from 'zustand'
import type { User } from '@/lib/api/types'
import { persistStore } from './storage'

export const AUTH_STORAGE_KEY = 'gg-auth'

export interface AuthState {
  token: string | null
  user: User | null
  /** True once persisted state has been read (synchronous with localStorage). */
  hydrated: boolean
  login: (token: string, user: User) => void
  setUser: (user: User) => void
  logout: () => void
}

/** Reads `exp` (seconds) from a JWT without verifying it. Returns `null` if absent/unparseable. */
export function jwtExpiry(token: string): number | null {
  const part = token.split('.')[1]
  if (!part) return null
  try {
    const json = atob(part.replace(/-/g, '+').replace(/_/g, '/').padEnd(Math.ceil(part.length / 4) * 4, '='))
    const exp = (JSON.parse(json) as { exp?: unknown }).exp
    return typeof exp === 'number' ? exp : null
  } catch {
    return null
  }
}

export function isTokenExpired(token: string, now = Date.now()): boolean {
  const exp = jwtExpiry(token)
  return exp !== null && exp * 1000 <= now
}

export const useAuthStore = create<AuthState>()((set) => ({
  token: null,
  user: null,
  hydrated: false,
  login: (token, user) => set({ token, user }),
  setUser: (user) => set({ user }),
  logout: () => set({ token: null, user: null }),
}))

/** Re-reads the session from storage (also wired to cross-tab `storage` events). */
export const rehydrateAuth = persistStore(useAuthStore, {
  key: AUTH_STORAGE_KEY,
  pick: (s) => ({ token: s.token, user: s.user }),
  merge: (persisted) => {
    const p = (persisted ?? {}) as Partial<AuthState>
    const token = typeof p.token === 'string' && !isTokenExpired(p.token) ? p.token : null
    return { token, user: token && p.user && typeof p.user === 'object' ? p.user : null }
  },
})
useAuthStore.setState({ hydrated: true })

export const isAuthenticated = () => Boolean(useAuthStore.getState().token)
export const isAdmin = (user: User | null | undefined) => Boolean(user?.is_admin)
export const isStaff = (user: User | null | undefined) => Boolean(user && (user.is_admin || user.is_staff))
