/** Vertical status timeline: created → paid → activated (→ credited), or created → cancelled. */
import { Check, X } from 'lucide-react'
import type { ReactNode } from 'react'
import { DateTime } from '@/components/Formatters'
import { useT } from '@/i18n'
import type { Order } from '@/lib/api/types'
import { cx } from '@/lib/cn'

type StepState = 'done' | 'current' | 'upcoming' | 'failed'
interface Step {
  key: string
  label: string
  state: StepState
  time?: ReactNode
}

export function OrderTimeline({ order }: { order: Order }) {
  const t = useT('orders')
  const { status } = order
  const paid = status === 1 || status === 3 || status === 4
  const steps: Step[] = [
    {
      key: 'created',
      label: t('timeline.created'),
      state: 'done',
      time: <DateTime value={order.created_at} />,
    },
  ]
  if (status === 2) {
    steps.push({
      key: 'cancelled',
      label: t('timeline.cancelled'),
      state: 'failed',
      time: <DateTime value={order.updated_at} />,
    })
  } else {
    steps.push({
      key: 'paid',
      label: paid ? t('timeline.paid') : t('timeline.awaitingPayment'),
      state: paid ? 'done' : 'current',
      ...(order.paid_at ? { time: <DateTime value={order.paid_at} /> } : {}),
    })
    steps.push({
      key: 'activated',
      label: status === 1 ? t('timeline.activating') : t('timeline.activated'),
      state: status === 3 || status === 4 ? 'done' : status === 1 ? 'current' : 'upcoming',
    })
    if (status === 4) {
      steps.push({
        key: 'discounted',
        label: t('timeline.discounted'),
        state: 'done',
        time: <DateTime value={order.updated_at} />,
      })
    }
  }

  return (
    <ol className="flex flex-col" aria-label={t('timeline.label')}>
      {steps.map((step, i) => (
        <li
          key={step.key}
          className="relative flex gap-3 pb-5 last:pb-0"
          aria-current={step.state === 'current' ? 'step' : undefined}
        >
          {i < steps.length - 1 ? (
            <span
              className={cx(
                'absolute start-[11px] top-6 bottom-0 w-0.5 rounded-full',
                step.state === 'done' ? 'bg-system-green' : 'bg-fill',
              )}
              aria-hidden="true"
            />
          ) : null}
          <Dot state={step.state} />
          <div className="flex min-w-0 flex-col pt-0.5">
            <span
              className={cx(
                'text-subhead',
                step.state === 'upcoming' ? 'text-label-secondary' : 'font-semibold text-label',
                step.state === 'failed' && 'text-label-red',
              )}
            >
              {step.label}
            </span>
            {step.time ? <span className="text-footnote text-label-secondary">{step.time}</span> : null}
          </div>
        </li>
      ))}
    </ol>
  )
}

function Dot({ state }: { state: StepState }) {
  return (
    <span
      className={cx(
        'relative z-10 flex size-6 shrink-0 items-center justify-center rounded-full',
        state === 'done' && 'bg-system-green text-white',
        state === 'failed' && 'bg-system-red text-white',
        state === 'current' && 'bg-elevated ring-2 ring-system-orange',
        state === 'upcoming' && 'bg-fill-secondary',
      )}
      aria-hidden="true"
    >
      {state === 'done' ? <Check className="size-3.5" strokeWidth={3} /> : null}
      {state === 'failed' ? <X className="size-3.5" strokeWidth={3} /> : null}
      {state === 'current' ? (
        <span className="size-2 rounded-full bg-system-orange motion-safe:animate-pulse" />
      ) : null}
    </span>
  )
}
