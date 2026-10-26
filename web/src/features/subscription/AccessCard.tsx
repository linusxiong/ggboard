/**
 * Subscription security: recent access (`GET /user/subscription/access-log`, masked IPs), leak-protection
 * status, "rotate link" (token only, `POST /user/subscription/rotate-token`) vs "reset everything" (uuid +
 * token, `ResetSubscription`).
 */
import { Button } from '@heroui/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { RotateCw, ShieldCheck } from 'lucide-react'
import { useState } from 'react'
import { confirm } from '@/components/feedback/confirm'
import { HelpTip, TitleWithHelp } from '@/components/HelpTip'
import { useNow } from '@/features/admin/shared/useNow'
import { useT } from '@/i18n'
import { qk } from '@/lib/api/queries'
import type { UserSubscription } from '@/lib/api/types'
import { rotateSubscriptionToken } from '@/lib/api/user'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { AccessLogList } from './AccessLogList'
import { recentIpCount, sortAccess } from './accessLog'
import { accessLogQuery, isMissing, telegramQuery } from './queries'
import { ResetSubscription } from './ResetSubscription'

const PREVIEW_ROWS = 5

export function AccessCard() {
  const t = useT(['subscription', 'common'])
  const queryClient = useQueryClient()
  const log = useQuery(accessLogQuery())
  const telegram = useQuery(telegramQuery())
  const now = useNow()
  const [expanded, setExpanded] = useState(false)

  const rotate = useMutation({
    mutationFn: rotateSubscriptionToken,
    onSuccess: async (result) => {
      queryClient.setQueryData<UserSubscription>(qk.user.subscription(), (old) =>
        old ? { ...old, subscribe_url: result.subscribe_url, token: result.token } : old,
      )
      toast.success(t('access.rotated'))
      // Per-profile links carry the token too.
      await queryClient.invalidateQueries({ queryKey: qk.user.subscription() })
    },
    onError: (error) => toast.danger(errorMessage(error)),
  })

  const onRotate = async () => {
    const ok = await confirm({
      title: t('access.rotateTitle'),
      description: t('access.rotateDescription'),
      confirmLabel: t('access.rotateConfirm'),
      tone: 'danger',
    })
    if (ok) rotate.mutate()
  }

  const entries = log.data ? sortAccess(log.data) : []
  const shown = expanded ? entries : entries.slice(0, PREVIEW_ROWS)
  const telegramBound = telegram.data?.bound === true
  const telegramAvailable = telegram.isSuccess && telegram.data.enabled !== false

  return (
    <section className="surface-card flex min-w-0 flex-col gap-4 p-6" aria-labelledby="access-title">
      <div className="flex items-start gap-3">
        <span
          className="flex size-9 shrink-0 items-center justify-center rounded-[10px] bg-fill-tertiary text-label-secondary"
          aria-hidden="true"
        >
          <ShieldCheck className="size-5" />
        </span>
        <TitleWithHelp
          help={`${t('access.subtitle')}\n\n${telegramBound ? t('access.leakTelegram') : t('access.leak')}`}
          subject={t('access.title')}
          className="self-center"
        >
          <h2 id="access-title" className="text-headline text-label">
            {t('access.title')}
          </h2>
        </TitleWithHelp>
      </div>

      {log.data || (telegramAvailable && !telegramBound) ? (
        <div className="flex flex-col gap-1 rounded-[12px] bg-fill-quaternary px-4 py-3">
          {log.data ? (
            <p className="text-subhead text-label">
              {t('access.distinct', { count: recentIpCount(log.data, now) })}
            </p>
          ) : null}
          {telegramAvailable && !telegramBound ? (
            <Link to="/profile" className="text-footnote text-label-blue hover:underline">
              {t('access.bindTelegram')}
            </Link>
          ) : null}
        </div>
      ) : null}

      <div className="flex flex-col gap-2" role="group" aria-labelledby="access-recent">
        <h3 id="access-recent" className="text-subhead font-semibold text-label">
          {t('access.recent')}
        </h3>
        {log.isPending ? (
          <div className="h-24 shimmer rounded-[12px] bg-fill-quaternary" aria-busy="true" />
        ) : log.isError ? (
          isMissing(log.error) ? (
            <p className="text-footnote text-label-secondary">{t('access.unavailable')}</p>
          ) : (
            <div role="alert" className="flex flex-wrap items-center justify-between gap-2">
              <p className="text-footnote text-label-red">{errorMessage(log.error)}</p>
              <Button size="sm" variant="secondary" onPress={() => void log.refetch()}>
                {t('common:actions.retry')}
              </Button>
            </div>
          )
        ) : entries.length === 0 ? (
          <p className="text-footnote text-label-secondary">{t('access.empty')}</p>
        ) : (
          <>
            <AccessLogList
              entries={shown}
              labels={{
                list: t('access.recent'),
                ip: t('access.ip'),
                client: t('access.client'),
                format: t('access.format'),
                time: t('access.time'),
                unknownClient: t('access.unknownClient'),
                location: t('access.location'),
              }}
            />
            {entries.length > PREVIEW_ROWS ? (
              <Button
                size="sm"
                variant="ghost"
                className="self-start"
                aria-expanded={expanded}
                onPress={() => setExpanded((v) => !v)}
              >
                {expanded ? t('access.showLess') : t('access.showAll', { count: entries.length })}
              </Button>
            ) : null}
          </>
        )}
      </div>

      <div className="flex flex-wrap items-center gap-x-4 gap-y-2 border-t border-hairline pt-4">
        <div className="flex items-center gap-1">
          <Button size="sm" variant="secondary" isPending={rotate.isPending} onPress={() => void onRotate()}>
            <RotateCw className="size-4" aria-hidden="true" />
            {t('access.rotate')}
          </Button>
          <HelpTip subject={t('access.rotate')}>{t('access.rotateHint')}</HelpTip>
        </div>
        <div className="flex items-center gap-1">
          <ResetSubscription size="sm" />
          <HelpTip subject={t('reset.action')}>{t('access.resetHint')}</HelpTip>
        </div>
      </div>
    </section>
  )
}
