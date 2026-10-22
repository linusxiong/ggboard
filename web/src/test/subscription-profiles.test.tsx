/**
 * FE-S: subscription profiles (switcher CRUD) and node operations on `/subscription`, against a stateful
 * fetch mock shaped like the contract (GET/POST /user/rule-profiles, GET/PUT/DELETE /user/rule-profiles/:slug,
 * POST …/:slug/default, the /user/rule-profile alias, GET /user/nodes).
 */
import { readFileSync } from 'node:fs'
import path from 'node:path'
import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { emptyNodeOps } from '@/features/subscription/nodeOps'
import { applyTemplate, findTemplate } from '@/features/subscription/ruleProfile'
import type { RuleProfile, RuleProfileSummary, RulesetCatalog, UserNode } from '@/lib/api/types'
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
const base = applyTemplate(findTemplate(catalog, 'standard')!, { source: 'blackmatrix7' })
const BASE_URL = 'https://panel.example.com/s/tok'
const NOT_FOUND = () => json({ code: 'not_found', message: 'not found' }, 404)

const node = (id: number, name: string, type: UserNode['type']): UserNode => ({
  id,
  name,
  type,
  tags: [],
  rate: 1,
  is_online: true,
  sort: id,
})
const NODES = [
  node(1, '香港 01', 'vless'),
  node(2, '香港 02 IPLC', 'trojan'),
  node(3, '日本 东京', 'hysteria'),
  node(4, '美国 洛杉矶', 'vless'),
]

/** In-memory server following the contract. */
function server(opts: { legacy?: boolean } = {}) {
  const store = new Map<string, RuleProfile>([
    ['default', { ...base, nodes: emptyNodeOps() }],
    ['work', { ...base, template: 'minimal', nodes: { ...emptyNodeOps(), prefix: 'W ' } }],
  ])
  let list: RuleProfileSummary[] = [
    { id: 1, slug: 'default', name: '默认', is_default: true, updated_at: '2026-10-22T00:00:00Z' },
    { id: 2, slug: 'work', name: 'Work', is_default: false, updated_at: '2026-10-22T00:00:00Z' },
  ]
  const defaultSlug = () => list.find((p) => p.is_default)!.slug
  const view = (slug: string) => {
    const doc = store.get(slug)
    const meta = list.find((p) => p.slug === slug)
    return doc && meta
      ? { ...doc, id: meta.id, slug, name: meta.name, is_default: meta.is_default }
      : undefined
  }
  const slugRoutes: Record<string, unknown> = {}
  for (const slug of ['default', 'work', 'travel', 'copy']) {
    // GET answers the view (document + id/slug/name/is_default), like the backend.
    slugRoutes[`GET /user/rule-profiles/${slug}`] = () => view(slug) ?? NOT_FOUND()
    slugRoutes[`PUT /user/rule-profiles/${slug}`] = ({ body }: { body: RuleProfile }) => {
      // The backend rejects unknown fields: a rename must never ride along with the profile document.
      if ('name' in body || 'slug' in body)
        return json({ code: 'validation_failed', message: 'unknown field' }, 400)
      store.set(slug, body)
      return body
    }
    slugRoutes[`PATCH /user/rule-profiles/${slug}`] = ({ body }: { body: { name: string } }) => {
      list = list.map((p) => (p.slug === slug ? { ...p, name: body.name } : p))
      return list.find((p) => p.slug === slug)
    }
    slugRoutes[`DELETE /user/rule-profiles/${slug}`] = () => {
      list = list.filter((p) => p.slug !== slug)
      return true
    }
    slugRoutes[`POST /user/rule-profiles/${slug}/default`] = () => {
      list = list.map((p) => ({ ...p, is_default: p.slug === slug }))
      return true
    }
  }
  const legacyProfile = { ...base }
  return mockApi({
    'GET /guest/config': GUEST_CONFIG,
    'GET /user/me': TEST_USER,
    'GET /user/rulesets/catalog': catalog,
    'GET /user/nodes': NODES,
    'GET /user/subscription': () => ({
      subscribe_url: BASE_URL,
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
      ...(opts.legacy
        ? {}
        : {
            profiles: list.map((p) => ({
              slug: p.slug,
              name: p.name,
              subscribe_url: p.is_default ? BASE_URL : `${BASE_URL}?profile=${p.slug}`,
            })),
          }),
    }),
    'GET /user/subscription/age': {
      enabled: false,
      public_key: null,
      private_key: null,
      created_at: null,
      formats: ['clash-meta'],
      optional: true,
      strict: false,
    },
    'GET /user/subscription/access-log': [],
    'GET /user/telegram': NOT_FOUND,
    ...(opts.legacy
      ? {
          'GET /user/rule-profile': legacyProfile,
          'PUT /user/rule-profile': ({ body }: { body: unknown }) => body,
        }
      : {
          'GET /user/rule-profile': () => view(defaultSlug()),
          'PUT /user/rule-profile': ({ body }: { body: RuleProfile }) => {
            if ('name' in body || 'slug' in body)
              return json({ code: 'validation_failed', message: 'unknown field' }, 400)
            store.set(defaultSlug(), body)
            return view(defaultSlug())
          },
          'GET /user/rule-profiles': () => list,
          'POST /user/rule-profiles': ({
            body,
          }: {
            body: { name: string; slug?: string; copy_from?: string }
          }) => {
            const created = {
              id: list.length + 10,
              slug: body.slug ?? 'generated',
              name: body.name,
              is_default: false,
              updated_at: '2026-10-22T00:00:00Z',
            }
            store.set(created.slug, { ...(store.get(body.copy_from ?? '') ?? base), nodes: emptyNodeOps() })
            list = [...list, created]
            return created
          },
          ...slugRoutes,
        }),
  })
}

const T = { timeout: 8000 }

// jsdom has no Web Animations API; React Aria's tab indicator (SharedElementTransition) calls it on switch.
if (!('getAnimations' in Element.prototype))
  Object.defineProperty(Element.prototype, 'getAnimations', { configurable: true, value: () => [] })

describe('subscription profiles', () => {
  beforeEach(() => {
    resetStores()
    signIn()
  })
  afterEach(() => {
    queryClient.clear()
    vi.unstubAllGlobals()
  })

  it('switches profiles, creates, renames and duplicates them', async () => {
    const fetchMock = server()
    const { router } = renderAt('/subscription')
    const user = userEvent.setup()

    const work = await screen.findByRole('tab', { name: 'Work' }, T)
    expect(screen.getByRole('tab', { name: /默认/ })).toHaveAttribute('aria-selected', 'true')
    expect(screen.getByLabelText('订阅地址')).toHaveValue(BASE_URL)

    // Switch: own link (from GET /user/subscription → profiles) + its stored rules.
    await user.click(work)
    await waitFor(() => expect(router.state.location.search).toEqual({ profile: 'work' }))
    await waitFor(() => expect(screen.getByLabelText('订阅地址')).toHaveValue(`${BASE_URL}?profile=work`))
    expect(screen.getByRole('link', { name: /导入到 Clash/ }).getAttribute('href')).toContain(
      encodeURIComponent('GG Bond · Work'),
    )

    // Create: slug suggested from the name.
    await user.click(screen.getByRole('button', { name: '新建配置' }))
    let dialog = await screen.findByRole('dialog', { name: '新建订阅配置' })
    await user.type(within(dialog).getByLabelText(/名称/), 'Travel')
    expect(within(dialog).getByLabelText('链接标识')).toHaveAttribute('placeholder', 'travel')
    await user.click(within(dialog).getByRole('button', { name: '创建' }))
    await waitFor(() =>
      expect(bodiesOf(fetchMock, 'POST /user/rule-profiles')).toEqual([{ name: 'Travel', slug: 'travel' }]),
    )
    expect(await screen.findByRole('tab', { name: 'Travel', selected: true }, T)).toBeInTheDocument()

    // Rename the active profile: PATCH {name}.
    await user.click(screen.getByRole('button', { name: '「Travel」的更多操作' }))
    await user.click(await screen.findByRole('menuitem', { name: '重命名' }))
    dialog = await screen.findByRole('dialog', { name: '重命名「Travel」' })
    const name = within(dialog).getByLabelText(/名称/)
    await user.clear(name)
    await user.type(name, 'Trips')
    await user.click(within(dialog).getByRole('button', { name: '保存' }))
    await waitFor(() =>
      expect(bodiesOf(fetchMock, 'PATCH /user/rule-profiles/travel')).toEqual([{ name: 'Trips' }]),
    )
    expect(bodiesOf(fetchMock, 'PUT /user/rule-profiles/travel')).toHaveLength(0)
    expect(await screen.findByRole('tab', { name: 'Trips' }, T)).toBeInTheDocument()

    // Duplicate → copy_from.
    await user.click(screen.getByRole('button', { name: '「Trips」的更多操作' }))
    await user.click(await screen.findByRole('menuitem', { name: '复制' }))
    dialog = await screen.findByRole('dialog', { name: '复制「Trips」' })
    await user.type(within(dialog).getByLabelText(/名称/), 'Copy')
    await user.click(within(dialog).getByRole('button', { name: '创建' }))
    await waitFor(() =>
      expect(bodiesOf(fetchMock, 'POST /user/rule-profiles')[1]).toEqual({
        name: 'Copy',
        slug: 'copy',
        copy_from: 'travel',
      }),
    )
  })

  it('sets a default and deletes a profile (the default cannot be deleted)', async () => {
    const fetchMock = server()
    const { router } = renderAt('/subscription?profile=work')
    const user = userEvent.setup()
    expect(await screen.findByRole('tab', { name: 'Work', selected: true }, T)).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '「Work」的更多操作' }))
    await user.click(await screen.findByRole('menuitem', { name: '设为默认' }))
    const confirmDefault = await screen.findByRole('alertdialog', { name: '将「Work」设为默认？' })
    await user.click(within(confirmDefault).getByRole('button', { name: '设为默认' }))
    await waitFor(() => expect(bodiesOf(fetchMock, 'POST /user/rule-profiles/work/default')).toHaveLength(1))
    // The new default comes first and owns the plain link.
    await waitFor(() => expect(screen.getAllByRole('tab')[0]).toHaveAccessibleName(/Work/))
    await waitFor(() => expect(screen.getByLabelText('订阅地址')).toHaveValue(BASE_URL))

    // The old default is now deletable.
    await user.click(screen.getByRole('tab', { name: '默认' }))
    await user.click(screen.getByRole('button', { name: '「默认」的更多操作' }))
    const remove = await screen.findByRole('menuitem', { name: '删除' })
    expect(remove).not.toHaveAttribute('aria-disabled', 'true')
    await user.click(remove)
    const confirmDelete = await screen.findByRole('alertdialog', { name: '删除「默认」？' })
    await user.click(within(confirmDelete).getByRole('button', { name: '删除' }))
    await waitFor(() => expect(bodiesOf(fetchMock, 'DELETE /user/rule-profiles/default')).toHaveLength(1))
    await waitFor(() => expect(screen.queryByRole('tab', { name: '默认' })).not.toBeInTheDocument())
    expect(router.state.location.search).toEqual({})

    // The (new) default profile's menu has delete disabled.
    await user.click(screen.getByRole('button', { name: '「Work」的更多操作' }))
    expect(await screen.findByRole('menuitem', { name: '删除' })).toHaveAttribute('aria-disabled', 'true')
  })

  it('node operations: chips, regex lists and rename rules → `nodes` payload, with a live preview', async () => {
    const fetchMock = server()
    renderAt('/subscription')
    const user = userEvent.setup()
    await user.click(await screen.findByRole('tab', { name: '节点处理' }, T))
    const count = await screen.findByTestId('nodeops-count', undefined, T)
    expect(count).toHaveTextContent('保留 4 / 4 个节点')

    await user.click(screen.getByRole('button', { name: 'HK · 2 个节点' }))
    await user.click(screen.getByRole('button', { name: 'JP · 1 个节点' }))
    expect(count).toHaveTextContent('保留 3 / 4 个节点')

    const exclude = screen.getByRole('textbox', { name: '添加到排除（正则）' })
    await user.type(exclude, 'foo(?=bar)')
    await user.keyboard('{Enter}')
    expect(await screen.findByText('不支持这种写法（RE2 不支持环视和反向引用）')).toBeInTheDocument()
    await user.clear(exclude)
    await user.type(exclude, 'IPLC')
    expect(screen.getByText('匹配 1 个节点')).toBeInTheDocument()
    await user.keyboard('{Enter}')
    expect(count).toHaveTextContent('保留 2 / 4 个节点')

    await user.click(screen.getByRole('button', { name: '添加规则' }))
    await user.type(screen.getByRole('textbox', { name: '匹配' }), '^(\\S+) (\\S+)$')
    await user.type(screen.getByRole('textbox', { name: '替换为' }), '$2 $1')
    await user.click(screen.getByRole('radio', { name: '自动添加' }))
    await user.type(screen.getByRole('textbox', { name: '后缀' }), ' ✈')
    const preview = screen.getByRole('list', { name: '处理后的节点' })
    expect(within(preview).getByText('🇭🇰 01 香港 ✈')).toBeInTheDocument()
    expect(within(preview).getByText('🇯🇵 东京 日本 ✈')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: '保存节点处理' }))
    await waitFor(() => expect(bodiesOf(fetchMock, 'PUT /user/rule-profile')).toHaveLength(1))
    const sent = bodiesOf(fetchMock, 'PUT /user/rule-profile')[0] as RuleProfile
    expect(sent.nodes).toEqual({
      ...emptyNodeOps(),
      regions: ['HK', 'JP'],
      exclude: ['IPLC'],
      rename: [{ match: '^(\\S+) (\\S+)$', replace: '$2 $1' }],
      emoji: 'add',
      suffix: ' ✈',
    })
    // The rule part of the profile is sent unchanged.
    expect(sent.template).toBe('standard')
    expect(sent.groups.map((g) => g.name)).toEqual(base.groups.map((g) => g.name))
  })

  it('saving rules keeps the stored node operations of a non-default profile', async () => {
    const fetchMock = server()
    renderAt('/subscription?profile=work')
    const user = userEvent.setup()
    const twitter = await screen.findByRole('switch', { name: /X \/ Twitter/ }, T)
    await user.click(twitter)
    await user.click(screen.getByRole('button', { name: '保存规则' }))
    await waitFor(() => expect(bodiesOf(fetchMock, 'PUT /user/rule-profiles/work')).toHaveLength(1))
    const sent = bodiesOf(fetchMock, 'PUT /user/rule-profiles/work')[0] as RuleProfile
    expect(sent.template).toBe('minimal')
    expect(sent.categories).toHaveProperty('twitter')
    expect(sent.nodes).toEqual({ ...emptyNodeOps(), prefix: 'W ' })
    expect(bodiesOf(fetchMock, 'PUT /user/rule-profile')).toHaveLength(0)
  })

  it('servers without profiles: no switcher, no `nodes` in the payload', async () => {
    const fetchMock = server({ legacy: true })
    renderAt('/subscription')
    const user = userEvent.setup()
    const twitter = await screen.findByRole('switch', { name: /X \/ Twitter/ }, T)
    expect(screen.queryByRole('button', { name: '新建配置' })).not.toBeInTheDocument()
    expect(screen.queryByRole('tablist', { name: '订阅配置' })).not.toBeInTheDocument()
    await user.click(twitter)
    await user.click(screen.getByRole('button', { name: '保存规则' }))
    await waitFor(() => expect(bodiesOf(fetchMock, 'PUT /user/rule-profile')).toHaveLength(1))
    expect(bodiesOf(fetchMock, 'PUT /user/rule-profile')[0]).not.toHaveProperty('nodes')
    await user.click(screen.getByRole('tab', { name: '节点处理' }))
    expect(await screen.findByText('当前服务器版本还不支持节点处理。')).toBeInTheDocument()
  })
})
