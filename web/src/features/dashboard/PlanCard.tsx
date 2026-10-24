import { Chip } from '@heroui/react'
import { Link } from '@tanstack/react-router'
import { Cable, CalendarClock, Gauge, MonitorSmartphone, Package, RotateCcw } from 'lucide-react'
import type { ReactNode } from 'react'
import { DateTime } from '@/components/Formatters'
import { TrafficRing } from '@/components/TrafficRing'
import { useLocale, useT } from '@/i18n'
import type { UserSubscription } from '@/lib/api/types'
import { buttonClass } from '@/lib/button'
import { cx } from '@/lib/cn'
import { daysUntil, formatBytes } from '@/lib/format'
import { planState } from './format'

const STATE_COLOR = { active: 'success', expired: 'danger', depleted: 'warning', none: 'default' } as const

/**
 * One `<dt>`/`<dd>` group (a `<dl>` allows exactly one wrapping `<div>`): the symbol badge lives
 * inside the `<dt>`. Badge = SF Symbols style: tinted glyph on a soft squircle.
 */
function Fact({
  icon,
  tint,
  label,
  children,
}: {
  icon: ReactNode
  tint: string
  label: string
  children: ReactNode
}) {
  return (
    <div className="relative min-w-0 ps-11">
      <dt className="text-footnote text-label-secondary">
        <span
          className={cx(
            'absolute start-0 top-0.5 flex size-8 items-center justify-center rounded-[9px]',
            tint,
          )}
          aria-hidden="true"
        >
          {icon}
        </span>
        {label}
      </dt>
      <dd className="text-subhead font-medium text-label">{children}</dd>
    </div>
  )
}

export function PlanCard({ sub }: { sub: UserSubscription }) {
  const t = useT(['dashboard', 'common'])
  const locale = useLocale()
  const state = planState(sub)

  if (state === 'none') {
    return (
      <section
        className="surface-card flex h-full flex-col items-start justify-center gap-4 p-6 sm:p-7"
        aria-labelledby="plan-title"
      >
        <span
          className="flex size-12 items-center justify-center rounded-[14px] bg-system-blue text-white"
          aria-hidden="true"
        >
          <Package className="size-6" />
        </span>
        <div>
          <h2 id="plan-title" className="text-title3 text-label">
            {t('plan.noneTitle')}
          </h2>
          <p className="mt-1 max-w-md text-subhead text-label-secondary">{t('plan.noneHint')}</p>
        </div>
        <Link to="/shop" className={buttonClass()}>
          {t('plan.buy')}
        </Link>
      </section>
    )
  }

  const used = sub.u + sub.d
  const left = daysUntil(sub.expired_at)
  const resetIn = daysUntil(sub.next_reset_at)
  const remaining = sub.transfer_enable > 0 ? Math.max(0, sub.transfer_enable - used) : null
  return (
    <section className="surface-card flex h-full flex-col gap-6 p-6 sm:p-7" aria-labelledby="plan-title">
      <div className="flex items-center gap-5 sm:gap-6">
        <TrafficRing used={used} total={sub.transfer_enable} size={104} />
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <p className="text-footnote text-label-secondary">{t('plan.current')}</p>
            <Chip size="sm" variant="soft" color={STATE_COLOR[state]}>
              {t(`plan.state.${state}`)}
            </Chip>
          </div>
          <h2 id="plan-title" className="mt-0.5 truncate text-title2 text-label">
            {sub.plan?.name}
          </h2>
          <p className="mt-1.5 text-subhead tabular text-label">
            {formatBytes(used, locale)}
            <span className="text-label-secondary">
              {' '}
              /{' '}
              {sub.transfer_enable > 0
                ? formatBytes(sub.transfer_enable, locale)
                : t('common:values.unlimited')}
            </span>
          </p>
          {remaining !== null ? (
            <p className="text-footnote text-label-secondary">
              {t('common:traffic.remaining', { value: formatBytes(remaining, locale) })}
            </p>
          ) : null}
        </div>
      </div>

      <dl className="grid gap-x-6 gap-y-4 border-t-[0.5px] border-hairline pt-5 sm:grid-cols-2">
        <Fact
          icon={<CalendarClock className="size-[18px]" />}
          tint="bg-system-blue/12 text-system-blue"
          label={t('plan.expires')}
        >
          {sub.expired_at ? (
            <>
              <DateTime value={sub.expired_at} format="date" />
              {left !== null && left >= 0 ? (
                <span className="font-normal text-label-secondary">
                  {' '}
                  · {t('plan.daysLeft', { count: left })}
                </span>
              ) : null}
            </>
          ) : (
            t('plan.forever')
          )}
        </Fact>
        <Fact
          icon={<RotateCcw className="size-[18px]" />}
          tint="bg-system-teal/14 text-system-teal"
          label={t('plan.reset')}
        >
          {sub.next_reset_at && resetIn !== null ? (
            <>
              {t('plan.resetIn', { count: Math.max(0, resetIn) })}
              <span className="font-normal text-label-secondary">
                {' · '}
                <DateTime value={sub.next_reset_at} format="date" />
              </span>
            </>
          ) : (
            t('plan.noReset')
          )}
        </Fact>
        <Fact
          icon={<Gauge className="size-[18px]" />}
          tint="bg-system-orange/14 text-system-orange"
          label={t('plan.speed')}
        >
          {sub.speed_limit ? t('plan.mbps', { value: sub.speed_limit }) : t('plan.unlimitedSpeed')}
        </Fact>
        <Fact
          icon={<MonitorSmartphone className="size-[18px]" />}
          tint="bg-system-purple/12 text-system-purple"
          label={t('plan.devices')}
        >
          {sub.device_limit
            ? t('plan.deviceCount', { count: sub.device_limit })
            : t('common:values.unlimited')}
        </Fact>
        {sub.conn_limit ? (
          <Fact
            icon={<Cable className="size-[18px]" />}
            tint="bg-system-indigo/12 text-system-indigo"
            label={t('plan.connections')}
          >
            {t('plan.connCount', { count: sub.conn_limit })}
          </Fact>
        ) : null}
      </dl>

      {state !== 'active' ? (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-[14px] bg-fill-quaternary px-4 py-3">
          <p className="text-subhead text-label">{t(`plan.${state}Hint`)}</p>
          <Link to="/shop" className={buttonClass({ size: 'sm' })}>
            {t('plan.renew')}
          </Link>
        </div>
      ) : null}
    </section>
  )
}
