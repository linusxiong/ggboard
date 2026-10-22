import { notFound } from '@tanstack/react-router'
import type { TicketLevel } from '@/lib/api/types'

export const TICKET_LEVELS: readonly TicketLevel[] = [0, 1, 2]
export const LEVEL_KEY = { 0: 'low', 1: 'medium', 2: 'high' } as const

/** `/tickets/$id` param → positive integer id, else the route's 404. */
export function parseTicketId(raw: string): number {
  const id = Number(raw)
  if (!Number.isInteger(id) || id <= 0) throw notFound()
  return id
}
