import { useState } from 'react'
import { confirm } from '@/components/feedback/confirm'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import type { AdminUser } from '@/lib/api/types'
import { useAdminMutation } from '../shared/useAdminMutation'

export type UserAction = 'ban' | 'unban' | 'resetSecret' | 'loginAs' | 'resetTraffic' | 'delete'

/** Row / drawer actions on a user, each confirmed where it is destructive. */
export function useUserActions(onDeleted?: (id: number) => void) {
  const t = useT(['admin-users', 'common'])
  const [loginUrl, setLoginUrl] = useState<{ email: string; url: string } | null>(null)

  const ban = useAdminMutation({
    mutationFn: (v: { id: number; banned: boolean }) => adminApi.updateUser(v.id, { banned: v.banned }),
    success: (_d, v) => (v.banned ? t('actions.banned') : t('actions.unbanned')),
  })
  const resetSecret = useAdminMutation({
    mutationFn: (id: number) => adminApi.resetUserSecret(id),
    success: t('actions.secretReset'),
  })
  const resetTraffic = useAdminMutation({
    mutationFn: (id: number) => adminApi.resetUserTraffic(id),
    success: t('actions.trafficReset'),
  })
  const remove = useAdminMutation({
    mutationFn: (id: number) => adminApi.deleteUser(id),
    success: t('actions.deleted'),
    onSuccess: (_d, id) => onDeleted?.(id),
  })
  const loginAs = useAdminMutation({
    mutationFn: (user: AdminUser) => adminApi.userLoginToken(user.id),
    invalidate: [],
    onSuccess: ({ url }, user) =>
      setLoginUrl({ email: user.email, url: new URL(url, window.location.origin).href }),
  })

  const run = async (action: UserAction, user: AdminUser) => {
    const name = user.email
    switch (action) {
      case 'ban':
        if (
          await confirm({
            title: t('actions.banTitle', { name }),
            description: t('actions.banHint'),
            confirmLabel: t('actions.ban'),
            tone: 'danger',
          })
        )
          ban.mutate({ id: user.id, banned: true })
        break
      case 'unban':
        ban.mutate({ id: user.id, banned: false })
        break
      case 'resetSecret':
        if (
          await confirm({
            title: t('actions.resetSecretTitle', { name }),
            description: t('actions.resetSecretHint'),
            confirmLabel: t('actions.resetSecret'),
            tone: 'danger',
          })
        )
          resetSecret.mutate(user.id)
        break
      case 'resetTraffic':
        if (
          await confirm({
            title: t('actions.resetTrafficTitle', { name }),
            description: t('actions.resetTrafficHint'),
            confirmLabel: t('actions.resetTraffic'),
          })
        )
          resetTraffic.mutate(user.id)
        break
      case 'loginAs':
        loginAs.mutate(user)
        break
      case 'delete':
        if (
          await confirm({
            title: t('actions.deleteTitle', { name }),
            description: t('actions.deleteHint'),
            confirmLabel: t('common:actions.delete'),
            tone: 'danger',
          })
        )
          remove.mutate(user.id)
        break
    }
  }

  return {
    run,
    loginUrl,
    closeLoginUrl: () => setLoginUrl(null),
    isPending:
      ban.isPending ||
      resetSecret.isPending ||
      resetTraffic.isPending ||
      remove.isPending ||
      loginAs.isPending,
  }
}
