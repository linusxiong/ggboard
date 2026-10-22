import { createFileRoute } from '@tanstack/react-router'
import { SubscriptionPage } from '@/features/subscription/SubscriptionPage'
import { loadNamespaces } from '@/i18n'

export const Route = createFileRoute('/_user/subscription')({
  // `?profile=<slug>` selects a subscription profile (FE-S). Search params are JSON-first: a numeric slug
  // arrives as a number.
  validateSearch: (search: Record<string, unknown>): { profile?: string } => {
    const raw = search.profile
    return typeof raw === 'string' || typeof raw === 'number' ? { profile: String(raw) } : {}
  },
  // Loaders stay in the entry bundle (only components are auto-split): keep the query factories lazy.
  loader: async ({ context: { queryClient } }) => {
    const [q, fes] = await Promise.all([
      import('@/lib/api/queries'),
      import('@/features/subscription/queries'),
      loadNamespaces(['subscription']),
    ])
    const prime = (p: Promise<unknown>) => p.catch(() => undefined)
    await Promise.all([
      prime(queryClient.query({ ...q.subscriptionQuery(), staleTime: 'static' })),
      prime(queryClient.query({ ...q.userCatalogQuery(), staleTime: 'static' })),
      prime(queryClient.query({ ...q.ruleProfileQuery(), staleTime: 'static' })),
      prime(queryClient.query({ ...q.guestConfigQuery(), staleTime: 'static' })),
      prime(queryClient.query({ ...fes.ruleProfilesQuery(), staleTime: 'static' })),
    ])
  },
  component: SubscriptionPage,
})
