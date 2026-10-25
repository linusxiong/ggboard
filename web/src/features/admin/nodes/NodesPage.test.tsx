/**
 * `/admin/nodes` through the real route tree (guards, i18n, React Query) with a mocked `fetch`:
 * the show/hide eye toggle and drag-to-reorder through the row handle.
 */
import { createMemoryHistory, createRouter, RouterProvider } from '@tanstack/react-router'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { Node, User } from '@/lib/api/types'
import { routeTree } from '@/routeTree.gen'
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import config from '../../../../scripts/fixtures/guest-config.json'
import fixtureUser from '../../../../scripts/fixtures/user.json'

const TOKEN = `eyJhbGciOiJub25lIn0.${btoa(JSON.stringify({ sub: 1, exp: 4102444800 })).replace(/=+$/, '')}.test`
const admin = { ...(fixtureUser as User), is_admin: true }

function node(id: number, name: string, show: boolean): Node {
  return {
    id,
    type: 'shadowsocks',
    name,
    group_ids: [],
    route_ids: [],
    parent_id: null,
    host: `n${id}.example.com`,
    port: '443',
    server_port: 443,
    tags: [],
    rate: 1,
    show,
    sort: id,
    protocol_settings: { cipher: 'aes-128-gcm' },
    status: { online: 0, last_check_at: null, last_push_at: null, load: null },
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-01T00:00:00Z',
  }
}

let nodes: Node[] = []
const writes: { method: string; path: string; body: unknown }[] = []

function respond(data: unknown) {
  return new Response(JSON.stringify({ data }), {
    status: 200,
    headers: { 'Content-Type': 'application/json' },
  })
}

/** JSON request body (the API client always sends a string). */
const jsonBody = <T,>(init: RequestInit | undefined): T =>
  (typeof init?.body === 'string' ? JSON.parse(init.body) : null) as T

function route(url: URL, init: RequestInit | undefined): Response {
  const p = url.pathname.replace('/api/v1', '')
  const method = init?.method ?? 'GET'
  if (method !== 'GET') writes.push({ method, path: p, body: jsonBody<unknown>(init) })
  const patch = /^\/admin\/nodes\/(\d+)$/.exec(p)
  if (patch && method === 'PATCH') {
    const body = jsonBody<Partial<Node>>(init)
    nodes = nodes.map((n) => (n.id === Number(patch[1]) ? { ...n, ...body } : n))
    return respond(nodes.find((n) => n.id === Number(patch[1])))
  }
  if (p === '/admin/nodes/sort' && method === 'POST') {
    const { ids } = jsonBody<{ ids: number[] }>(init)
    nodes = ids.map((id, i) => ({ ...nodes.find((n) => n.id === id)!, sort: i + 1 }))
    return respond(true)
  }
  switch (p) {
    case '/guest/config':
      return respond(config)
    case '/user/me':
      return respond(admin)
    case '/admin/nodes':
      return respond(nodes)
    case '/admin/groups':
    case '/admin/routes':
      return respond([])
    default:
      return new Response(JSON.stringify({ code: 'not_found', message: p }), { status: 404 })
  }
}

const fetchMock = vi.fn<typeof fetch>()

function renderAt(url: string) {
  const router = createRouter({ routeTree, history: createMemoryHistory({ initialEntries: [url] }) })
  render(<RouterProvider router={router} />)
}

beforeEach(() => {
  nodes = [node(1, 'HK01', true), node(2, 'JP02', true), node(3, 'SG03', false)]
  writes.length = 0
  useUiStore.setState({ locale: 'zh-CN', theme: 'system' })
  useAuthStore.setState({ token: TOKEN, user: admin })
  fetchMock.mockImplementation((input, init) => {
    const raw = input instanceof Request ? input.url : input.toString()
    return Promise.resolve(route(new URL(raw, 'http://localhost'), init))
  })
  vi.stubGlobal('fetch', fetchMock)
  // Desktop layout (the DataTable renders the HeroUI table at ≥ 768 px).
  vi.stubGlobal(
    'matchMedia',
    vi.fn().mockImplementation((query: string) => ({
      matches: query.includes('min-width'),
      media: query,
      onchange: null,
      addEventListener: vi.fn(),
      removeEventListener: vi.fn(),
      addListener: vi.fn(),
      removeListener: vi.fn(),
      dispatchEvent: vi.fn(),
    })),
  )
})
afterEach(() => vi.unstubAllGlobals())

const nameOrder = () =>
  within(screen.getByRole('grid', { name: '节点' }))
    .getAllByRole('button', { name: /^编辑节点 / })
    .map((b) => b.textContent)

describe('admin nodes page', () => {
  it('shows visibility as an eye toggle and saves it with PATCH {show}', { timeout: 30_000 }, async () => {
    const user = userEvent.setup()
    renderAt('/admin/nodes')
    const hk = await screen.findByRole('button', { name: '对用户显示「HK01」' }, { timeout: 15_000 })
    const sg = screen.getByRole('button', { name: '对用户显示「SG03」' })
    expect(hk).toHaveAttribute('aria-pressed', 'true')
    expect(sg).toHaveAttribute('aria-pressed', 'false')
    // No text chip any more; the hidden node's name is dimmed instead.
    expect(screen.queryByText('已隐藏')).not.toBeInTheDocument()
    expect(screen.getByRole('button', { name: '编辑节点 SG03' })).toHaveClass('text-label-secondary')
    expect(screen.getByRole('button', { name: '编辑节点 HK01' })).toHaveClass('text-label-blue')

    await user.click(hk)
    await waitFor(() => expect(hk).toHaveAttribute('aria-pressed', 'false'))
    expect(writes).toContainEqual({ method: 'PATCH', path: '/admin/nodes/1', body: { show: false } })
    await waitFor(() =>
      expect(screen.getByRole('button', { name: '编辑节点 HK01' })).toHaveClass('text-label-secondary'),
    )

    await user.click(sg)
    await waitFor(() => expect(sg).toHaveAttribute('aria-pressed', 'true'))
    expect(writes).toContainEqual({ method: 'PATCH', path: '/admin/nodes/3', body: { show: true } })

    // The "…" menu no longer duplicates the toggle.
    await user.click(screen.getByRole('button', { name: 'JP02 的更多操作' }))
    expect(await screen.findByRole('menuitem', { name: '复制节点' })).toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: /对用户隐藏|向用户显示/ })).not.toBeInTheDocument()
  })

  it(
    'reorders by keyboard through the drag handle and saves the full order',
    { timeout: 30_000 },
    async () => {
      const user = userEvent.setup()
      renderAt('/admin/nodes')
      const handle = await screen.findByRole('button', { name: '拖动以排序：HK01' }, { timeout: 15_000 })
      expect(handle).toBeEnabled()
      expect(screen.queryByRole('button', { name: /上移|下移/ })).not.toBeInTheDocument()
      expect(nameOrder()).toEqual(['HK01', 'JP02', 'SG03'])

      // Grid keyboard model: focus the first row, → into its first cell, Tab to the handle.
      within(screen.getByRole('grid', { name: '节点' }))
        .getAllByRole('row')[1]
        ?.focus()
      await user.keyboard('{ArrowRight}')
      await user.tab()
      await waitFor(() => expect(handle).toHaveFocus())
      await user.keyboard('{Enter}')
      await waitFor(() => expect(document.activeElement).toHaveAttribute('role', 'button'))
      await user.keyboard('{ArrowDown}{Enter}')

      await waitFor(() =>
        expect(writes).toContainEqual({
          method: 'POST',
          path: '/admin/nodes/sort',
          body: { ids: [2, 1, 3] },
        }),
      )
      await waitFor(() => expect(nameOrder()).toEqual(['JP02', 'HK01', 'SG03']))
    },
  )

  it('disables reordering while a filter is active', { timeout: 30_000 }, async () => {
    renderAt('/admin/nodes?q=0')
    // `?q=0` (a node ID typed into the URL) is parsed as a number: it must filter, not crash.
    expect(
      await screen.findByRole('button', { name: '编辑节点 HK01' }, { timeout: 15_000 }),
    ).toBeInTheDocument()
    const handle = await screen.findByRole('button', { name: '拖动以排序：HK01' }, { timeout: 15_000 })
    expect(handle).toBeDisabled()
    expect(screen.getByRole('button', { name: '拖动以排序：SG03' })).toBeDisabled()
    expect(handle.closest('[role="row"]')).not.toHaveAttribute('draggable', 'true')
  })
})
