import { useRouter } from '@tanstack/react-router'
import { useCallback } from 'react'
import type { AuthResult } from '@/lib/api/types'
import { homeFor, safeRedirect } from '@/lib/guards'
import { toast } from '@/lib/toast'
import { useAuthStore } from '@/stores/auth'

/**
 * Shared post-auth step for login / register / one-time token login. Uses `router.history`
 * because `redirect` is a full href (path + search), as recommended by the TanStack Router docs.
 * (No React Query here: the auth pages don't load it; the shell refetches `me`.)
 */
export function useFinishAuth(redirect: string | undefined) {
  const router = useRouter()
  return useCallback(
    (result: AuthResult, message: string) => {
      useAuthStore.getState().login(result.token, result.user)
      toast.success(message)
      router.history.replace(safeRedirect(redirect, homeFor(result.user)))
    },
    [router, redirect],
  )
}
