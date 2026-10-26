import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterAll, afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import { adminApi } from '@/lib/api'
import { useUiStore } from '@/stores/ui'
import { OrderFilters } from './OrderFilters'
import { activeOrderFilters, orderSearchToQuery, splitUserQuery } from './query'
import { parseOrderSearch } from './search'

const urlOf = (input: RequestInfo | URL) => (input instanceof Request ? input.url : input.toString())

/**
 * Runs the rest of the block with the browser (Node) time zone `zone`: Node re-reads `TZ` when it is
 * assigned. Fails loudly if the zone did not take effect, so the date assertions never pass by accident.
 */
function inBrowserZone(zone: string, offsetOn20261026: number) {
  const saved = process.env.TZ
  beforeAll(() => {
    process.env.TZ = zone
    expect(new Date(2026, 9, 26).getTimezoneOffset(), `TZ=${zone} not applied`).toBe(offsetOn20261026)
  })
  afterAll(() => {
    if (saved === undefined) delete process.env.TZ
    else process.env.TZ = saved
  })
}

describe('order search params', () => {
  inBrowserZone('America/New_York', 240)

  it('parses the URL defensively', () => {
    expect(parseOrderSearch({})).toEqual({ page: 1, page_size: 20 })
    expect(
      parseOrderSearch({
        page: '3',
        page_size: 500,
        status: '3',
        commission_status: 9,
        plan_id: 2,
        payment_id: 'x',
        date_from: '2026-10-01',
        date_to: 'yesterday',
        trade_no: '  2026A  ',
        q: '',
      }),
    ).toEqual({ page: 3, page_size: 20, status: 3, plan_id: 2, date_from: '2026-10-01', trade_no: '2026A' })
  })

  it('maps the "email or user id" box and every filter onto API params', () => {
    expect(splitUserQuery('42')).toEqual({ user_id: 42 })
    expect(splitUserQuery('a@example.com')).toEqual({ email: 'a@example.com' })
    expect(splitUserQuery('  ')).toEqual({})
    const search = parseOrderSearch({
      page: 2,
      q: '42',
      status: 0,
      commission_status: 2,
      plan_id: 5,
      payment_id: 7,
      date_from: '2026-10-01',
      date_to: '2026-10-31',
      trade_no: 'T1',
    })
    expect(orderSearchToQuery(search)).toEqual({
      page: 2,
      page_size: 20,
      sort: '-created_at',
      trade_no: 'T1',
      user_id: 42,
      status: 0,
      commission_status: 2,
      plan_id: 5,
      payment_id: 7,
      // New York (EDT): Oct 1 00:00 → 04:00Z; the day after Oct 31 starts Nov 1 00:00 EDT → 04:00Z.
      date_from: '2026-10-01T04:00:00Z',
      date_to: '2026-11-01T04:00:00Z',
    })
    expect(activeOrderFilters(search)).toBe(8)
  })

  it('sends the filters as query parameters', async () => {
    const fetchMock = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify({ data: [], total: 0 }), {
        headers: { 'Content-Type': 'application/json' },
      }),
    )
    vi.stubGlobal('fetch', fetchMock)
    await adminApi.orders(
      orderSearchToQuery(parseOrderSearch({ q: 'a@example.com', status: 3, date_from: '2026-10-01' })),
    )
    const url = new URL(urlOf(fetchMock.mock.calls[0]![0]), 'http://localhost')
    expect(url.pathname).toBe('/api/v1/admin/orders')
    expect(Object.fromEntries(url.searchParams)).toEqual({
      page: '1',
      page_size: '20',
      sort: '-created_at',
      email: 'a@example.com',
      status: '3',
      date_from: '2026-10-01T04:00:00Z',
    })
    vi.unstubAllGlobals()
  })
})

/**
 * A picked day is the browser's calendar day, the one the order list displays `created_at` in, not
 * the server's: the API gets that day's local start and the next day's local start (exclusive).
 */
describe('order date filter bounds', () => {
  const bounds = (from: string, to: string) => {
    const q = orderSearchToQuery(parseOrderSearch({ date_from: from, date_to: to }))
    return [q.date_from, q.date_to]
  }

  describe('admin in New York (UTC−4, server may be in Shanghai)', () => {
    inBrowserZone('America/New_York', 240)
    it('sends the local day as instants', () => {
      // An order created 2026-10-27T02:30Z is shown as Oct 26 22:30 here and falls in [from, to).
      expect(bounds('2026-10-26', '2026-10-26')).toEqual(['2026-10-26T04:00:00Z', '2026-10-27T04:00:00Z'])
    })
    it('covers the 25-hour day when daylight saving time ends', () => {
      expect(bounds('2026-11-01', '2026-11-01')).toEqual(['2026-11-01T04:00:00Z', '2026-11-02T05:00:00Z'])
    })
  })

  describe('admin in Shanghai (UTC+8)', () => {
    inBrowserZone('Asia/Shanghai', -480)
    it('sends the local day as instants', () => {
      expect(bounds('2026-10-26', '2026-10-27')).toEqual(['2026-10-25T16:00:00Z', '2026-10-27T16:00:00Z'])
    })
  })

  it('keeps plain dates in the URL state', () => {
    expect(parseOrderSearch({ date_from: '2026-10-26', date_to: '2026-10-26' })).toMatchObject({
      date_from: '2026-10-26',
      date_to: '2026-10-26',
    })
  })
})

describe('OrderFilters', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'en' })
    await loadNamespaces(['admin-orders', 'admin-settings', 'common', 'core'], 'en')
  })
  afterEach(() => vi.useRealTimers())

  const plans = [
    { id: 1, name: 'Basic' },
    { id: 2, name: 'Pro' },
  ]
  const payments = [{ id: 7, name: 'Alipay' }]

  it('emits filter patches (select immediately, text after a debounce, reset clears all)', async () => {
    const user = userEvent.setup()
    const onChange = vi.fn()
    const { rerender } = render(
      <OrderFilters
        value={{ page: 1, page_size: 20 }}
        onChange={onChange}
        plans={plans}
        payments={payments}
      />,
    )

    // Status is the segmented quick filter (radio group).
    expect(screen.getByRole('radiogroup', { name: 'Status' })).toBeInTheDocument()
    await user.click(screen.getByRole('radio', { name: 'Completed' }))
    expect(onChange).toHaveBeenLastCalledWith({ status: 3 })

    await user.click(screen.getByRole('button', { name: /Plan/ }))
    await user.click(await screen.findByRole('option', { name: 'Pro' }))
    expect(onChange).toHaveBeenLastCalledWith({ plan_id: 2 })

    await user.type(screen.getByRole('searchbox', { name: 'User' }), '42')
    await waitFor(() => expect(onChange).toHaveBeenLastCalledWith({ q: '42' }), { timeout: 2000 })

    const from = screen.getByLabelText('From')
    await act(async () => {
      await user.type(from, '2026-10-01')
    })
    expect(onChange).toHaveBeenCalledWith({ date_from: '2026-10-01' })

    rerender(
      <OrderFilters
        value={{ page: 1, page_size: 20, status: 3, plan_id: 2, q: '42' }}
        onChange={onChange}
        plans={plans}
        payments={payments}
      />,
    )
    await user.click(screen.getByRole('button', { name: 'Reset filters (3)' }))
    expect(onChange).toHaveBeenLastCalledWith(
      expect.objectContaining({ status: undefined, plan_id: undefined, q: undefined, trade_no: undefined }),
    )
  })
})
