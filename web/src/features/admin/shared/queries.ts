/**
 * Query factories for admin reads that `lib/api/queries.ts` does not cover (rankings, node
 * stats / online users). Keys live under `qk.admin.all` so `invalidateQueries({ queryKey: qk.admin.all })`
 * refreshes them too.
 */
import { keepPreviousData, queryOptions } from '@tanstack/react-query'
import * as adminApi from '@/lib/api/admin'
import type { ID, ISODate, StatsRange } from '@/lib/api/types'

/** Dashboard data refreshes every minute while the tab is visible (Query pauses intervals in background tabs). */
export const DASHBOARD_REFRESH = 60_000

export const adminNodeRankingQuery = (date: ISODate) =>
  queryOptions({
    queryKey: ['admin', 'ranking', 'nodes', date] as const,
    queryFn: ({ signal }) => adminApi.nodeRanking(date, signal),
    placeholderData: keepPreviousData,
    refetchInterval: DASHBOARD_REFRESH,
  })

export const adminUserRankingQuery = (date: ISODate, limit: number) =>
  queryOptions({
    queryKey: ['admin', 'ranking', 'users', date, limit] as const,
    queryFn: ({ signal }) => adminApi.userRanking({ date, limit }, signal),
    placeholderData: keepPreviousData,
    refetchInterval: DASHBOARD_REFRESH,
  })

export const adminNodeStatsQuery = (id: ID, range: StatsRange) =>
  queryOptions({
    queryKey: ['admin', 'node', id, 'stats', range] as const,
    queryFn: ({ signal }) => adminApi.nodeStats(id, range, signal),
    placeholderData: keepPreviousData,
  })

export const adminNodeOnlineQuery = (id: ID) =>
  queryOptions({
    queryKey: ['admin', 'node', id, 'online'] as const,
    queryFn: ({ signal }) => adminApi.nodeOnline(id, signal),
    refetchInterval: DASHBOARD_REFRESH,
  })
