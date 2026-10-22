/**
 * Password-manager compatibility (AGENTS.md §1 item 16): real <form>s, native inputs with id/name and a
 * <label for>, the right `autocomplete` tokens, `type="password"` by default.
 */
import { screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { queryClient } from '@/lib/queryClient'
import { GUEST_CONFIG, mockApi, renderAt, resetStores, signIn, TEST_USER } from './fe1-helpers'

/** The input a visible <label for> points at, checked for the attributes managers rely on. */
function field(label: RegExp | string) {
  const input = screen.getByLabelText(label)
  expect(input.tagName).toBe('INPUT')
  expect(input.id).not.toBe('')
  expect(input.getAttribute('name')).toBeTruthy()
  expect(document.querySelector(`label[for="${input.id}"]`)).not.toBeNull()
  expect(input.closest('form')).not.toBeNull()
  return input
}

describe('password-manager autofill attributes', () => {
  beforeEach(() => {
    resetStores()
    mockApi({
      'GET /guest/config': { ...GUEST_CONFIG, email_verify: true },
      'GET /user/me': TEST_USER,
      'GET /user/security': {
        totp_enabled: false,
        totp_enabled_at: null,
        backup_codes_remaining: 0,
        passkeys: [],
      },
      'POST /passport/login': {
        mfa_required: true,
        mfa_token: 'x'.repeat(64),
        methods: ['totp'],
        has_passkey: false,
      },
    })
  })
  afterEach(() => {
    queryClient.clear()
    vi.unstubAllGlobals()
  })

  it('login: username + webauthn, current-password; MFA: one-time-code', async () => {
    renderAt('/login')
    const email = await screen.findByLabelText('邮箱', undefined, { timeout: 8000 })
    expect(field('邮箱')).toHaveAttribute('autocomplete', 'username webauthn')
    expect(email).toHaveAttribute('type', 'email')
    const password = field('密码')
    expect(password).toHaveAttribute('autocomplete', 'current-password')
    expect(password).toHaveAttribute('type', 'password')

    const user = userEvent.setup()
    await user.type(email, 'a@example.com')
    await user.type(password, 'secret-pass')
    await user.keyboard('{Enter}') // Enter submits the real <form>
    await screen.findByLabelText('验证码')
    const code = field('验证码')
    expect(code).toHaveAttribute('autocomplete', 'one-time-code')
    expect(code).toHaveAttribute('inputmode', 'numeric')
    expect(code).toHaveAttribute('pattern', '[0-9]*')
    await user.click(screen.getByRole('button', { name: '使用备用码' }))
    expect(field('备用码')).toHaveAttribute('autocomplete', 'off')
  })

  it('register: username + new-password, email code one-time-code', async () => {
    renderAt('/register')
    await screen.findByLabelText('邮箱', undefined, { timeout: 8000 })
    expect(field('邮箱')).toHaveAttribute('autocomplete', 'username')
    const password = field('密码')
    expect(password).toHaveAttribute('autocomplete', 'new-password')
    expect(password).toHaveAttribute('type', 'password')
    expect(field('邮箱验证码')).toHaveAttribute('autocomplete', 'one-time-code')
  })

  it('forgot: username, one-time-code (numeric), new-password', async () => {
    renderAt('/forgot')
    await screen.findByLabelText('邮箱', undefined, { timeout: 8000 })
    expect(field('邮箱')).toHaveAttribute('autocomplete', 'username')
    const code = field('邮箱验证码')
    expect(code).toHaveAttribute('autocomplete', 'one-time-code')
    expect(code).toHaveAttribute('inputmode', 'numeric')
    const password = field('新密码')
    expect(password).toHaveAttribute('autocomplete', 'new-password')
    expect(password).toHaveAttribute('type', 'password')
  })

  it('change password: read-only username, current-password, new-password', async () => {
    signIn()
    renderAt('/profile?tab=security')
    await screen.findByLabelText('当前密码', undefined, { timeout: 8000 })
    const username = field('账户')
    expect(username).toHaveAttribute('autocomplete', 'username')
    expect(username).toHaveValue(TEST_USER.email)
    expect(username).toHaveAttribute('readonly')
    const current = field('当前密码')
    expect(current).toHaveAttribute('autocomplete', 'current-password')
    expect(current).toHaveAttribute('type', 'password')
    const next = field('新密码')
    expect(next).toHaveAttribute('autocomplete', 'new-password')
    expect(next).toHaveAttribute('type', 'password')
  })
})
