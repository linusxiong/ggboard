import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Passkey, SecurityOverview } from '@/lib/api/types'
import { queryClient } from '@/lib/queryClient'
import { useAuthStore } from '@/stores/auth'
import {
  bodiesOf,
  GUEST_CONFIG,
  json,
  mockApi,
  renderAt,
  resetStores,
  signIn,
  TEST_USER,
} from './fe1-helpers'

const webauthn = vi.hoisted(() => ({ startRegistration: vi.fn(), startAuthentication: vi.fn() }))
vi.mock('@simplewebauthn/browser', () => ({
  ...webauthn,
  WebAuthnAbortService: { cancelCeremony: vi.fn(), createNewAbortSignal: vi.fn() },
  browserSupportsWebAuthn: () => true,
}))

const CODES = Array.from({ length: 10 }, (_, i) => `abcd${i}-efgh${i}`)
const CREDENTIAL = {
  id: 'cred-id',
  rawId: 'cred-id',
  type: 'public-key',
  response: { clientDataJSON: 'cdj', attestationObject: 'ao', transports: ['internal'] },
  clientExtensionResults: {},
  authenticatorAttachment: 'platform',
}
const OPTIONS = {
  rp: { name: 'GG Bond', id: 'panel.example.com' },
  user: { name: 'demo@example.com', displayName: 'demo@example.com', id: 'dXNlcg' },
  challenge: 'Y2hhbGxlbmdl',
  pubKeyCredParams: [{ type: 'public-key', alg: -7 }],
  timeout: 120000,
  excludeCredentials: [],
  authenticatorSelection: {
    requireResidentKey: true,
    residentKey: 'required',
    userVerification: 'preferred',
  },
  attestation: 'none',
  extensions: { credProps: true },
}

describe('profile → security', () => {
  let overview: SecurityOverview
  beforeEach(() => {
    resetStores()
    signIn()
    overview = { totp_enabled: false, totp_enabled_at: null, backup_codes_remaining: 0, passkeys: [] }
    Object.defineProperty(window, 'PublicKeyCredential', {
      configurable: true,
      value: function PublicKeyCredential() {},
    })
    webauthn.startRegistration.mockReset()
  })
  afterEach(() => {
    queryClient.clear()
    vi.unstubAllGlobals()
    Reflect.deleteProperty(window, 'PublicKeyCredential')
  })

  it('2FA wizard: setup → QR + secret → code → backup codes (copy / download)', async () => {
    const fetchMock = mockApi({
      'GET /guest/config': { ...GUEST_CONFIG, passkey_enabled: true },
      'GET /user/me': TEST_USER,
      'GET /user/security': () => overview,
      'POST /user/security/totp/setup': {
        secret: 'JBSWY3DPEHPK3PXP',
        otpauth_url: 'otpauth://totp/GG%20Bond:demo%40example.com?secret=JBSWY3DPEHPK3PXP&issuer=GG%20Bond',
      },
      'POST /user/security/totp/enable': ({ body }: { body: unknown }) => {
        if ((body as { code: string }).code !== '123456')
          return json(
            { code: 'validation_failed', message: 'invalid', errors: { code: ['is invalid'] } },
            400,
          )
        overview = {
          ...overview,
          totp_enabled: true,
          totp_enabled_at: '2026-10-22T00:00:00Z',
          backup_codes_remaining: 10,
        }
        return { backup_codes: CODES }
      },
    })
    renderAt('/profile?tab=security')
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: /开启两步验证/ }, { timeout: 8000 }))

    const dialog = await screen.findByRole('dialog')
    expect(await within(dialog).findByText('JBSW Y3DP EHPK 3PXP')).toBeInTheDocument()
    expect(within(dialog).getByRole('link', { name: '在本设备的验证器中打开' })).toHaveAttribute(
      'href',
      expect.stringMatching(/^otpauth:\/\/totp\//),
    )
    const code = within(dialog).getByLabelText('验证码')
    expect(code).toHaveAttribute('autocomplete', 'one-time-code')

    await user.type(code, '000000')
    expect(await within(dialog).findByText('验证码或备用码不正确')).toBeInTheDocument()
    expect(useAuthStore.getState().token).not.toBeNull() // a 400 never ends the session

    await user.type(within(dialog).getByLabelText('验证码'), '123456')
    const codesDialog = await screen.findByRole('dialog', { name: '保存备用码' })
    for (const c of CODES) expect(within(codesDialog).getByText(c)).toBeInTheDocument()
    expect(within(codesDialog).getByRole('button', { name: '复制全部' })).toBeInTheDocument()
    expect(within(codesDialog).getByRole('button', { name: /下载 \.txt/ })).toBeInTheDocument()
    const close = within(codesDialog).getByRole('button', { name: '关闭' })
    expect(close).toBeDisabled()
    await user.click(within(codesDialog).getByRole('checkbox', { name: '我已妥善保存备用码' }))
    await user.click(close)

    expect(await screen.findByText('剩余 10 个可用备用码')).toBeInTheDocument()
    expect(bodiesOf(fetchMock, 'POST /user/security/totp/enable')).toEqual([
      { code: '000000' },
      { code: '123456' },
    ])
  })

  it('adds a passkey: name → startRegistration(optionsJSON) → finish with the credential', async () => {
    const created: Passkey = {
      id: 7,
      name: 'Work laptop',
      created_at: '2026-10-22T00:00:00Z',
      last_used_at: null,
      backup_eligible: true,
      transports: ['internal'],
    }
    webauthn.startRegistration.mockResolvedValueOnce(CREDENTIAL)
    const fetchMock = mockApi({
      'GET /guest/config': { ...GUEST_CONFIG, passkey_enabled: true },
      'GET /user/me': TEST_USER,
      'GET /user/security': () => overview,
      'POST /user/security/passkeys/begin': { challenge_id: 'ch-1', options: OPTIONS },
      'POST /user/security/passkeys/finish': () => {
        overview = { ...overview, passkeys: [created] }
        return created
      },
    })
    renderAt('/profile?tab=security')
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: /添加通行密钥/ }, { timeout: 8000 }))
    const dialog = await screen.findByRole('dialog', { name: '添加通行密钥' })
    const name = within(dialog).getByLabelText('名称')
    await user.clear(name)
    await user.type(name, 'Work laptop')
    await user.click(within(dialog).getByRole('button', { name: '创建' }))

    expect(await screen.findByText('Work laptop')).toBeInTheDocument()
    expect(webauthn.startRegistration).toHaveBeenCalledWith({ optionsJSON: OPTIONS })
    expect(bodiesOf(fetchMock, 'POST /user/security/passkeys/begin')).toEqual([{ name: 'Work laptop' }])
    expect(bodiesOf(fetchMock, 'POST /user/security/passkeys/finish')).toEqual([
      { challenge_id: 'ch-1', credential: CREDENTIAL, name: 'Work laptop' },
    ])
  })

  it('treats a cancelled ceremony (NotAllowedError) as a gentle notice', async () => {
    webauthn.startRegistration.mockRejectedValueOnce(
      Object.assign(new Error('cancelled'), { name: 'NotAllowedError' }),
    )
    const fetchMock = mockApi({
      'GET /guest/config': { ...GUEST_CONFIG, passkey_enabled: true },
      'GET /user/me': TEST_USER,
      'GET /user/security': () => overview,
      'POST /user/security/passkeys/begin': { challenge_id: 'ch-2', options: OPTIONS },
    })
    renderAt('/profile?tab=security')
    const user = userEvent.setup()
    await user.click(await screen.findByRole('button', { name: /添加通行密钥/ }, { timeout: 8000 }))
    const dialog = await screen.findByRole('dialog', { name: '添加通行密钥' })
    await user.click(within(dialog).getByRole('button', { name: '创建' }))
    expect(await within(dialog).findByText('已取消创建通行密钥，可随时重试。')).toHaveAttribute(
      'role',
      'status',
    )
    expect(bodiesOf(fetchMock, 'POST /user/security/passkeys/finish')).toHaveLength(0)
    await waitFor(() => expect(within(dialog).getByRole('button', { name: '创建' })).toBeEnabled())
  })
})
