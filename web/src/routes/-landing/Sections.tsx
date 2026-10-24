/**
 * Below-the-fold landing plans, split from the route chunk so the hero renders with the smallest possible
 * first-load bundle (the features grid is static and ships with the route: see Features.tsx). Files in
 * `-landing/` are ignored by the route generator (`-` prefix).
 */
import { Link } from '@tanstack/react-router'
import { type ReactNode, use } from 'react'
import { Money } from '@/components/Formatters'
import { Check } from './icons'
import { BlurText, WhenIdle } from '@/components/effects'
import { useT } from '@/i18n'
import { guestPlansResource } from '@/lib/publicData'
import { PLAN_PERIODS, type Plan, type PlanPeriod } from '@/lib/api/types'
import { buttonClass } from '@/lib/button'

interface SectionsProps {
  signedIn: boolean
  currency?: string | undefined
  symbol?: string | undefined
}

export default function LandingSections({ signedIn, currency, symbol }: SectionsProps) {
  return <Plans signedIn={signedIn} currency={currency} symbol={symbol} />
}

function firstPrice(plan: Plan): { period: PlanPeriod; cents: number } | null {
  for (const period of PLAN_PERIODS) {
    const cents = plan.prices[period]
    if (typeof cents === 'number') return { period, cents }
  }
  return null
}

function Plans({ signedIn, currency, symbol }: SectionsProps) {
  const t = useT('landing')
  // Suspends (with the section's chunk) until plans arrive: a single reveal, no skeleton swap.
  const plans = use(guestPlansResource())
  const isError = plans === null

  return (
    <section id="plans" aria-labelledby="plans-title" className="scroll-mt-20 bg-elevated py-20">
      <div className="mx-auto max-w-6xl px-4 sm:px-6">
        <h2 id="plans-title" className="text-center text-title1 text-label">
          {t('plans.title')}
        </h2>
        <WhenIdle
          fallback={
            <p className="mt-2 text-center text-subhead text-label-secondary">{t('plans.subtitle')}</p>
          }
        >
          <BlurText
            text={t('plans.subtitle')}
            className="mt-2 justify-center text-center text-subhead text-label-secondary"
            delay={60}
            animateBy="words"
          />
        </WhenIdle>

        <div className="mt-10 min-h-[320px]">
          {isError || plans.length === 0 ? (
            <p className="py-16 text-center text-subhead text-label-secondary">
              {isError ? t('plans.error') : t('plans.empty')}
            </p>
          ) : (
            <ul className="grid gap-4 md:grid-cols-3">
              {plans.map((plan) => {
                const price = firstPrice(plan)
                return (
                  <li key={plan.id} className="flex flex-col rounded-card bg-canvas p-6 hairline">
                    <div className="flex items-start justify-between gap-2">
                      <h3 className="text-title3 text-label">{plan.name}</h3>
                      {plan.tags[0] ? (
                        <span className="rounded-full bg-accent-soft px-2.5 py-0.5 text-caption font-semibold text-label-blue-soft">
                          {plan.tags[0]}
                        </span>
                      ) : null}
                    </div>
                    {price ? (
                      <p className="mt-4 flex items-baseline gap-1">
                        <Money
                          cents={price.cents}
                          currency={currency}
                          symbol={symbol}
                          trimZeros
                          className="text-large-title text-label"
                        />
                        <span className="text-subhead text-label-secondary">
                          / {t(`periodUnit.${price.period}`)}
                        </span>
                      </p>
                    ) : null}
                    {plan.content ? (
                      <p className="mt-3 text-subhead text-label-secondary">{plan.content}</p>
                    ) : null}
                    <ul className="mt-5 flex flex-1 flex-col gap-2 text-subhead text-label">
                      <Feature>{t('plans.traffic', { gb: plan.transfer_enable })}</Feature>
                      <Feature>
                        {plan.device_limit
                          ? t('plans.devices', { count: plan.device_limit })
                          : t('plans.unlimitedDevices')}
                      </Feature>
                      {plan.conn_limit ? (
                        <Feature>{t('plans.connections', { count: plan.conn_limit })}</Feature>
                      ) : null}
                      <Feature>
                        {plan.speed_limit
                          ? t('plans.speed', { mbps: plan.speed_limit })
                          : t('plans.unlimitedSpeed')}
                      </Feature>
                    </ul>
                    <Link
                      to={signedIn ? '/shop' : '/register'}
                      className={buttonClass({ fullWidth: true, className: 'mt-6' })}
                      aria-label={t('plans.choose', { name: plan.name })}
                    >
                      {t('plans.choose', { name: plan.name })}
                    </Link>
                  </li>
                )
              })}
            </ul>
          )}
        </div>
      </div>
    </section>
  )
}

function Feature({ children }: { children: ReactNode }) {
  return (
    <li className="flex items-center gap-2">
      <Check className="size-4 shrink-0 text-label-green" aria-hidden="true" />
      <span>{children}</span>
    </li>
  )
}
