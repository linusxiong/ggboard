import { Chip } from '@heroui/react'
import { Link } from '@tanstack/react-router'
import { CalendarClock, Gauge, MonitorSmartphone, RotateCcw } from 'lucide-react'
import type { ReactNode } from 'react'
import { DateTime } from '@/components/Formatters'
import { TrafficBar } from '@/components/TrafficBar'
import { useT } from '@/i18n'
import type { UserSubscription } from '@/lib/api/types'
import { buttonClass } from '@/lib/button'
import { daysUntil } from '@/lib/format'
import { planState } from './format'

const STATE_COLOR = { active: 'success', expired: 'danger', depleted: 'warning', none: 'default' } as const

/** One `<dt>`/`<dd>` group (a `<dl>` allows exactly one wrapping `<div>`): the icon lives inside the `<dt>`. */
function Fact({ icon, label, children }: { icon: ReactNode; label: string; children: ReactNode }) {
  return (
    <div className="relative min-w-0 ps-11">
      <dt className="text-footnote text-label-secondary">
        <span
          className="absolute start-0 top-0.5 flex size-8 items-center justify-center rounded-[10px] bg-fill-tertiary text-label-secondary"
          aria-hidden="true"
        >
          {icon}
        </span>
        {label}
      </dt>
      <dd className="text-subhead text-label">{children}</dd>
    </div>
  )
}

export function PlanCard({ sub }: { sub: UserSubscription }) {
  const t = useT(['dashboard', 'common'])
  const state = planState(sub)

  if (state === 'none') {
    return (
      <section className="surface-card flex flex-col items-start gap-3 p-6" aria-labelledby="plan-title">
        <h2 id="plan-title" className="text-title3 text-label">
          {t('plan.noneTitle')}
        </h2>
        <p className="text-subhead text-label-secondary">{t('plan.noneHint')}</p>
        <Link to="/shop" className={buttonClass({ size: 'md' })}>
          {t('plan.buy')}
        </Link>
      </section>
    )
  }

  const left = daysUntil(sub.expired_at)
  const resetIn = daysUntil(sub.next_reset_at)
  return (
    <section className="surface-card flex flex-col gap-5 p-6" aria-labelledby="plan-title">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="min-w-0">
          <p className="text-footnote text-label-secondary">{t('plan.current')}</p>
          <h2 id="plan-title" className="truncate text-title2 text-label">
            {sub.plan?.name}
          </h2>
        </div>
        <Chip size="sm" variant="soft" color={STATE_COLOR[state]}>
          {t(`plan.state.${state}`)}
        </Chip>
      </div>

      <TrafficBar used={sub.u + sub.d} total={sub.transfer_enable} />

      <dl className="grid gap-4 sm:grid-cols-2">
        <Fact icon={<CalendarClock className="size-4" />} label={t('plan.expires')}>
          {sub.expired_at ? (
            <>
              <DateTime value={sub.expired_at} format="date" />
              {left !== null && left >= 0 ? (
                <span className="text-label-secondary"> · {t('plan.daysLeft', { count: left })}</span>
              ) : null}
            </>
          ) : (
            t('plan.forever')
          )}
        </Fact>
        <Fact icon={<RotateCcw className="size-4" />} label={t('plan.reset')}>
          {sub.next_reset_at && resetIn !== null ? (
            <>
              {t('plan.resetIn', { count: Math.max(0, resetIn) })}
              <span className="text-label-secondary">
                {' · '}
                <DateTime value={sub.next_reset_at} format="date" />
              </span>
            </>
          ) : (
            t('plan.noReset')
          )}
        </Fact>
        <Fact icon={<Gauge className="size-4" />} label={t('plan.speed')}>
          {sub.speed_limit ? t('plan.mbps', { value: sub.speed_limit }) : t('plan.unlimitedSpeed')}
        </Fact>
        <Fact icon={<MonitorSmartphone className="size-4" />} label={t('plan.devices')}>
          {sub.device_limit
            ? t('plan.deviceCount', { count: sub.device_limit })
            : t('common:values.unlimited')}
        </Fact>
      </dl>

      {state !== 'active' ? (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-[12px] bg-fill-quaternary px-4 py-3">
          <p className="text-subhead text-label">{t(`plan.${state}Hint`)}</p>
          <Link to="/shop" className={buttonClass({ size: 'sm' })}>
            {t('plan.renew')}
          </Link>
        </div>
      ) : null}
    </section>
  )
}
