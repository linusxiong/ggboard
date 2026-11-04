import { QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import type { Node, NodeRoute } from '@/lib/api/types'
import { createQueryClient } from '@/lib/queryClient'
import { useUiStore } from '@/stores/ui'
import { RoutePushDialog } from './RoutePushDialog'
import { nodeMatches, pushTargets } from './routePush'

const route: NodeRoute = {
  id: 7,
  remarks: '屏蔽广告',
  match: ['ads.example.com'],
  action: 'block',
  action_value: '',
  created_at: '2026-10-01T00:00:00Z',
  updated_at: '2026-10-01T00:00:00Z',
}

function node(id: number, name: string, extra: Partial<Node> = {}): Node {
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
    show: true,
    sort: id,
    protocol_settings: {},
    status: { online: 0, last_check_at: null, last_push_at: null, load: null },
    token: '',
    created_at: '2026-10-01T00:00:00Z',
    updated_at: '2026-10-01T00:00:00Z',
    ...extra,
  }
}

const bodies: unknown[] = []
const onOpenChange = vi.fn()

function renderDialog(nodes: Node[]) {
  render(
    <QueryClientProvider client={createQueryClient()}>
      <RoutePushDialog route={route} nodes={nodes} isOpen onOpenChange={onOpenChange} />
    </QueryClientProvider>,
  )
}

const pressed = (name: string) => screen.getByRole('button', { name }).getAttribute('aria-pressed')

describe('route push dialog', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    await loadNamespaces(['admin-nodes', 'common'])
  })
  beforeEach(() => {
    bodies.length = 0
    onOpenChange.mockClear()
    vi.stubGlobal(
      'fetch',
      vi.fn((_input: RequestInfo | URL, init?: RequestInit) => {
        const body = JSON.parse(init?.body as string) as { all?: boolean; node_ids?: number[] }
        bodies.push(body)
        const node_count = body.all ? 2 : (body.node_ids?.length ?? 0)
        return Promise.resolve(
          new Response(JSON.stringify({ data: { node_count, added: node_count, removed: 0 } }), {
            status: 200,
            headers: { 'Content-Type': 'application/json' },
          }),
        )
      }),
    )
  })
  afterEach(() => vi.unstubAllGlobals())

  it('sends a fresh route to all nodes in one press, not counting child nodes', async () => {
    const user = userEvent.setup()
    renderDialog([node(1, 'HK01'), node(2, 'JP02'), node(3, 'HK01 relay', { parent_id: 1 })])

    await user.click(await screen.findByRole('button', { name: '下发到 2 个节点' }))

    await waitFor(() => expect(onOpenChange).toHaveBeenCalledWith(false))
    expect(bodies).toEqual([{ all: true }])
  })

  it('opens on the current nodes of a partly pushed route and warns before withdrawing', async () => {
    const user = userEvent.setup()
    renderDialog([node(1, 'HK01', { route_ids: [7] }), node(2, 'JP02'), node(3, 'SG03')])

    expect(await screen.findByRole('button', { name: 'HK01' })).toHaveAttribute('aria-pressed', 'true')
    expect(pressed('JP02')).toBe('false')
    expect(screen.queryByRole('status')).toBeNull()

    await user.click(screen.getByRole('button', { name: 'JP02' }))
    await user.click(screen.getByRole('button', { name: 'SG03' }))
    await user.click(screen.getByRole('button', { name: 'HK01' }))
    expect(screen.getByRole('status')).toHaveTextContent('将从 1 个节点撤回这条规则')

    await user.click(screen.getByRole('button', { name: '下发到 2 个节点' }))
    await waitFor(() => expect(bodies).toEqual([{ node_ids: [2, 3] }]))
  })

  it('withdraws from every node when nothing is left selected', async () => {
    const user = userEvent.setup()
    renderDialog([node(1, 'HK01', { route_ids: [7] }), node(2, 'JP02')])

    await user.click(await screen.findByRole('button', { name: '清空' }))
    await user.click(screen.getByRole('button', { name: '从 1 个节点撤回' }))

    await waitFor(() => expect(bodies).toEqual([{ node_ids: [] }]))
  })

  it('narrows the chips by name or tag; select all only touches the matches', async () => {
    const user = userEvent.setup()
    renderDialog([
      node(1, 'HK01', { route_ids: [7] }),
      node(2, 'JP02', { tags: ['Premium'] }),
      node(3, 'JP03', { tags: ['premium'] }),
    ])

    await user.type(await screen.findByRole('searchbox', { name: '搜索节点' }), 'PREM')
    expect(screen.queryByRole('button', { name: 'HK01' })).toBeNull()
    await user.click(screen.getByRole('button', { name: '全选' }))

    await user.click(screen.getByRole('button', { name: '下发到 3 个节点' }))
    await waitFor(() => expect(bodies).toEqual([{ node_ids: [1, 2, 3] }]))
  })

  it('cannot submit without any node', async () => {
    renderDialog([])
    expect(await screen.findByText('还没有节点')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '下发到 0 个节点' })).toBeDisabled()
  })
})

describe('route push helpers', () => {
  it('leaves child nodes out of the targets', () => {
    expect(pushTargets([node(1, 'a'), node(2, 'b', { parent_id: 1 })]).map((n) => n.id)).toEqual([1])
  })

  it('matches name, host and tags', () => {
    const n = node(5, 'Tokyo', { host: 'edge.example.com', tags: ['日本'] })
    expect(nodeMatches(n, ' tok ')).toBe(true)
    expect(nodeMatches(n, 'EDGE')).toBe(true)
    expect(nodeMatches(n, '日本')).toBe(true)
    expect(nodeMatches(n, 'seoul')).toBe(false)
    expect(nodeMatches(n, '')).toBe(true)
  })
})
