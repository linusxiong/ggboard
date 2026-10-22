import { isRedirect } from '@tanstack/react-router'
import { describe, expect, it } from 'vitest'
import { useAuthStore } from '@/stores/auth'
import { homeFor, redirectIfAuthenticated, requireAuth, safeRedirect } from './guards'

describe('guards', () => {
  it('only accepts same-origin paths as redirect targets', () => {
    expect(safeRedirect('/orders?page=2')).toBe('/orders?page=2')
    expect(safeRedirect('//evil.example.com')).toBe('/dashboard')
    expect(safeRedirect('https://evil.example.com')).toBe('/dashboard')
    expect(safeRedirect('/\\evil')).toBe('/dashboard')
    expect(safeRedirect(undefined, '/admin/dashboard')).toBe('/admin/dashboard')
  })

  it('sends staff to the admin area', () => {
    expect(homeFor({ is_admin: false, is_staff: true } as never)).toBe('/admin/dashboard')
    expect(homeFor({ is_admin: false, is_staff: false } as never)).toBe('/dashboard')
  })

  it('redirects anonymous users to /login with the current href', () => {
    useAuthStore.setState({ token: null, user: null })
    try {
      requireAuth({ location: { href: '/orders?page=3' } })
      expect.unreachable()
    } catch (error) {
      expect(isRedirect(error)).toBe(true)
      expect((error as { options: { to: string; search: unknown } }).options).toMatchObject({
        to: '/login',
        search: { redirect: '/orders?page=3' },
      })
    }
  })

  it('bounces signed-in users away from the auth pages unless a one-time token is present', () => {
    useAuthStore.setState({ token: 't', user: { is_admin: false, is_staff: false } as never })
    expect(() => redirectIfAuthenticated({ search: {} })).toThrow()
    expect(() => redirectIfAuthenticated({ search: { token: 'one-time' } })).not.toThrow()
  })
})
