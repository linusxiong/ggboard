import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { act, renderHook } from '@testing-library/react'
import type { ReactNode } from 'react'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { qk } from '@/lib/api/queries'
import { callsTo, mockApi } from '@/features/shop/testing'
import { POLL_INTERVAL_MS, POLL_WINDOW_MS, useOrderPolling } from './useOrderPolling'

function setup(statuses: number[]) {
  let i = 0
  const fetchMock = mockApi({
    'GET /user/orders/T1/status': () => ({ status: statuses[Math.min(i++, statuses.length - 1)] }),
  })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const invalidate = vi.spyOn(client, 'invalidateQueries')
  const wrapper = ({ children }: { children: ReactNode }) => (
    <QueryClientProvider client={client}>{children}</QueryClientProvider>
  )
  return { fetchMock, client, invalidate, wrapper }
}

const polls = (fetchMock: ReturnType<typeof mockApi>) =>
  callsTo(fetchMock, 'GET', '/user/orders/T1/status').length

describe('useOrderPolling', () => {
  beforeEach(() => {
    vi.useFakeTimers({ toFake: ['setTimeout', 'clearTimeout', 'setInterval', 'clearInterval', 'Date'] })
  })
  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('polls every 3 s while pending and refreshes the order once it is paid', async () => {
    const { fetchMock, invalidate, wrapper } = setup([0, 0, 3])
    const { result } = renderHook(() => useOrderPolling('T1', true), { wrapper })

    await act(() => vi.advanceTimersByTimeAsync(10))
    expect(polls(fetchMock)).toBe(1)
    expect(result.current.status).toBe(0)

    await act(() => vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS))
    expect(polls(fetchMock)).toBe(2)

    await act(() => vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS))
    expect(polls(fetchMock)).toBe(3)
    expect(result.current.status).toBe(3)
    expect(invalidate).toHaveBeenCalledWith({ queryKey: qk.user.order('T1'), exact: true })

    // Paid → no more polling.
    await act(() => vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 5))
    expect(polls(fetchMock)).toBe(3)
  })

  it('stops after 15 minutes and can be restarted', async () => {
    const { fetchMock, wrapper } = setup([0])
    const { result } = renderHook(() => useOrderPolling('T1', true), { wrapper })

    await act(() => vi.advanceTimersByTimeAsync(POLL_WINDOW_MS + POLL_INTERVAL_MS))
    expect(result.current.timedOut).toBe(true)
    const count = polls(fetchMock)
    expect(count).toBeGreaterThanOrEqual(POLL_WINDOW_MS / POLL_INTERVAL_MS - 1)
    expect(count).toBeLessThanOrEqual(POLL_WINDOW_MS / POLL_INTERVAL_MS + 1)

    await act(() => vi.advanceTimersByTimeAsync(60_000))
    expect(polls(fetchMock)).toBe(count)

    act(() => result.current.restart())
    await act(() => vi.advanceTimersByTimeAsync(10))
    expect(result.current.timedOut).toBe(false)
    expect(polls(fetchMock)).toBeGreaterThan(count)
  })

  it('does not poll orders that are not pending', async () => {
    const { fetchMock, wrapper } = setup([3])
    renderHook(() => useOrderPolling('T1', false), { wrapper })
    await act(() => vi.advanceTimersByTimeAsync(POLL_INTERVAL_MS * 3))
    expect(polls(fetchMock)).toBe(0)
  })
})
