import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import type { AdminLogEntry } from '@/lib/api/types'
import { useUiStore } from '@/stores/ui'
import { buildDiagnostics } from './diagnostics'
import { formatDuration, toSeconds } from './format'
import { filterLogs, logsQuery } from './logs'
import { LogsViewer } from './LogsViewer'

const entries: AdminLogEntry[] = [
  { time: '2026-10-22T01:00:00Z', level: 'INFO', msg: 'server started', attrs: { addr: ':8080' } },
  {
    time: '2026-10-22T01:00:05Z',
    level: 'WARN',
    msg: 'payment notify verification failed',
    attrs: { gateway: 'epay' },
  },
  { time: '2026-10-22T01:00:09Z', level: 'ERROR', msg: 'mail send failed', attrs: null },
]

const urlOf = (input: RequestInfo | URL) => (input instanceof Request ? input.url : input.toString())

describe('system helpers', () => {
  it('parses Go durations and formats uptime', () => {
    expect(toSeconds(90)).toBe(90)
    expect(toSeconds('72h3m0.5s')).toBeCloseTo(259380.5)
    expect(toSeconds('1m30s')).toBe(90)
    expect(toSeconds('soon')).toBeNull()
    expect(formatDuration(93784, 'en')).toMatch(/1\s?d.*2\s?h.*3\s?m/)
  })

  it('builds the logs query and filters by text', () => {
    expect(logsQuery('all', 200)).toEqual({ limit: 200 })
    expect(logsQuery('warn', 500)).toEqual({ level: 'warn', limit: 500 })
    expect(filterLogs(entries, 'EPAY').map((e) => e.msg)).toEqual(['payment notify verification failed'])
    expect(filterLogs(entries, undefined)).toHaveLength(3)
  })

  it('builds diagnostics without secrets', () => {
    const text = buildDiagnostics(
      {
        version: '0.1.0',
        go_version: 'go1.27',
        uptime: '1h0m0s',
        db_driver: 'sqlite',
        started_at: '2026-10-22T00:00:00Z',
        scheduler: [{ name: 'order_timeout', spec: '*/1 * * * *', last_run: null, last_error: '' }],
      },
      entries.slice(1),
    )
    const parsed = JSON.parse(text) as { panel: { uptime_seconds: number }; recent_problems: unknown[] }
    expect(parsed.panel.uptime_seconds).toBe(3600)
    expect(parsed.recent_problems).toHaveLength(2)
  })
})

describe('LogsViewer', () => {
  const fetchMock = vi.fn<typeof fetch>()
  beforeAll(async () => {
    useUiStore.setState({ locale: 'en' })
    await loadNamespaces(['admin-system', 'admin-settings', 'common', 'core'], 'en')
  })
  beforeEach(() => {
    fetchMock.mockImplementation((input) => {
      const url = new URL(urlOf(input), 'http://localhost')
      const level = url.searchParams.get('level')
      const data = level === 'warn' ? entries.filter((e) => e.level !== 'INFO') : entries
      return Promise.resolve(
        new Response(JSON.stringify({ data }), { headers: { 'Content-Type': 'application/json' } }),
      )
    })
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => vi.unstubAllGlobals())

  it('filters by level (server) and text (client), and expands attributes', async () => {
    const user = userEvent.setup()
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={qc}>
        <LogsViewer />
      </QueryClientProvider>,
    )
    const list = await screen.findByRole('list', { name: 'Logs' })
    expect(within(list).getAllByRole('listitem')).toHaveLength(3)
    // newest first
    expect(within(list).getAllByRole('listitem')[0]).toHaveTextContent('mail send failed')
    expect(urlOf(fetchMock.mock.calls[0]![0])).toContain('limit=200')
    expect(urlOf(fetchMock.mock.calls[0]![0])).not.toContain('level=')

    await user.click(screen.getByRole('radio', { name: 'Warn' }))
    await waitFor(() => expect(urlOf(fetchMock.mock.lastCall![0])).toContain('level=warn'))
    await waitFor(() =>
      expect(within(screen.getByRole('list', { name: 'Logs' })).getAllByRole('listitem')).toHaveLength(2),
    )

    await user.type(screen.getByRole('searchbox', { name: 'Search logs' }), 'notify')
    await waitFor(() =>
      expect(within(screen.getByRole('list', { name: 'Logs' })).getAllByRole('listitem')).toHaveLength(1),
    )

    const toggle = screen.getByRole('button', { name: '1 attrs' })
    expect(toggle).toHaveAttribute('aria-expanded', 'false')
    await user.click(toggle)
    expect(toggle).toHaveAttribute('aria-expanded', 'true')
    expect(screen.getByText(/"gateway": "epay"/)).toBeInTheDocument()
  })
})
