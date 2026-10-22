import { type QueryKey, useMutation, useQueryClient } from '@tanstack/react-query'
import { qk } from '@/lib/api/queries'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'

interface AdminMutationOptions<TVars, TData> {
  mutationFn: (vars: TVars) => Promise<TData>
  /** Success toast (omit for none). */
  success?: string | ((data: TData, vars: TVars) => string)
  /** Keys to invalidate after success (default: everything under `qk.admin.all`). */
  invalidate?: readonly QueryKey[]
  onSuccess?: (data: TData, vars: TVars) => void
  /** Leave error handling to the caller (forms map field errors themselves). */
  silentError?: boolean
}

/**
 * `useMutation` with the admin defaults: invalidate, toast on success, toast the server message
 * on failure (409 conflicts such as "plan still has users" surface verbatim).
 */
export function useAdminMutation<TVars = void, TData = unknown>(options: AdminMutationOptions<TVars, TData>) {
  const queryClient = useQueryClient()
  return useMutation({
    mutationFn: options.mutationFn,
    onSuccess: async (data, vars) => {
      options.onSuccess?.(data, vars)
      if (options.success)
        toast.success(typeof options.success === 'function' ? options.success(data, vars) : options.success)
      await Promise.all(
        (options.invalidate ?? [qk.admin.all]).map((queryKey) => queryClient.invalidateQueries({ queryKey })),
      )
    },
    onError: (error) => {
      if (!options.silentError) toast.danger(errorMessage(error))
    },
  })
}
