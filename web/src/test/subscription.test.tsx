import { readFileSync } from 'node:fs'
import path from 'node:path'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { applyTemplate, findTemplate } from '@/features/subscription/ruleProfile'
import type { RuleProfile, RulesetCatalog } from '@/lib/api/types'
import { queryClient } from '@/lib/queryClient'
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

const catalog = JSON.parse(
  readFileSync(
    path.resolve(import.meta.dirname, '../../../internal/subscription/rulesets/catalog.json'),
    'utf8',
  ),
) as RulesetCatalog
const saved: RuleProfile = applyTemplate(findTemplate(catalog, 'standard')!, { source: 'blackmatrix7' })
// What GET /user/rulesets/catalog really returns (Go fixture, kept current by TestPublicCatalogFixture).
const publicCatalog = JSON.parse(
  readFileSync(
    path.resolve(import.meta.dirname, '../../../internal/subscription/rulesets/testdata/public-catalog.json'),
    'utf8',
  ),
) as RulesetCatalog

function api(overrides: Record<string, unknown> = {}) {
  return mockApi({
    'GET /guest/config': GUEST_CONFIG,
    'GET /user/me': TEST_USER,
    'GET /user/subscription': {
      subscribe_url: 'https://panel.example.com/s/tok',
      token: 'tok',
      plan: { id: 2, name: 'Pro' },
      transfer_enable: 0,
      u: 0,
      d: 0,
      expired_at: null,
      next_reset_at: null,
      reset_day: null,
      device_limit: null,
      speed_limit: null,
    },
    'GET /user/rulesets/catalog': catalog,
    'GET /user/rule-profile': saved,
    'PUT /user/rule-profile': ({ body }: { body: unknown }) => body,
    ...overrides,
  })
}

describe('subscription page', () => {
  beforeEach(() => {
    resetStores()
    signIn()
  })
  afterEach(() => {
    queryClient.clear()
    vi.unstubAllGlobals()
  })

  it('toggling categories changes the payload sent to PUT /user/rule-profile', async () => {
    const fetchMock = api()
    renderAt('/subscription')
    const user = userEvent.setup()

    const twitter = await screen.findByRole('switch', { name: /X \/ Twitter/ }, { timeout: 8000 })
    expect(twitter).not.toBeChecked()
    const ads = screen.getByRole('switch', { name: /广告拦截/ })
    expect(ads).toBeChecked()

    await user.click(twitter)
    await user.click(ads)
    expect(screen.getByText('有未保存的更改')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '保存规则' }))
    await waitFor(() => expect(bodiesOf(fetchMock, 'PUT /user/rule-profile')).toHaveLength(1))
    const sent = bodiesOf(fetchMock, 'PUT /user/rule-profile')[0] as RuleProfile
    expect(sent.categories.twitter).toBe('Proxy')
    expect(sent.categories).not.toHaveProperty('ads')
    expect(sent.categories.openai).toBe('AI')
    expect(sent.template).toBe('standard')
    expect(sent.groups.map((g) => g.name)).toEqual(saved.groups.map((g) => g.name))
    expect(await screen.findByText('所有更改已保存')).toBeInTheDocument()
  })

  it('keeps explanations behind ⓘ buttons (second level)', async () => {
    api()
    renderAt('/subscription')
    const user = userEvent.setup()
    await screen.findByRole('switch', { name: /广告拦截/ }, { timeout: 8000 })
    // Section and source explanations are not on the page until their ⓘ is opened.
    expect(screen.queryByText('模板决定默认启用的分类和策略组。')).toBeNull()
    await user.click(screen.getByRole('button', { name: '关于「模板」的说明' }))
    expect(await screen.findByRole('dialog')).toHaveTextContent('模板决定默认启用的分类和策略组。')
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    // A switch's explanation sits outside its <label>: the switch keeps its own name.
    const geoip = screen.getByRole('switch', { name: '中国大陆 IP 直连（GEOIP,CN,DIRECT）' })
    expect(geoip).toBeInTheDocument()
    await user.click(
      screen.getByRole('button', { name: '关于「中国大陆 IP 直连（GEOIP,CN,DIRECT）」的说明' }),
    )
    expect(await screen.findByRole('dialog')).toHaveTextContent('在兜底规则前追加，适合国内使用。')
    await user.keyboard('{Escape}')
    // The source's technical description stays its radio's accessible description.
    const source = screen.getByRole('radio', { name: /MetaCubeX/ })
    expect(source).toHaveAccessibleDescription(/mihomo 官方 geosite\/geoip 规则集/)
  })

  it('shows which source each client gets: the default source needs no switch for sing-box', async () => {
    api({ 'GET /user/rulesets/catalog': publicCatalog })
    renderAt('/subscription')
    const user = userEvent.setup()
    const list = await screen.findByRole('list', { name: '各客户端实际使用' }, { timeout: 8000 })
    const rows = () =>
      within(list)
        .getAllByRole('listitem')
        .map((li) => li.textContent)
    expect(rows()).toEqual([
      'Clash Meta · sing-boxMetaCubeX/meta-rules-dat',
      'Stash · Surge · Surfboard · Loon · Quantumult Xblackmatrix7/ios_rule_script',
    ])
    expect(screen.getByRole('radio', { name: /blackmatrix7/ })).toBeChecked()
    // Why, behind the ⓘ: the popover is the dialog that appears on click.
    expect(screen.queryByText(/不需要为某个客户端手动切换来源/)).toBeNull()
    await user.click(screen.getByRole('button', { name: '关于「各客户端实际使用」的说明' }))
    const dialogs = await screen.findAllByRole('dialog')
    expect(dialogs[dialogs.length - 1]).toHaveTextContent(/blackmatrix7 没有 sing-box 规则集/)
    await user.keyboard('{Escape}')
    await waitFor(() => expect(screen.queryByRole('dialog')).toBeNull())
    // The list follows the draft: SagerNet serves sing-box except `gfw`, which MetaCubeX fills in.
    await user.click(screen.getByRole('radio', { name: /SagerNet/ }))
    expect(rows()).toContain('sing-boxSagerNet/sing-geosite + SagerNet/sing-geoip + MetaCubeX/meta-rules-dat')
  })

  it('shows API validation errors inline', async () => {
    api({
      'PUT /user/rule-profile': json(
        { code: 'validation_failed', message: 'invalid', errors: { 'groups[1].name': ['名称不可用'] } },
        400,
      ),
    })
    renderAt('/subscription')
    const user = userEvent.setup()
    await user.click(await screen.findByRole('switch', { name: /X \/ Twitter/ }, { timeout: 8000 }))
    await user.click(screen.getByRole('button', { name: '保存规则' }))
    expect(await screen.findByText('名称不可用')).toBeInTheDocument()
    expect(screen.getAllByRole('alert').some((el) => el.textContent === '请先修正标出的问题')).toBe(true)
  })

  it('shows a whole-profile API error (`errors.profile`, e.g. the size cap) as the form error', async () => {
    api({
      'PUT /user/rule-profile': json(
        {
          code: 'validation_failed',
          message: 'invalid',
          errors: { profile: ['must be at most 256 KiB as stored'] },
        },
        400,
      ),
    })
    renderAt('/subscription')
    const user = userEvent.setup()
    await user.click(await screen.findByRole('switch', { name: /X \/ Twitter/ }, { timeout: 8000 }))
    await user.click(screen.getByRole('button', { name: '保存规则' }))
    expect(await screen.findByText('must be at most 256 KiB as stored')).toHaveAttribute('role', 'alert')
    expect(screen.queryByText('请先修正标出的问题')).toBeNull()
  })

  it('builds the import links and the template override link', async () => {
    api()
    renderAt('/subscription')
    const link = await screen.findByRole('link', { name: /导入到 Clash/ }, { timeout: 8000 })
    expect(link).toHaveAttribute(
      'href',
      `clash://install-config?url=${encodeURIComponent('https://panel.example.com/s/tok')}&name=GG%20Bond`,
    )
    const url = screen.getByLabelText('订阅地址')
    expect(url).toHaveValue('https://panel.example.com/s/tok')
    const card = url.closest('section')!
    expect(
      within(card)
        .getByRole('link', { name: /导入到 Shadowrocket/ })
        .getAttribute('href'),
    ).toMatch(/^shadowrocket:\/\/add\/sub:\/\//)
  })

  it('is read-only when the admin disabled custom rules', async () => {
    api({ 'GET /guest/config': { ...GUEST_CONFIG, allow_user_rule_profile: false } })
    renderAt('/subscription')
    expect(
      await screen.findByText('站点管理员已关闭自定义分流规则，所有用户使用默认模板。', undefined, {
        timeout: 8000,
      }),
    ).toBeInTheDocument()
    expect(screen.getByRole('switch', { name: /X \/ Twitter/ })).toBeDisabled()
    expect(screen.queryByRole('button', { name: '保存规则' })).not.toBeInTheDocument()
  })
})
