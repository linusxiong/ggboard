/**
 * The boot script's session mark and the auth store read the same storage with separate code: the mark picks
 * what the snapshot paints, the store what the app then renders. They have to agree, or the page changes at the swap.
 */
import { afterEach, beforeEach, describe, expect, it } from 'vitest'
import type { User } from '@/lib/api/types'
import { AUTH_STORAGE_KEY, rehydrateAuth, useAuthStore } from '@/stores/auth'
import { markSession } from '../../scripts/session-mark.mjs'

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
