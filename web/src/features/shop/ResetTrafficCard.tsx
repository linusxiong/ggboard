import { Button } from '@heroui/react'
import { RotateCcw } from 'lucide-react'
import { useT } from '@/i18n'
import type { Plan } from '@/lib/api/types'
import { SiteMoney } from './SiteMoney'

/** "Reset traffic" add-on for the current plan (only when `prices.reset_traffic` exists). */
export function ResetTrafficCard({ plan, cents, onBuy }: { plan: Plan; cents: number; onBuy: () => void }) {
  const t = useT('shop')
  return (
    <section
      aria-labelledby="reset-traffic-title"
      className="surface-card flex flex-col gap-4 p-5 sm:flex-row sm:items-center sm:justify-between"
    >
      <div className="flex items-start gap-4">
        <span
          className="flex size-11 shrink-0 items-center justify-center rounded-[12px] bg-system-orange text-white"
          aria-hidden="true"
        >
          <RotateCcw className="size-5" />
        </span>
        <div className="min-w-0">
          <h2 id="reset-traffic-title" className="text-headline text-label">
            {t('reset.title')}
          </h2>
          <p className="mt-0.5 text-subhead text-label-secondary">{t('reset.body', { name: plan.name })}</p>
        </div>
      </div>
      <div className="flex items-center gap-4 sm:shrink-0">
        <SiteMoney cents={cents} className="text-title3 text-label" />
        <Button variant="secondary" onPress={onBuy}>
          {t('reset.buy')}
        </Button>
      </div>
    </section>
  )
}
