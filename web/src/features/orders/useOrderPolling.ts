/**
 * Polls `GET /user/orders/:trade_no/status` every 3 s while the order is pending, for at most
 * 15 minutes (then the page offers a manual refresh). When the status leaves "pending" the order
 * and everything a paid order changes (me, subscription, stats, lists) are refetched.
 */
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { useCallback, useEffect, useState } from 'react'
import { userApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import { ORDER_STATUS, type OrderStatus } from '@/lib/api/types'

export const POLL_INTERVAL_MS = 3_000
export const POLL_WINDOW_MS = 15 * 60_000

export interface OrderPolling {
  /** Latest status seen by the poller (`undefined` before the first poll). */
  status: OrderStatus | undefined
  /** Polling stopped because the 15-minute window elapsed. */
  timedOut: boolean
  /** Restart the window and poll now. */
  restart: () => void
}

export function useOrderPolling(tradeNo: string, active: boolean): OrderPolling {
  const queryClient = useQueryClient()
  const [startedAt, setStartedAt] = useState(() => Date.now())
  const [timedOut, setTimedOut] = useState(false)

  const query = useQuery({
    queryKey: [...qk.user.order(tradeNo), 'status'] as const,
    queryFn: ({ signal }) => userApi.orderStatus(tradeNo, signal),
    enabled: active && !timedOut,
    gcTime: 0,
    staleTime: 0,
    retry: false,
    refetchIntervalInBackground: true,
    refetchInterval: (q) => {
      if (q.state.data && q.state.data.status !== ORDER_STATUS.pending) return false
      return Date.now() - startedAt >= POLL_WINDOW_MS ? false : POLL_INTERVAL_MS
    },
  })

  // Stop when the window ends (the interval callback above also stops refetching by itself).
  useEffect(() => {
    if (!active || timedOut) return
    const left = Math.max(0, startedAt + POLL_WINDOW_MS - Date.now())
    const timer = window.setTimeout(() => setTimedOut(true), left)
    return () => window.clearTimeout(timer)
  }, [active, timedOut, startedAt])

  const status = query.data?.status
  useEffect(() => {
    if (status === undefined || status === ORDER_STATUS.pending) return
    void queryClient.invalidateQueries({ queryKey: qk.user.order(tradeNo), exact: true })
    void queryClient.invalidateQueries({ queryKey: qk.user.me() })
    void queryClient.invalidateQueries({ queryKey: qk.user.subscription() })
    void queryClient.invalidateQueries({ queryKey: qk.user.stats() })
    void queryClient.invalidateQueries({ queryKey: ['user', 'orders'] })
  }, [status, tradeNo, queryClient])

  const { refetch } = query
  const restart = useCallback(() => {
    setStartedAt(Date.now())
    setTimedOut(false)
    void refetch()
  }, [refetch])

  return { status, timedOut, restart }
}
