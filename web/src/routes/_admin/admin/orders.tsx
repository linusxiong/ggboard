import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useCallback } from 'react'
import { OrdersPage } from '@/features/admin/orders/OrdersPage'
import { type OrderSearch, parseOrderSearch } from '@/features/admin/orders/search'
import { loadNamespaces } from '@/i18n'
import { requireAdmin } from '@/lib/guards'

export const Route = createFileRoute('/_admin/admin/orders')({
  beforeLoad: requireAdmin,
  validateSearch: parseOrderSearch,
  loader: () => loadNamespaces(['admin-orders', 'admin-settings']),
  component: AdminOrdersRoute,
})

function AdminOrdersRoute() {
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const onSearchChange = useCallback(
    (patch: Partial<OrderSearch>) =>
      void navigate({ search: (prev) => parseOrderSearch({ ...prev, ...patch }), replace: true }),
    [navigate],
  )
  return <OrdersPage search={search} onSearchChange={onSearchChange} />
}
