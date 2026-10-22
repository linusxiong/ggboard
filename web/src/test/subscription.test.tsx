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
