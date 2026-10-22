/**
 * FE-S: `/subscription` security + age cards — access log rendering, link rotation (token only) and the age
 * key flow (generate → enable → reveal/copy the private key → regenerate with confirm).
 */
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { AgeKey, SubscriptionAccess } from '@/lib/api/types'
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

if (!('getAnimations' in Element.prototype))
  Object.defineProperty(Element.prototype, 'getAnimations', { configurable: true, value: () => [] })

const T = { timeout: 8000 }
const BASE_URL = 'https://panel.example.com/s/tok'
const NOT_FOUND = () => json({ code: 'not_found', message: 'not found' }, 404)
const hoursAgo = (h: number) => new Date(Date.now() - h * 3_600_000).toISOString()

const LOG: SubscriptionAccess[] = [
  { ip: '203.0.113.*', client: 'clash-verge/v2.2.3', format: 'clash-meta', at: hoursAgo(30) },
  { ip: '198.51.100.*', client: 'Shadowrocket/2.2.65', format: 'shadowrocket', at: hoursAgo(1) },
  { ip: '203.0.113.*', client: 'mihomo/1.19.10', format: 'clash-meta', at: hoursAgo(2) },
  { ip: '192.0.2.*', client: '', format: 'v2ray', at: hoursAgo(5) },
]

const PUB = (n: number) => `age1publickey${n}xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx`
const PRIV = (n: number) => `AGE-SECRET-KEY-1PRIVATE${n}QQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQQ`
/** Backend age view: null key fields without a key. */
const view = (key: { n: number; enabled: boolean } | null, extra: Partial<AgeKey> = {}): AgeKey => ({
  enabled: key?.enabled ?? false,
  public_key: key ? PUB(key.n) : null,
  private_key: key ? PRIV(key.n) : null,
  created_at: key ? '2026-10-22T08:00:00Z' : null,
  formats: ['clash-meta'],
  optional: true,
  strict: false,
  ...extra,
})

function api(overrides: Record<string, unknown> = {}) {
  let token = 'tok'
  return mockApi({
    'GET /guest/config': { ...GUEST_CONFIG, subscribe_age_optional: true },
    'GET /user/me': TEST_USER,
    'GET /user/rulesets/catalog': { sources: {}, categories: [], templates: [] },
    'GET /user/rule-profile': {
      template: 'none',
      source: '',
      categories: {},
      groups: [{ name: 'Proxy', type: 'select', include: '*' }],
      final: 'Proxy',
      geoip_cn_direct: false,
      mirror: '',
      dns: 'default',
    },
    'GET /user/rule-profiles': NOT_FOUND,
    'GET /user/subscription': () => ({
      subscribe_url: `https://panel.example.com/s/${token}`,
      token,
      plan: null,
      transfer_enable: 0,
      u: 0,
      d: 0,
      expired_at: null,
      next_reset_at: null,
      reset_day: null,
      device_limit: null,
      speed_limit: null,
      formats: [
        { id: 'clash-meta', name: 'Clash Meta', supports_age: true },
        { id: 'singbox', name: 'sing-box', supports_age: false },
      ],
    }),
    'GET /user/subscription/access-log': LOG,
    'POST /user/subscription/rotate-token': () => {
      token = 'rotated'
      return { subscribe_url: `https://panel.example.com/s/${token}`, token }
    },
    'GET /user/telegram': { enabled: true, bound: false, bot_username: 'gg_bot', discuss_link: '' },
    'GET /user/subscription/age': view(null),
    ...overrides,
  })
}

describe('subscription security', () => {
  beforeEach(() => {
    resetStores()
    signIn()
  })
  afterEach(() => {
    queryClient.clear()
    vi.unstubAllGlobals()
  })

  it('renders the access log newest first with the 24 h distinct-IP count', async () => {
    api()
    renderAt('/subscription')
    const list = await screen.findByRole('list', { name: '最近访问' }, T)
    const rows = within(list).getAllByRole('listitem')
    expect(rows.map((r) => r.querySelector('.font-mono')?.textContent)).toEqual([
      '198.51.100.*',
      '203.0.113.*',
      '192.0.2.*',
      '203.0.113.*',
    ])
    expect(within(rows[0]!).getByText('Shadowrocket/2.2.65')).toBeInTheDocument()
    expect(within(rows[0]!).getByText('shadowrocket')).toBeInTheDocument()
    expect(within(rows[2]!).getByText('未知客户端')).toBeInTheDocument()
    expect(screen.getByText('近 24 小时有 3 个不同 IP 获取过订阅。')).toBeInTheDocument()
    // Telegram is available but not bound → link to bind it.
    expect(screen.getByRole('link', { name: '绑定 Telegram 接收提醒' })).toHaveAttribute('href', '/profile')
  })

  it('rotates only the token (separate from the full reset)', async () => {
    const fetchMock = api()
    renderAt('/subscription')
    const user = userEvent.setup()
    expect(await screen.findByLabelText('订阅地址', undefined, T)).toHaveValue(BASE_URL)
    expect(screen.getByRole('button', { name: '重置订阅' })).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: '轮换订阅链接' }))
    const confirm = await screen.findByRole('alertdialog', { name: '轮换订阅链接？' })
    await user.click(within(confirm).getByRole('button', { name: '轮换' }))
    await waitFor(() =>
      expect(screen.getByLabelText('订阅地址')).toHaveValue('https://panel.example.com/s/rotated'),
    )
    expect(bodiesOf(fetchMock, 'POST /user/subscription/rotate-token')).toHaveLength(1)
    expect(bodiesOf(fetchMock, 'POST /user/subscription/reset')).toHaveLength(0)
  })

  it('age key: generate, enable, reveal and copy the private key, regenerate with confirmation', async () => {
    let current: { n: number; enabled: boolean } | null = null
    const fetchMock = api({
      'GET /user/subscription/age': () => view(current),
      // A new key starts disabled; regenerate keeps the switch.
      'POST /user/subscription/age': ({ body }: { body: { regenerate?: boolean } }) => {
        current = {
          n: body.regenerate ? 2 : 1,
          enabled: body.regenerate ? (current?.enabled ?? false) : false,
        }
        return view(current)
      },
      'PATCH /user/subscription/age': ({ body }: { body: { enabled: boolean } }) => {
        current = { ...current!, enabled: body.enabled }
        return view(current)
      },
    })
    renderAt('/subscription')
    const user = userEvent.setup()
    const card = (await screen.findByRole('heading', { name: /订阅加密/ }, T)).closest('section')!
    expect(await within(card).findByText('加密的格式：Clash Meta (mihomo)')).toBeInTheDocument()
    expect(within(card).getByText('需要明文时，可在 Clash Meta 链接后追加 &age=0。')).toBeInTheDocument()

    await user.click(await within(card).findByRole('button', { name: '生成密钥' }, T))
    expect(await within(card).findByText(PUB(1))).toBeInTheDocument()
    expect(bodiesOf(fetchMock, 'POST /user/subscription/age')).toEqual([{}])

    // Hidden by default; reveal shows the full identity.
    const secret = within(card).getByTestId('age-private-key')
    expect(secret).not.toHaveTextContent(PRIV(1))
    expect(secret).toHaveTextContent(/^AGE-SECRET-KEY-1•+$/)
    await user.click(within(card).getByRole('button', { name: '显示私钥' }))
    expect(secret).toHaveTextContent(PRIV(1))
    await user.click(within(card).getByRole('button', { name: '隐藏私钥' }))
    expect(secret).not.toHaveTextContent(PRIV(1))

    await user.click(within(card).getByRole('button', { name: '复制私钥' }))
    await waitFor(async () => expect(await navigator.clipboard.readText()).toBe(PRIV(1)))

    await user.click(within(card).getByRole('switch', { name: '加密订阅内容' }))
    await waitFor(() =>
      expect(bodiesOf(fetchMock, 'PATCH /user/subscription/age')).toEqual([{ enabled: true }]),
    )
    expect(await within(card).findByText('已开启')).toBeInTheDocument()

    await user.click(within(card).getByRole('button', { name: '重新生成' }))
    const confirm = await screen.findByRole('alertdialog', { name: '重新生成 age 密钥？' })
    await user.click(within(confirm).getByRole('button', { name: '重新生成' }))
    expect(await within(card).findByText(PUB(2))).toBeInTheDocument()
    expect(bodiesOf(fetchMock, 'POST /user/subscription/age')).toEqual([{}, { regenerate: true }])
    expect(within(card).getByRole('switch', { name: '加密订阅内容' })).toBeChecked()
  })

  it('strict mode warning, a lost private key and the proxy-providers example', async () => {
    api({
      'GET /user/subscription/age': view(
        { n: 3, enabled: true },
        { private_key: null, strict: true, optional: false },
      ),
    })
    renderAt('/subscription')
    const card = (await screen.findByRole('heading', { name: /订阅加密/ }, T)).closest('section')!
    expect(await within(card).findByText(/私钥已无法显示/, undefined, T)).toBeInTheDocument()
    expect(within(card).queryByRole('button', { name: '显示私钥' })).not.toBeInTheDocument()
    expect(within(card).getByText(/站点启用了严格模式/)).toBeInTheDocument()
    expect(within(card).queryByText(/&age=0/)).not.toBeInTheDocument()
    expect(within(card).getByLabelText('proxy-providers 示例')).toHaveTextContent(`url: "${BASE_URL}"`)
  })
})
