import { screen, within } from '@testing-library/react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { isoDay } from '@/features/dashboard/traffic'
import { queryClient } from '@/lib/queryClient'
import { GUEST_CONFIG, mockApi, renderAt, resetStores, signIn, TEST_USER } from './fe1-helpers'

const today = isoDay(new Date())

const SUBSCRIPTION = {
  subscribe_url: 'https://panel.example.com/s/tok',
  token: 'tok',
  plan: { id: 2, name: 'Pro' },
  transfer_enable: 100 * 1024 ** 3,
  u: 10 * 1024 ** 3,
  d: 20 * 1024 ** 3,
  expired_at: null,
  next_reset_at: null,
  reset_day: null,
  device_limit: 3,
  conn_limit: 64,
  speed_limit: 100,
}

function mockDashboard(subscription: Record<string, unknown> = SUBSCRIPTION) {
  return mockApi({
    'GET /guest/config': GUEST_CONFIG,
    'GET /user/me': TEST_USER,
    'GET /user/subscription': subscription,
    'GET /user/stats': {
      pending_orders: 2,
      open_tickets: 1,
      notices_count: 1,
      invite_count: 0,
      commission_pending: 0,
    },
    'GET /user/notices': [
      {
        id: 1,
        title: '维护通知',
        content: '**周五** 02:00 维护',
        img_url: '',
        tags: [],
        created_at: '2026-10-20T00:00:00Z',
      },
    ],
    'GET /user/nodes': [
      {
        id: 1,
        name: '香港 01',
        type: 'vless',
        tags: ['IPLC'],
        rate: 1,
        host: 'hk.example.com',
        port: '443',
        is_online: true,
        sort: 1,
      },
      { id: 2, name: '日本 02', type: 'hysteria', tags: [], rate: 1.5, is_online: false, sort: 2 },
    ],
    'GET /user/traffic-logs': [
      { record_at: today, node_id: 1, node_name: '香港 01', rate: 1, u: 1024 ** 3, d: 2 * 1024 ** 3 },
      { record_at: today, node_id: 9, node_name: null, rate: 1, u: 0, d: 1024 ** 3 },
    ],
  })
}

describe('dashboard', () => {
  beforeEach(() => {
    resetStores()
    signIn()
    mockDashboard()
  })
  afterEach(() => {
    queryClient.clear()
    vi.unstubAllGlobals()
  })

  it('renders plan, stats, notices, nodes and the 7-day traffic chart', async () => {
    renderAt('/dashboard')

    expect(
      await screen.findByRole('heading', { level: 1, name: /demo/ }, { timeout: 8000 }),
    ).toBeInTheDocument()
    const plan = screen.getByRole('heading', { level: 2, name: 'Pro' }).closest('section')!
    expect(within(plan).getByText('长期有效')).toBeInTheDocument()
    expect(within(plan).getByText('100 Mbps')).toBeInTheDocument()
    expect(within(plan).getByText('最多 3 台')).toBeInTheDocument()
    expect(within(plan).getByText('单节点连接数')).toBeInTheDocument()
    expect(within(plan).getByText('最多 64 个')).toBeInTheDocument()
    expect(within(plan).getByRole('meter')).toHaveAttribute('aria-valuenow', '30')

    expect(screen.getByRole('button', { name: /复制订阅链接/ })).toBeInTheDocument()
    expect(await screen.findByText('维护通知')).toBeInTheDocument()
    expect(screen.getByText('香港 01')).toBeInTheDocument()
    expect(screen.getByText('hk.example.com:443')).toBeInTheDocument()
    expect(screen.getByText('离线')).toBeInTheDocument()
    expect(screen.getByText('1 / 2 在线')).toBeInTheDocument()

    // Accessible data table behind the SVG chart: today's row sums both nodes (incl. the deleted one).
    const table = screen.getByRole('table', { name: '近 7 天流量' })
    const rows = within(table).getAllByRole('row')
    expect(rows).toHaveLength(8)
    expect(within(rows[7]!).getByRole('rowheader')).toHaveTextContent(today)
    expect(rows[7]).toHaveTextContent('4 GB')
    expect(screen.getByText('合计 4 GB')).toBeInTheDocument()
  })

  it('hides the connection limit when the plan has none (null or 0)', async () => {
    for (const conn_limit of [null, 0]) {
      queryClient.clear()
      mockDashboard({ ...SUBSCRIPTION, conn_limit })
      const view = renderAt('/dashboard')
      const heading = await screen.findByRole('heading', { level: 2, name: 'Pro' }, { timeout: 8000 })
      const plan = heading.closest('section')!
      expect(within(plan).getByText('最多 3 台')).toBeInTheDocument()
      expect(within(plan).queryByText('单节点连接数')).not.toBeInTheDocument()
      expect(within(plan).queryByText(/最多 \d+ 个/)).not.toBeInTheDocument()
      view.unmount()
    }
  })
})
