/**
 * The boot script's session mark and the auth store read the same storage with separate code: the mark picks
 * what the snapshot paints, the store what the app then renders. They have to agree, or the page changes at the swap.
 * The same holds for the theme mark and `applyTheme`: one paints the first frame, the other takes over.
 */
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import type { User } from '@/lib/api/types'
import { applyTheme } from '@/lib/theme'
import { AUTH_STORAGE_KEY, rehydrateAuth, useAuthStore } from '@/stores/auth'
import { rehydrateUi, UI_STORAGE_KEY, useUiStore } from '@/stores/ui'
import { markSession, markTheme } from '../../scripts/session-mark.mjs'

const user = { id: 1, email: 'a@example.com' } as User
const inAnHour = Math.floor(Date.now() / 1000) + 3600
const b64url = (text: string) => btoa(text).replace(/=+$/, '').replace(/\+/g, '-').replace(/\//g, '_')
const jwt = (payload: object) => `x.${b64url(JSON.stringify(payload))}.y`
const URL_SAFE = jwt({ exp: 1, sub: '~~~???>>>x' })
const stored = (token: unknown) => JSON.stringify({ state: { token, user }, version: 1 })
const marked = () => document.documentElement.dataset.session === 'member'

function reset() {
  useAuthStore.setState({ token: null, user: null })
  localStorage.clear()
  delete document.documentElement.dataset.session
}

describe('session mark', () => {
  beforeEach(reset)
  afterEach(reset)

  it('has a fixture that needs the URL-safe decoding', () => {
    const payload = URL_SAFE.split('.')[1] ?? ''
    expect(payload).toMatch(/-/)
    expect(payload).toMatch(/_/)
    expect(payload.length % 4).not.toBe(0)
  })

  it('follows a real login and logout', () => {
    useAuthStore.getState().login(jwt({ exp: inAnHour }), user)
    markSession()
    expect(marked()).toBe(true)

    useAuthStore.getState().logout()
    delete document.documentElement.dataset.session
    markSession()
    expect(marked()).toBe(false)
  })

  it.each([
    ['nothing stored', null, false],
    ['a live token', stored(jwt({ exp: inAnHour })), true],
    // URL-safe alphabet, no padding. Expired on purpose: a misread expiry keeps the token, so a live one proves nothing.
    ['an expired url-safe payload', stored(URL_SAFE), false],
    ['an expired token', stored(jwt({ exp: 1 })), false],
    ['a token without exp', stored(jwt({ sub: 1 })), true],
    ['an opaque token', stored('opaque-token'), true],
    ['a token with a broken payload', stored('x.%%%.y'), true],
    ['a logged out state', stored(null), false],
    ['an empty token', stored(''), false],
    ['a numeric token', stored(7), false],
    ['no state', '{"version":1}', false],
    ['corrupt storage', '{not json', false],
  ])('agrees with the auth store on %s', (_name, value, expected) => {
    if (value !== null) localStorage.setItem(AUTH_STORAGE_KEY, value)
    markSession()
    rehydrateAuth()
    expect(marked()).toBe(expected)
    expect(Boolean(useAuthStore.getState().token)).toBe(expected)
  })
})

describe('theme mark', () => {
  const head = document.head.innerHTML
  const ui = (theme: unknown) =>
    JSON.stringify({ state: { theme, locale: 'en', sidebarCollapsed: false }, version: 1 })

  /** The document as index.html ships it: no forced theme, one `theme-color` per system scheme. */
  function freshPage() {
    delete document.documentElement.dataset.theme
    document.head.innerHTML =
      '<meta name="theme-color" content="#F2F2F7" media="(prefers-color-scheme: light)">' +
      '<meta name="theme-color" content="#000000" media="(prefers-color-scheme: dark)">'
  }
  /** What the browser paints from: the forced theme and the toolbar colour per scheme. */
  const painted = () => ({
    theme: document.documentElement.dataset.theme,
    themeColor: [...document.querySelectorAll<HTMLMetaElement>('meta[name="theme-color"]')].map((meta) => ({
      media: meta.getAttribute('media'),
      content: meta.content,
    })),
  })

  function reset() {
    localStorage.clear()
    useUiStore.setState({ theme: 'system' })
    localStorage.clear()
    freshPage()
  }
  beforeEach(reset)
  afterEach(() => {
    reset()
    document.head.innerHTML = head
  })

  it('follows a real theme choice', () => {
    useUiStore.getState().setTheme('dark')
    markTheme()
    expect(painted().theme).toBe('dark')

    freshPage()
    useUiStore.getState().setTheme('system')
    markTheme()
    expect(painted().theme).toBeUndefined()
  })

  it.each([
    ['nothing stored', null, undefined],
    ['the system theme', ui('system'), undefined],
    ['a forced light theme', ui('light'), 'light'],
    ['a forced dark theme', ui('dark'), 'dark'],
    ['an unknown theme', ui('solarized'), undefined],
    // A key that only exists on Object.prototype must not pass for a theme.
    ['a prototype key', ui('constructor'), undefined],
    ['a non-string theme', ui(1), undefined],
    ['no state', '{"version":1}', undefined],
    ['corrupt storage', '{not json', undefined],
  ])('agrees with applyTheme on %s', (_name, value, expected) => {
    if (value !== null) localStorage.setItem(UI_STORAGE_KEY, value)
    markTheme()
    const boot = painted()
    expect(boot.theme).toBe(expected)

    freshPage()
    rehydrateUi()
    applyTheme(useUiStore.getState().theme)
    expect(painted()).toEqual(boot)
  })

  it('hands over to the app, which can still return to the system theme', () => {
    const system = painted()
    localStorage.setItem(UI_STORAGE_KEY, ui('dark'))
    markTheme()
    const boot = painted()
    expect(boot.themeColor).toEqual([
      { media: null, content: '#000000' },
      { media: null, content: '#000000' },
    ])

    applyTheme('dark')
    expect(painted()).toEqual(boot)
    applyTheme('system')
    expect(painted()).toEqual(system)
  })
})
