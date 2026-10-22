import { describe, expect, it } from 'vitest'
import {
  AUTH_STORAGE_KEY,
  isAdmin,
  isStaff,
  isTokenExpired,
  jwtExpiry,
  rehydrateAuth,
  useAuthStore,
} from './auth'
import { UI_STORAGE_KEY, useUiStore } from './ui'

const user = { id: 1, email: 'a@example.com', is_admin: false, is_staff: true } as never
const jwt = (payload: object) => `x.${btoa(JSON.stringify(payload)).replace(/=+$/, '')}.y`

describe('auth store', () => {
  it('logs in, persists and logs out', () => {
    useAuthStore.getState().login('token-1', user)
    expect(useAuthStore.getState()).toMatchObject({ token: 'token-1', hydrated: true })
    const saved = JSON.parse(localStorage.getItem(AUTH_STORAGE_KEY) ?? '{}') as { state: { token: string } }
    expect(saved.state.token).toBe('token-1')
    useAuthStore.getState().logout()
    expect(useAuthStore.getState().token).toBeNull()
    expect(JSON.parse(localStorage.getItem(AUTH_STORAGE_KEY) ?? '{}')).toMatchObject({
      state: { token: null },
    })
  })

  it('drops expired tokens on rehydrate', () => {
    const expired = jwt({ exp: Math.floor(Date.now() / 1000) - 60 })
    localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify({ state: { token: expired, user }, version: 1 }))
    rehydrateAuth()
    expect(useAuthStore.getState().token).toBeNull()
    const valid = jwt({ exp: Math.floor(Date.now() / 1000) + 3600 })
    localStorage.setItem(AUTH_STORAGE_KEY, JSON.stringify({ state: { token: valid, user }, version: 1 }))
    rehydrateAuth()
    expect(useAuthStore.getState().token).toBe(valid)
  })

  it('parses JWT expiry and role helpers', () => {
    expect(jwtExpiry(jwt({ exp: 123 }))).toBe(123)
    expect(jwtExpiry('not-a-jwt')).toBeNull()
    expect(isTokenExpired(jwt({ exp: 1 }))).toBe(true)
    expect(isTokenExpired('opaque-token')).toBe(false)
    expect(isStaff(user)).toBe(true)
    expect(isAdmin(user)).toBe(false)
    expect(isStaff(null)).toBe(false)
  })
})

describe('ui store', () => {
  it('cycles theme system → light → dark → system and persists', () => {
    useUiStore.setState({ theme: 'system' })
    useUiStore.getState().cycleTheme()
    expect(useUiStore.getState().theme).toBe('light')
    useUiStore.getState().cycleTheme()
    expect(useUiStore.getState().theme).toBe('dark')
    useUiStore.getState().cycleTheme()
    expect(useUiStore.getState().theme).toBe('system')
    useUiStore.getState().setTheme('dark')
    expect(JSON.parse(localStorage.getItem(UI_STORAGE_KEY) ?? '{}')).toMatchObject({
      state: { theme: 'dark' },
    })
  })

  it('rejects unknown locales and toggles the sidebar', () => {
    useUiStore.getState().setLocale('en')
    expect(useUiStore.getState().locale).toBe('en')
    useUiStore.getState().setLocale('fr' as never)
    expect(useUiStore.getState().locale).toBe('zh-CN')
    const before = useUiStore.getState().sidebarCollapsed
    useUiStore.getState().toggleSidebar()
    expect(useUiStore.getState().sidebarCollapsed).toBe(!before)
  })
})
