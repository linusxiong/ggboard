import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useCallback } from 'react'
import { parseTicketSearch, type TicketSearch } from '@/features/admin/tickets/search'
import { TicketsPage } from '@/features/admin/tickets/TicketsPage'
import { loadNamespaces } from '@/i18n'

/** Staff page (no `requireAdmin`). */
export const Route = createFileRoute('/_admin/admin/tickets')({
  validateSearch: parseTicketSearch,
  loader: () => loadNamespaces(['admin-support']),
  component: AdminTicketsRoute,
})

function AdminTicketsRoute() {
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const onSearchChange = useCallback(
    (patch: Partial<TicketSearch>) =>
      void navigate({
        search: (prev) => parseTicketSearch({ ...prev, ...patch }),
        // Opening a ticket is a navigation step (back returns to the list on phones); filters replace.
        replace: !('id' in patch),
      }),
    [navigate],
  )
  return <TicketsPage search={search} onSearchChange={onSearchChange} />
}
