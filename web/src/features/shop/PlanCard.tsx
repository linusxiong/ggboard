/**
 * One plan in the shop grid: name, tags, limits, Markdown description, a segmented period picker
 * (only offered prices, with per-month equivalent and saving), "current plan" / "sold out" states.
 */
import { Button, Chip, Radio, RadioGroup } from '@heroui/react'
import { Gauge, MonitorSmartphone, Sparkles } from 'lucide-react'
import { type ReactNode, useId, useState } from 'react'
import { Markdown } from '@/components/Markdown'
import { useT } from '@/i18n'
import type { Plan, PlanPeriod } from '@/lib/api/types'
import { cx } from '@/lib/cn'
import { isSoldOut, offeredPeriods, perMonth, savingPercent } from './pricing'
import { useFormatMoney } from './site'
import { SiteMoney } from './SiteMoney'

interface PlanCardProps {
  plan: Plan
  isCurrent: boolean
  onBuy: (period: PlanPeriod) => void
}

export function PlanCard({ plan, isCurrent, onBuy }: PlanCardProps) {
  const t = useT(['shop', 'common'])
  const money = useFormatMoney()
  const offers = offeredPeriods(plan)
  const [period, setPeriod] = useState<PlanPeriod | undefined>(offers[0]?.period)
  const offer = offers.find((o) => o.period === period) ?? offers[0]
  const titleId = useId()

  const soldOut = isSoldOut(plan) && !isCurrent
  const purchasable = isCurrent ? plan.renew : plan.sell
  const disabledReason = !offer
    ? t('card.noPrice')
    : soldOut
      ? t('card.soldOut')
      : !purchasable
        ? isCurrent
          ? t('card.noRenew')
          : t('card.notForSale')
        : null

  const monthly = offer ? perMonth(offer.cents, offer.period) : null
  const saving = offer ? savingPercent(plan, offer) : null

  return (
    <article
      aria-labelledby={titleId}
      className={cx(
        'surface-card flex h-full flex-col gap-5 p-6',
        isCurrent && 'ring-2 ring-tint ring-offset-0',
      )}
    >
      <header className="flex flex-col gap-2">
        <div className="flex items-start justify-between gap-3">
          <h2 id={titleId} className="text-title3 text-label">
            {plan.name}
          </h2>
          {isCurrent ? (
            <Chip size="sm" color="accent" variant="primary">
              {t('card.current')}
            </Chip>
          ) : soldOut ? (
            <Chip size="sm" color="danger" variant="soft">
              {t('card.soldOut')}
            </Chip>
          ) : null}
        </div>
        {plan.tags.length > 0 ? (
          <ul className="flex flex-wrap gap-1.5" aria-label={t('card.tags')}>
            {plan.tags.map((tag) => (
              <li key={tag}>
                <Chip size="sm" variant="soft">
                  {tag}
                </Chip>
              </li>
            ))}
          </ul>
        ) : null}
      </header>

      {offer ? (
        <div className="flex flex-col gap-1">
          <p className="flex items-baseline gap-1.5">
            <SiteMoney cents={offer.cents} trimZeros className="text-large-title text-label" />
            <span className="text-subhead text-label-secondary">
              / {t(`common:periodUnit.${offer.period}`)}
            </span>
          </p>
          <p className="min-h-5 text-footnote text-label-secondary">
            {monthly !== null && offer.period !== 'monthly'
              ? t('card.perMonth', { price: money(monthly) })
              : ' '}
            {saving !== null ? (
              <span className="ms-2 font-semibold text-label-green">
                {t('card.save', { percent: saving })}
              </span>
            ) : null}
          </p>
        </div>
      ) : null}

      {offers.length > 1 ? (
        <RadioGroup
          aria-label={t('card.period', { name: plan.name })}
          value={offer?.period ?? null}
          onChange={(value) => setPeriod(value as PlanPeriod)}
          orientation="horizontal"
          className="flex flex-row flex-wrap gap-2"
        >
          {offers.map(({ period: p, cents }) => (
            <Radio key={p} value={p} className="group">
              <Radio.Content
                className={cx(
                  'rounded-full bg-fill-tertiary px-3.5 py-1.5 text-subhead font-medium text-label transition-colors',
                  'group-data-selected:bg-tint group-data-selected:text-white',
                  'data-focus-visible:ring-2 data-focus-visible:ring-tint data-focus-visible:ring-offset-2',
                )}
              >
                {t(`common:periods.${p}`)}
                <span className="sr-only"> {money(cents)}</span>
              </Radio.Content>
            </Radio>
          ))}
        </RadioGroup>
      ) : null}

      <ul className="flex flex-col gap-2 text-subhead text-label">
        <Spec icon={<Sparkles className="size-4" />}>{t('card.traffic', { gb: plan.transfer_enable })}</Spec>
        <Spec icon={<MonitorSmartphone className="size-4" />}>
          {plan.device_limit ? t('card.devices', { count: plan.device_limit }) : t('card.unlimitedDevices')}
        </Spec>
        <Spec icon={<Gauge className="size-4" />}>
          {plan.speed_limit ? t('card.speed', { mbps: plan.speed_limit }) : t('card.unlimitedSpeed')}
        </Spec>
      </ul>

      {plan.content ? (
        <Markdown source={plan.content} className="text-subhead text-label-secondary [&_li]:ps-0.5" />
      ) : null}

      <div className="mt-auto flex flex-col gap-2 pt-1">
        <Button
          fullWidth
          size="lg"
          variant={isCurrent ? 'secondary' : 'primary'}
          isDisabled={disabledReason !== null}
          onPress={() => offer && onBuy(offer.period)}
          aria-label={
            isCurrent ? t('card.renewNamed', { name: plan.name }) : t('card.buyNamed', { name: plan.name })
          }
        >
          {disabledReason ?? (isCurrent ? t('card.renew') : t('card.buy'))}
        </Button>
      </div>
    </article>
  )
}

function Spec({ icon, children }: { icon: ReactNode; children: ReactNode }) {
  return (
    <li className="flex items-center gap-2.5">
      <span
        className="flex size-6 items-center justify-center rounded-full bg-fill-tertiary text-label-secondary"
        aria-hidden="true"
      >
        {icon}
      </span>
      <span>{children}</span>
    </li>
  )
}
