import { useSuspenseQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { ShoppingBag } from 'lucide-react'
import { lazy, Suspense, useState } from 'react'
import { EmptyState } from '@/components/EmptyState'
import { PageShell } from '@/components/PageShell'
import type { CheckoutSelection } from '@/features/shop/CheckoutSheet'
import { PlanCard } from '@/features/shop/PlanCard'
import { ResetTrafficCard } from '@/features/shop/ResetTrafficCard'
import { loadNamespaces, useT } from '@/i18n'
import { userPlansQuery } from '@/lib/api/queries'
import { useAuthStore } from '@/stores/auth'

const CheckoutSheet = lazy(() => import('@/features/shop/CheckoutSheet'))

export const Route = createFileRoute('/_user/shop')({
  // Loaders stay in the entry chunk (only components are split): import the query factories lazily.
  loader: async ({ context }) => {
    const q = await import('@/lib/api/queries')
    return Promise.all([
      loadNamespaces(['shop', 'common']),
      context.queryClient.query({ ...q.userPlansQuery(), staleTime: 'static' }),
      context.queryClient.query({ ...q.guestConfigQuery(), staleTime: 'static' }).catch(() => null),
    ])
  },
  component: ShopPage,
})

function ShopPage() {
  const t = useT(['shop', 'common'])
  const { data: plans } = useSuspenseQuery(userPlansQuery())
  const planId = useAuthStore((s) => s.user?.plan_id ?? null)
  const [selection, setSelection] = useState<CheckoutSelection | null>(null)

  const current = plans.find((p) => p.id === planId)
  const resetPrice = current?.prices.reset_traffic

  return (
    <PageShell title={t('title')} help={t('subtitle')}>
      {current && typeof resetPrice === 'number' ? (
        <ResetTrafficCard
          plan={current}
          cents={resetPrice}
          onBuy={() => setSelection({ plan: current, period: 'reset_traffic' })}
        />
      ) : null}

      {plans.length === 0 ? (
        <section className="surface-card">
          <EmptyState
            icon={<ShoppingBag className="size-7" strokeWidth={1.75} />}
            title={t('empty')}
            description={t('emptyHint')}
          />
        </section>
      ) : (
        <ul className="grid gap-5 md:grid-cols-2 xl:grid-cols-3" aria-label={t('listLabel')}>
          {plans.map((plan) => (
            <li key={plan.id}>
              <PlanCard
                plan={plan}
                isCurrent={plan.id === planId}
                onBuy={(period) => setSelection({ plan, period })}
              />
            </li>
          ))}
        </ul>
      )}

      {selection ? (
        <Suspense fallback={null}>
          <CheckoutSheet
            selection={selection}
            isCurrent={selection.plan.id === planId}
            onClose={() => setSelection(null)}
          />
        </Suspense>
      ) : null}
    </PageShell>
  )
}
