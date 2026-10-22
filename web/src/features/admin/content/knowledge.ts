import type { AdminKnowledge } from '@/lib/api/types'

/** Placeholders the user-facing knowledge endpoint substitutes (docs/API.md). */
export const KNOWLEDGE_PLACEHOLDERS = [
  '{{siteName}}',
  '{{subscribeUrl}}',
  '{{urlEncodeSubscribeUrl}}',
  '{{safeBase64SubscribeUrl}}',
] as const
export const ACCESS_START = '<!--access start-->'
export const ACCESS_END = '<!--access end-->'
export const LANGUAGE_SUGGESTIONS = ['zh-CN', 'en'] as const

export interface KnowledgeFilters {
  language?: string
  category?: string
  q?: string
}

/** Client-side filtering of the (unpaginated) admin list. */
export function filterKnowledge(items: readonly AdminKnowledge[], f: KnowledgeFilters): AdminKnowledge[] {
  const q = f.q?.trim().toLowerCase()
  return items.filter(
    (k) =>
      (!f.language || k.language === f.language) &&
      (!f.category || k.category === f.category) &&
      (!q || k.title.toLowerCase().includes(q) || k.category.toLowerCase().includes(q)),
  )
}
