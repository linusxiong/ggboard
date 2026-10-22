import { act, render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import { adminApi } from '@/lib/api'
import { useUiStore } from '@/stores/ui'
import { OrderFilters } from './OrderFilters'
import { activeOrderFilters, orderSearchToQuery, splitUserQuery } from './query'
import { parseOrderSearch } from './search'

const urlOf = (input: RequestInfo | URL) => (input instanceof Request ? input.url : input.toString())

describe('order search params', () => {
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
      date_from: '2026-10-01',
      date_to: '2026-10-31',
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
      date_from: '2026-10-01',
    })
    vi.unstubAllGlobals()
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
