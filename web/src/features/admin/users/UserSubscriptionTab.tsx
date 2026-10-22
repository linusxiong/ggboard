/**
 * User drawer → Subscription (FE-S): the user's subscription access log with full IPs and token tags
 * (`GET /admin/users/:id/subscription/access-log`), their age key status (`GET …/subscription/age`, public key
 * only), age key reset (`POST …/subscription/age/reset`: new key pair, switch kept) and link rotation.
 * There is no admin token-rotation endpoint, so "rotate" reuses `reset-secret` (uuid + token, sessions end).
 */
import { Button, Chip } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { KeyRound, RotateCw } from 'lucide-react'
import { confirm } from '@/components/feedback/confirm'
import { AccessLogList } from '@/features/subscription/AccessLogList'
import { recentIpCount, sortAccess } from '@/features/subscription/accessLog'
import { isMissing } from '@/features/subscription/queries'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import { qk } from '@/lib/api/queries'
import type { AdminUser } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { useAdminMutation } from '../shared/useAdminMutation'
import { useNow } from '../shared/useNow'

const userAccessLogKey = (id: number) => [...qk.admin.user(id), 'subscription-access'] as const
const userAgeKey = (id: number) => [...qk.admin.user(id), 'subscription-age'] as const

export function UserSubscriptionTab({ user }: { user: AdminUser }) {
  const t = useT(['admin-users', 'common'])
  const now = useNow()
  const log = useQuery({
    queryKey: userAccessLogKey(user.id),
    queryFn: ({ signal }) => adminApi.userSubscriptionAccessLog(user.id, signal),
  })
  const age = useQuery({
    queryKey: userAgeKey(user.id),
    queryFn: ({ signal }) => adminApi.userAgeKey(user.id, signal),
    retry: false,
  })
  const hasAgeKey = Boolean(age.data?.public_key)
  const resetAge = useAdminMutation({
    mutationFn: () => adminApi.resetUserAgeKey(user.id),
    success: t('subscription.ageReset'),
    invalidate: [qk.admin.user(user.id)],
    silentError: true,
  })
  const rotate = useAdminMutation({
    mutationFn: () => adminApi.resetUserSecret(user.id),
    success: t('subscription.rotated'),
  })

  const onResetAge = async () => {
    if (
      await confirm({
        title: t('subscription.resetAgeTitle', { name: user.email }),
        description: t('subscription.resetAgeHint'),
        confirmLabel: t('subscription.resetAge'),
        tone: 'danger',
      })
    )
      resetAge.mutate(undefined, {
        onError: (error) => toast.danger(isMissing(error) ? t('subscription.ageNone') : errorMessage(error)),
      })
  }
  const onRotate = async () => {
    if (
      await confirm({
        title: t('subscription.rotateTitle', { name: user.email }),
        description: t('subscription.rotateHint'),
        confirmLabel: t('subscription.rotate'),
        tone: 'danger',
      })
    )
      rotate.mutate()
  }

  const entries = log.data ? sortAccess(log.data) : []

  return (
    <div className="flex flex-col gap-5">
      <section className="flex flex-col gap-2" aria-labelledby="user-sub-log">
        <div className="flex flex-wrap items-end justify-between gap-2">
          <div>
            <h3 id="user-sub-log" className="text-headline text-label">
              {t('subscription.accessLog')}
            </h3>
            <p className="text-footnote text-label-secondary">{t('subscription.accessHint')}</p>
          </div>
          {log.data ? (
            <span className="text-footnote text-label-secondary">
              {t('subscription.distinct', { count: recentIpCount(log.data, now) })}
            </span>
          ) : null}
        </div>
        {log.isPending ? (
          <div className="flex flex-col gap-2" aria-busy="true">
            {Array.from({ length: 3 }, (_, i) => (
              <span key={i} className="block h-12 animate-pulse rounded-[10px] bg-fill-tertiary" />
            ))}
          </div>
        ) : log.isError ? (
          <p role={isMissing(log.error) ? undefined : 'alert'} className="text-subhead text-label-secondary">
            {isMissing(log.error) ? t('subscription.unavailable') : errorMessage(log.error)}
          </p>
        ) : entries.length === 0 ? (
          <p className="text-subhead text-label-secondary">{t('subscription.empty')}</p>
        ) : (
          <AccessLogList
            entries={entries}
            className="inset-group max-h-[28rem] overflow-auto"
            labels={{
              list: t('subscription.accessLog'),
              ip: t('subscription.ip'),
              client: t('subscription.client'),
              format: t('subscription.format'),
              time: t('subscription.time'),
              unknownClient: '—',
              tokenTag: t('subscription.tokenTag'),
            }}
          />
        )}
      </section>

      {age.data ? (
        <section className="flex flex-col gap-2" aria-labelledby="user-sub-age">
          <h3 id="user-sub-age" className="flex items-center gap-2 text-headline text-label">
            {t('subscription.age')}
            {hasAgeKey ? (
              <Chip size="sm" variant="soft" color={age.data.enabled ? 'success' : 'default'}>
                {age.data.enabled ? t('subscription.ageOn') : t('subscription.ageOff')}
              </Chip>
            ) : null}
          </h3>
          {hasAgeKey ? (
            <dl className="rounded-[12px] bg-fill-quaternary px-4 py-3">
              <dt className="text-footnote text-label-secondary">{t('subscription.publicKey')}</dt>
              <dd className="break-all font-mono text-footnote text-label">{age.data.public_key}</dd>
            </dl>
          ) : (
            <p className="text-subhead text-label-secondary">{t('subscription.ageNone')}</p>
          )}
        </section>
      ) : null}

      <section className="flex flex-col gap-3" aria-labelledby="user-sub-actions">
        <h3 id="user-sub-actions" className="text-headline text-label">
          {t('subscription.actions')}
        </h3>
        <div className="flex flex-wrap gap-2">
          <Button
            size="sm"
            variant="secondary"
            isPending={resetAge.isPending}
            isDisabled={age.isSuccess && !hasAgeKey}
            onPress={() => void onResetAge()}
          >
            <KeyRound className="size-4" aria-hidden="true" />
            {t('subscription.resetAge')}
          </Button>
          <Button
            size="sm"
            variant="danger-soft"
            isPending={rotate.isPending}
            onPress={() => void onRotate()}
          >
            <RotateCw className="size-4" aria-hidden="true" />
            {t('subscription.rotate')}
          </Button>
        </div>
        <p className="text-footnote text-label-secondary">{t('subscription.rotateNote')}</p>
      </section>
    </div>
  )
}
