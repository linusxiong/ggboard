import { createFileRoute, redirect } from '@tanstack/react-router'
import { isAdmin, useAuthStore } from '@/stores/auth'

/** `/admin` → dashboard (admins) or tickets (staff). */
export const Route = createFileRoute('/_admin/admin/')({
  beforeLoad: () => {
    throw redirect({ to: isAdmin(useAuthStore.getState().user) ? '/admin/dashboard' : '/admin/tickets', replace: true })
  },
})
