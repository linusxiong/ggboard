import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useCallback } from 'react'
import { CouponsPage } from '@/features/admin/coupons/CouponsPage'
import { type CouponSearch, parseCouponSearch } from '@/features/admin/coupons/search'
import { loadNamespaces } from '@/i18n'
import { requireAdmin } from '@/lib/guards'

export const Route = createFileRoute('/_admin/admin/coupons')({
  beforeLoad: requireAdmin,
  validateSearch: parseCouponSearch,
  loader: () => loadNamespaces(['admin-orders', 'admin-settings']),
  component: AdminCouponsRoute,
})

function AdminCouponsRoute() {
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const onSearchChange = useCallback(
    (patch: Partial<CouponSearch>) =>
      void navigate({ search: (prev) => parseCouponSearch({ ...prev, ...patch }), replace: true }),
    [navigate],
  )
  return <CouponsPage search={search} onSearchChange={onSearchChange} />
}
