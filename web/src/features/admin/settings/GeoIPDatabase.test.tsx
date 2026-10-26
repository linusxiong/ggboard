import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import type { GeoIPStatus } from '@/lib/api/types'
import { createQueryClient } from '@/lib/queryClient'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import { GeoIPDatabase } from './GeoIPDatabase'
import { sectionsOf } from './layout'

const STATUS: GeoIPStatus = {
  enabled: true,
  loaded: true,
  updating: false,
  version: '20261014',
  records: 1578019,
  size: 27_300_000,
  sha256: 'ab'.repeat(32),
  updated_at: '2026-10-15T05:23:00Z',
  checked_at: '2026-10-26T05:23:00Z',
  last_error: '',
  last_error_at: null,
}

type Reply = { status?: number; body: unknown }

/** Routes answer in order (the last answer repeats); returns the calls made. */
function mockFetch(routes: Record<string, Reply[]>) {
  const calls: string[] = []
  vi.stubGlobal(
    'fetch',
    vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
      const url = new URL(input instanceof Request ? input.url : input.toString(), 'http://localhost')
      const key = `${(init?.method ?? 'GET').toUpperCase()} ${url.pathname.replace(/^\/api\/v1/, '')}`
      calls.push(key)
      const queue = routes[key]
      const reply = queue && (queue.length > 1 ? queue.shift()! : queue[0])
      if (!reply)
        return Promise.resolve(
          new Response(JSON.stringify({ code: 'not_found', message: 'nope' }), { status: 404 }),
        )
      const status = reply.status ?? 200
      return Promise.resolve(
        new Response(JSON.stringify(status < 300 ? { data: reply.body } : reply.body), {
          status,
          headers: { 'Content-Type': 'application/json' },
        }),
      )
    }),
  )
  return calls
}

function renderDatabase(dirty = false) {
  const client = createQueryClient()
  render(
    <QueryClientProvider client={client}>
      <GeoIPDatabase dirty={dirty} />
    </QueryClientProvider>,
  )
  return client
}

describe('settings → IP locations', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    useAuthStore.setState({ token: 'header.eyJleHAiOjQxMDI0NDQ4MDB9.sig', user: null })
    await loadNamespaces(['admin-settings', 'common'])
  })
  afterEach(() => vi.unstubAllGlobals())

  it('puts the three keys in their own section, URLs only while enabled', () => {
    const sections = sectionsOf({
      name: 'subscribe',
      keys: [
        { key: 'geoip_enable', type: 'bool' },
        { key: 'geoip_download_url', type: 'string', url: true },
        { key: 'geoip_version_url', type: 'string', url: true },
      ],
    })
    expect(sections).toHaveLength(1)
    expect(sections[0]).toMatchObject({ id: 'geoip', help: true })
    expect(sections[0]!.rows.map((r) => [r.def.key, r.children.map((c) => c.key)])).toEqual([
      ['geoip_enable', ['geoip_download_url', 'geoip_version_url']],
    ])
  })

  it('shows the database and updates it now', async () => {
    const calls = mockFetch({
      'GET /admin/geoip': [{ body: STATUS }],
      'POST /admin/geoip/update': [
        { body: { ...STATUS, version: '20261021', records: 1581844, updated: true } },
        { status: 502, body: { code: 'geoip_update_failed', message: 'downloaded file rejected: 3 ranges' } },
      ],
    })
    const client = renderDatabase()
    const user = userEvent.setup()
    const card = await screen.findByRole('group', { name: '数据库状态' })
    expect(within(card).getByText('20261014')).toBeInTheDocument()
    expect(within(card).getByText('1,578,019')).toBeInTheDocument()
    expect(within(card).getByText('下载于')).toBeInTheDocument()

    await user.click(within(card).getByRole('button', { name: '立即更新' }))
    expect(await within(card).findByText('20261021')).toBeInTheDocument()
    expect(calls.filter((c) => c === 'POST /admin/geoip/update')).toHaveLength(1)

    // A rejected download: the error is shown and the status is reloaded.
    await user.click(within(card).getByRole('button', { name: '立即更新' }))
    await waitFor(() => expect(calls.filter((c) => c === 'GET /admin/geoip').length).toBeGreaterThan(1))
    client.clear()
  })

  it('polls a download that continues in the background and reports the error', async () => {
    const calls = mockFetch({
      'GET /admin/geoip': [
        { body: STATUS },
        {
          body: { ...STATUS, last_error: 'version check: HTTP 404', last_error_at: '2026-10-26T06:00:00Z' },
        },
      ],
      'POST /admin/geoip/update': [{ status: 202, body: { ...STATUS, updating: true, updated: false } }],
    })
    const client = renderDatabase()
    const user = userEvent.setup()
    const card = await screen.findByRole('group', { name: '数据库状态' })
    await user.click(within(card).getByRole('button', { name: '立即更新' }))
    expect(await within(card).findByRole('status')).toHaveTextContent('正在后台下载')
    expect(within(card).getByRole('button', { name: /立即更新/ })).toBeDisabled()
    expect(
      await within(card).findByText('上次更新失败：version check: HTTP 404', undefined, { timeout: 6000 }),
    ).toBeInTheDocument()
    expect(calls.filter((c) => c === 'GET /admin/geoip').length).toBeGreaterThanOrEqual(2)
    client.clear()
  }, 10_000)

  it('waits for unsaved settings and hides while the feature is off', async () => {
    mockFetch({ 'GET /admin/geoip': [{ body: STATUS }] })
    const client = renderDatabase(true)
    const card = await screen.findByRole('group', { name: '数据库状态' })
    expect(within(card).getByRole('button', { name: '立即更新' })).toBeDisabled()
    expect(within(card).getByText('有未保存的更改，请先保存再更新。')).toBeInTheDocument()
    client.clear()
  })

  it('renders nothing while disabled and says so when there is no database yet', async () => {
    const calls = mockFetch({ 'GET /admin/geoip': [{ body: { ...STATUS, enabled: false } }] })
    let client = renderDatabase()
    await waitFor(() => expect(calls).toContain('GET /admin/geoip'))
    await waitFor(() => expect(document.querySelector('[aria-busy="true"]')).toBeNull())
    expect(screen.queryByRole('group', { name: '数据库状态' })).toBeNull()
    client.clear()

    mockFetch({
      'GET /admin/geoip': [
        { body: { ...STATUS, loaded: false, version: '', records: 0, size: 0, updated_at: null } },
      ],
    })
    client = renderDatabase()
    const card = await screen.findByRole('group', { name: '数据库状态' })
    expect(within(card).getByText('还没有数据库')).toBeInTheDocument()
    client.clear()
  })
})
