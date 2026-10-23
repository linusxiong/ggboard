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

  it('keeps ordinary in-app targets (path, search and hash)', () => {
    expect(safeRedirect('/admin/users?page=3&email=a%40example.com')).toBe(
      '/admin/users?page=3&email=a%40example.com',
    )
    expect(safeRedirect('/profile?tab=security#password')).toBe('/profile?tab=security#password')
    expect(safeRedirect('/orders/ABC123')).toBe('/orders/ABC123')
    // Returned in resolved form: what the router gets is exactly what was checked.
    expect(safeRedirect('/orders/../tickets')).toBe('/tickets')
  })

  it.each([
    ['protocol-relative', '//evil.example'],
    ['slash + backslash', '/\\evil.example'],
    ['backslash anywhere', '/orders\\..\\..\\evil'],
    ['leading TAB', '\t//evil.example'],
    ['TAB between slashes', '/\t/evil.example'],
    ['LF between slashes', '/\n/evil.example'],
    ['CR + backslash', '/\r\\evil.example'],
    ['TAB + backslash', '/\t\\evil.example'],
    ['LF inside a path', '/orders\n/x'],
    ['NUL', '/\0/evil.example'],
    ['DEL', '/\x7f/evil.example'],
    ['C1 control', '/\x85/evil.example'],
    ['space', '/ /evil.example'],
    ['leading space', ' /dashboard'],
    ['no-break space', '/ /evil.example'],
    ['ideographic space', '/　/evil.example'],
    ['line separator', '/ /evil.example'],
    ['dot segment → //', '/..//evil.example'],
    ['dot + backslash', '/./\\evil.example'],
    ['encoded slash → //', '/%2F/evil.example'],
    ['encoded slash, lower case', '/%2f%2fevil.example'],
    ['encoded backslash → /\\', '/%5Cevil.example'],
    ['encoded TAB', '/%09/evil.example'],
    ['encoded NUL', '/orders%00'],
    ['malformed escape', '/%E0%A4%A'],
    ['javascript: URL', 'javascript:alert(1)'],
    ['javascript: with TAB', 'java\tscript:alert(1)'],
    ['javascript: after a leading space', ' javascript:alert(1)'],
    ['absolute URL', 'https://evil.example/dashboard'],
    ['relative path', 'dashboard'],
    ['empty string', ''],
    ['non-string', 42],
  ])('rejects %s', (_name, target) => {
    expect(safeRedirect(target, '/fallback')).toBe('/fallback')
  })

  it('treats `/javascript:` as an ordinary same-origin path, not a URL scheme', () => {
    expect(safeRedirect('/javascript:alert(1)')).toBe('/javascript:alert(1)')
    expect(new URL(safeRedirect('/javascript:alert(1)'), 'https://panel.example.com').origin).toBe(
      'https://panel.example.com',
    )
  })

  it('accepts encoded characters that cannot change the origin', () => {
    expect(safeRedirect('/knowledge?keyword=%E4%B8%AD%E6%96%87')).toBe(
      '/knowledge?keyword=%E4%B8%AD%E6%96%87',
    )
    expect(safeRedirect('/knowledge?keyword=a%20b')).toBe('/knowledge?keyword=a%20b')
    expect(safeRedirect('/tickets/a%2Fb')).toBe('/tickets/a%2Fb')
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

  it('sends signed-in users home instead of to a smuggled off-site `?redirect=`', () => {
    useAuthStore.setState({ token: 't', user: { is_admin: false, is_staff: false } as never })
    for (const [redirect, to] of [
      ['/\t/evil.example', '/dashboard'],
      ['/..//evil.example', '/dashboard'],
      ['/orders?page=2', '/orders?page=2'],
    ] as const) {
      try {
        redirectIfAuthenticated({ search: { redirect } })
        expect.unreachable()
      } catch (error) {
        expect(isRedirect(error)).toBe(true)
        expect((error as { options: { to: string } }).options.to).toBe(to)
      }
    }
  })
})
