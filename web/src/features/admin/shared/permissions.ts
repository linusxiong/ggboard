import { isAdmin, useAuthStore } from '@/stores/auth'

/** `true` for admins; staff (`is_staff` only) get read-only admin views. */
export function useIsAdmin(): boolean {
  return useAuthStore((s) => isAdmin(s.user))
}
