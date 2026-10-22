import { screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'
import { mockApi, renderApp, resetApp, testConfig } from '@/features/shop/testing'
import { buildInviteLink, parseAmount } from './inviteLink'

describe('invite link builder', () => {
  it('builds <app_url>/register?code=<code>', () => {
    expect(buildInviteLink('https://panel.example.com', 'Ab3d')).toBe(
      'https://panel.example.com/register?code=Ab3d',
    )
    expect(buildInviteLink(' https://panel.example.com/// ', 'a b&c')).toBe(
      'https://panel.example.com/register?code=a%20b%26c',
    )
  })

  it('parses currency amounts into cents', () => {
    expect(parseAmount(12.5)).toBe(1250)
    expect(parseAmount(0.1 + 0.2)).toBe(30)
    expect(parseAmount(0)).toBeNull()
    expect(parseAmount(null)).toBeNull()
    expect(parseAmount(Number.NaN)).toBeNull()
  })
})

describe('invite page', () => {
  afterEach(resetApp)

  it('shows stats, the invite link for the selected code and respects the code limit', async () => {
    mockApi({
      'GET /user/invite': () => ({
        codes: [
          { code: 'CODEONE', pv: 3, created_at: '2026-10-01T00:00:00Z' },
          { code: 'CODETWO', pv: 0, created_at: '2026-10-02T00:00:00Z' },
        ],
        stat: {
          register_count: 4,
          commission_pending: 1800,
          commission_paid: 7800,
          commission_rate: 10,
          invite_gen_limit: 2,
        },
      }),
      'GET /user/invite/commissions': () => [],
    })
    renderApp('/invite')
    const u = userEvent.setup()

    expect(
      await screen.findByRole('heading', { level: 1, name: '邀请返利' }, { timeout: 4000 }),
    ).toBeInTheDocument()
    const link = screen.getByLabelText('邀请链接地址')
    expect(link).toHaveTextContent(`${testConfig.app_url}/register?code=CODEONE`)
    expect(screen.getByText('已生成 2 / 2 个未使用的邀请码')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /已达上限/ })).toBeDisabled()
    expect(screen.getByText('10%')).toBeInTheDocument()
    expect(await screen.findByText('暂无佣金记录')).toBeInTheDocument()

    // QR code for the link on demand.
    await u.click(screen.getByRole('button', { name: '显示二维码' }))
    expect(await screen.findByRole('img', { name: '邀请链接二维码' })).toBeInTheDocument()

    // Switch code → link follows.
    const section = screen.getByRole('heading', { name: '邀请链接' }).closest('section')
    expect(section).not.toBeNull()
    await u.click(within(section as HTMLElement).getByRole('button', { name: /CODEONE/ }))
    await u.click(await screen.findByRole('option', { name: 'CODETWO' }))
    expect(screen.getByLabelText('邀请链接地址')).toHaveTextContent(
      `${testConfig.app_url}/register?code=CODETWO`,
    )
  })
})
