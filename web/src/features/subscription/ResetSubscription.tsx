import { Button } from '@heroui/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { RefreshCcw } from 'lucide-react'
import { confirm } from '@/components/feedback/confirm'
import { useT } from '@/i18n'
import { qk } from '@/lib/api/queries'
import type { UserSubscription } from '@/lib/api/types'
import { resetSubscription } from '@/lib/api/user'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { useAuthStore } from '@/stores/auth'

/**
 * Danger action: new UUID + subscription token (`POST /user/subscription/reset`). The reset bumps
 * `password_version`, so the response's `auth_token` replaces the session token to stay signed in.
 */
export function ResetSubscription({ size = 'md' }: { size?: 'sm' | 'md' }) {
  const t = useT(['subscription', 'common'])
  const queryClient = useQueryClient()
  const reset = useMutation({
    mutationFn: resetSubscription,
    onSuccess: async (result) => {
      if (result.auth_token) useAuthStore.setState({ token: result.auth_token })
      queryClient.setQueryData<UserSubscription>(qk.user.subscription(), (old) =>
        old ? { ...old, subscribe_url: result.subscribe_url, token: result.token } : old,
      )
      toast.success(t('reset.done'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: qk.user.subscription() }),
        queryClient.invalidateQueries({ queryKey: qk.user.me() }),
      ])
    },
    onError: (error) => toast.danger(errorMessage(error)),
  })

  const onPress = async () => {
    const ok = await confirm({
      title: t('reset.title'),
      description: t('reset.description'),
      confirmLabel: t('reset.confirm'),
      tone: 'danger',
    })
    if (ok) reset.mutate()
  }

  return (
    <Button variant="danger-soft" size={size} isPending={reset.isPending} onPress={() => void onPress()}>
      <RefreshCcw className="size-4" aria-hidden="true" />
      {t('reset.action')}
    </Button>
  )
}
