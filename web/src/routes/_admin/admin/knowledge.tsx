import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useCallback } from 'react'
import type { KnowledgeFilters } from '@/features/admin/content/knowledge'
import { KnowledgePage } from '@/features/admin/content/KnowledgePage'
import { compact, optString } from '@/features/admin/orders/search'
import { loadNamespaces } from '@/i18n'

const parseSearch = (raw: Record<string, unknown>): KnowledgeFilters =>
  compact({ language: optString(raw.language), category: optString(raw.category), q: optString(raw.q) })

/** Staff page (no `requireAdmin`). */
export const Route = createFileRoute('/_admin/admin/knowledge')({
  validateSearch: parseSearch,
  loader: () => loadNamespaces(['admin-support', 'admin-settings']),
  component: AdminKnowledgeRoute,
})

function AdminKnowledgeRoute() {
  const filters = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const onFiltersChange = useCallback(
    (patch: KnowledgeFilters) =>
      void navigate({ search: (prev) => parseSearch({ ...prev, ...patch }), replace: true }),
    [navigate],
  )
  return <KnowledgePage filters={filters} onFiltersChange={onFiltersChange} />
}
