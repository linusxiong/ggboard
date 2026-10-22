import { screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth'
import { bodiesOf, GUEST_CONFIG, json, mockApi, renderAt, resetStores, TEST_USER } from './fe1-helpers'

const CHALLENGE = { mfa_required: true, mfa_token: 'f'.repeat(64), methods: ['totp'], has_passkey: false }
const SESSION = 'header.eyJleHAiOjQxMDI0NDQ4MDB9.sig'

async function passwordStep() {
  const user = userEvent.setup()
  await user.type(await screen.findByLabelText(/邮箱/, undefined, { timeout: 8000 }), 'a@example.com')
  await user.type(screen.getByLabelText(/^密码/), 'correct horse')
  await user.click(screen.getByRole('button', { name: '登录' }))
  return user
}

describe('login: second factor', () => {
  beforeEach(() => resetStores())
  afterEach(() => vi.unstubAllGlobals())

  it('asks for the TOTP code, shows a wrong code, then signs in', async () => {
    let attempts = 0
    const fetchMock = mockApi({
      'GET /guest/config': GUEST_CONFIG,
      'POST /passport/login': CHALLENGE,
      'POST /passport/login/mfa': () =>
        ++attempts === 1
          ? json({ code: 'mfa_invalid', message: 'invalid code' }, 401)
          : { token: SESSION, user: TEST_USER },
    })
    const { router } = renderAt('/login?redirect=/subscription')
    const user = await passwordStep()

    expect(await screen.findByRole('heading', { level: 1, name: '两步验证' })).toBeInTheDocument()
    const code = await screen.findByLabelText(/验证码/)
    expect(code).toHaveFocus()
    expect(code).toHaveAttribute('autocomplete', 'one-time-code')
    expect(code).toHaveAttribute('inputmode', 'numeric')
    expect(code).toHaveAttribute('pattern', '[0-9]*')
    expect(useAuthStore.getState().token).toBeNull()

    await user.type(code, '111111') // 6 digits submit on their own (autofill-friendly)
    expect(await screen.findByText('验证码错误，请重试。')).toBeInTheDocument()

    await user.type(screen.getByLabelText(/验证码/), '123456')
    await waitFor(() => expect(useAuthStore.getState().token).toBe(SESSION))
    expect(bodiesOf(fetchMock, 'POST /passport/login/mfa')).toEqual([
      { mfa_token: CHALLENGE.mfa_token, code: '111111' },
      { mfa_token: CHALLENGE.mfa_token, code: '123456' },
    ])
    await waitFor(() => expect(router.state.location.pathname).toBe('/subscription'))
  })

  it('accepts a backup code and goes back to the password step when the token expired', async () => {
    const fetchMock = mockApi({
      'GET /guest/config': GUEST_CONFIG,
      'POST /passport/login': CHALLENGE,
      'POST /passport/login/mfa': json({ code: 'mfa_expired', message: 'expired' }, 401),
    })
    renderAt('/login')
    const user = await passwordStep()
    await user.click(await screen.findByRole('button', { name: '使用备用码' }))
    const backup = screen.getByLabelText(/备用码/)
    expect(backup).toHaveAttribute('autocomplete', 'off')
    await user.type(backup, 'abcde-12345')
    await user.click(screen.getByRole('button', { name: '验证' }))

    expect(await screen.findByRole('heading', { level: 1, name: '登录' })).toBeInTheDocument()
    expect(screen.getByText('验证已超时，请重新登录。')).toHaveAttribute('role', 'alert')
    expect(screen.getByLabelText(/邮箱/)).toHaveValue('a@example.com')
    expect(bodiesOf(fetchMock, 'POST /passport/login/mfa')[0]).toEqual({
      mfa_token: CHALLENGE.mfa_token,
      code: 'abcde-12345',
    })
  })

  it('shows the lockout wait from Retry-After', async () => {
    mockApi({
      'GET /guest/config': GUEST_CONFIG,
      'POST /passport/login': json({ code: 'rate_limited', message: 'locked' }, 429, {
        'Retry-After': '900',
      }),
    })
    renderAt('/login')
    await passwordStep()
    expect(await screen.findByText(/尝试次数过多，请15\s?分钟后再试/)).toHaveAttribute('role', 'alert')
  })
})
