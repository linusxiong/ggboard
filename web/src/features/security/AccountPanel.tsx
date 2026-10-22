/** Profile → account: identity, e-mail reminders (`PATCH /user/me`), language and appearance. */
import { Avatar } from '@heroui/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { DateTime } from '@/components/Formatters'
import { OptionSelect, Toggle } from '@/features/subscription/controls'
import { useT } from '@/i18n'
import { qk } from '@/lib/api/queries'
import type { Locale, User } from '@/lib/api/types'
import { updateMe } from '@/lib/api/user'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { useAuthStore } from '@/stores/auth'
import { LOCALES, type ThemePreference, useUiStore } from '@/stores/ui'

type Reminders = Pick<User, 'remind_expire' | 'remind_traffic'>

export function AccountPanel({ user }: { user: User }) {
  const t = useT(['security', 'common', 'core'])
  const queryClient = useQueryClient()
  const locale = useUiStore((s) => s.locale)
  const theme = useUiStore((s) => s.theme)
  const setLocale = useUiStore((s) => s.setLocale)
  const setTheme = useUiStore((s) => s.setTheme)

  const reminders = useMutation({
    mutationFn: (patch: Partial<Reminders>) => updateMe(patch),
    onMutate: (patch) => {
      const previous = queryClient.getQueryData<User>(qk.user.me())
      if (previous) queryClient.setQueryData<User>(qk.user.me(), { ...previous, ...patch })
      return { previous }
    },
    onSuccess: (next) => {
      queryClient.setQueryData(qk.user.me(), next)
      useAuthStore.getState().setUser(next)
      toast.success(t('account.saved'))
    },
    onError: (error, _patch, context) => {
      if (context?.previous) queryClient.setQueryData(qk.user.me(), context.previous)
      toast.danger(errorMessage(error))
    },
  })

  const initial = user.email.charAt(0).toUpperCase()

  return (
    <div className="flex flex-col gap-6">
      <section
        className="surface-card flex flex-wrap items-center gap-5 p-6"
        aria-labelledby="identity-title"
      >
        <Avatar size="lg" className="size-16">
          {user.avatar_url ? <Avatar.Image src={user.avatar_url} alt="" /> : null}
          <Avatar.Fallback>{initial}</Avatar.Fallback>
        </Avatar>
        <div className="min-w-0 flex-1">
          <h2 id="identity-title" className="truncate text-title3 text-label">
            {user.email}
          </h2>
          <dl className="mt-2 grid gap-x-6 gap-y-1 text-footnote sm:grid-cols-2">
            <div className="flex gap-2">
              <dt className="text-label-secondary">{t('account.createdAt')}</dt>
              <dd className="text-label">
                <DateTime value={user.created_at} format="date" />
              </dd>
            </div>
            <div className="flex gap-2">
              <dt className="text-label-secondary">{t('account.lastLogin')}</dt>
              <dd className="text-label">
                <DateTime value={user.last_login_at} format="datetime" fallback={t('account.never')} />
              </dd>
            </div>
          </dl>
          <p className="mt-2 text-caption text-label-secondary">{t('account.avatarHint')}</p>
        </div>
      </section>

      <section className="surface-card flex flex-col gap-4 p-6" aria-labelledby="notify-title">
        <div>
          <h2 id="notify-title" className="text-headline text-label">
            {t('account.notifications')}
          </h2>
          <p className="mt-1 text-footnote text-label-secondary">{t('account.notificationsHint')}</p>
        </div>
        <div className="flex flex-col gap-3">
          <Toggle
            isSelected={user.remind_expire}
            isDisabled={reminders.isPending}
            onChange={(remind_expire) => reminders.mutate({ remind_expire })}
          >
            {t('account.remindExpire')}
          </Toggle>
          <Toggle
            isSelected={user.remind_traffic}
            isDisabled={reminders.isPending}
            onChange={(remind_traffic) => reminders.mutate({ remind_traffic })}
          >
            {t('account.remindTraffic')}
          </Toggle>
        </div>
      </section>

      <section className="surface-card flex flex-col gap-4 p-6" aria-labelledby="prefs-title">
        <h2 id="prefs-title" className="text-headline text-label">
          {t('account.preferences')}
        </h2>
        <div className="grid max-w-xl gap-4 sm:grid-cols-2">
          <OptionSelect
            label={t('core:locale.label')}
            value={locale}
            options={LOCALES.map((value) => ({ value, label: t(`core:locale.${value}`) }))}
            onChange={(value) => setLocale(value as Locale)}
          />
          <OptionSelect
            label={t('core:theme.label')}
            value={theme}
            options={(['system', 'light', 'dark'] as const).map((value) => ({
              value,
              label: t(`core:theme.${value}`),
            }))}
            onChange={(value) => setTheme(value as ThemePreference)}
          />
        </div>
      </section>
    </div>
  )
}
