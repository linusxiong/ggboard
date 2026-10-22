import { Button } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { ChevronDown, FileCode2 } from 'lucide-react'
import { lazy, Suspense, useState } from 'react'
import { PageShell } from '@/components/PageShell'
import { QueryStatus } from '@/features/dashboard/QueryStatus'
import { withSubscribeQuery } from '@/features/subscription/importLinks'
import { RuleBuilder } from '@/features/subscription/RuleBuilder'
import { SAVED_PROFILE, SubscribeCard } from '@/features/subscription/SubscribeCard'
import { loadNamespaces, useT } from '@/i18n'
import { guestConfigQuery, ruleProfileQuery, subscriptionQuery, userCatalogQuery } from '@/lib/api/queries'

const PreviewPanel = lazy(() => import('@/features/subscription/PreviewPanel'))

export const Route = createFileRoute('/_user/subscription')({
  // Loaders stay in the entry bundle (only components are auto-split): keep the query factories lazy.
  loader: async ({ context: { queryClient } }) => {
    const [q] = await Promise.all([import('@/lib/api/queries'), loadNamespaces(['subscription'])])
    const prime = (p: Promise<unknown>) => p.catch(() => undefined)
    await Promise.all([
      prime(queryClient.query({ ...q.subscriptionQuery(), staleTime: 'static' })),
      prime(queryClient.query({ ...q.userCatalogQuery(), staleTime: 'static' })),
      prime(queryClient.query({ ...q.ruleProfileQuery(), staleTime: 'static' })),
      prime(queryClient.query({ ...q.guestConfigQuery(), staleTime: 'static' })),
    ])
  },
  component: SubscriptionPage,
})

function SubscriptionPage() {
  const t = useT(['subscription', 'common'])
  const sub = useQuery(subscriptionQuery())
  const catalog = useQuery(userCatalogQuery())
  const profile = useQuery(ruleProfileQuery())
  const config = useQuery(guestConfigQuery())
  const [override, setOverride] = useState(SAVED_PROFILE)
  const [previewOpen, setPreviewOpen] = useState(false)
  const allowCustom = config.data?.allow_user_rule_profile !== false

  return (
    <PageShell title={t('title')} description={t('subtitle')}>
      <QueryStatus query={sub} minHeight={360}>
        {(data) => (
          <SubscribeCard
            url={withSubscribeQuery(data.subscribe_url, override ? { profile: override } : {})}
            appName={config.data?.app_name ?? 'GG Bond'}
            catalog={catalog.data}
            override={override}
            onOverride={setOverride}
            allowOverride={allowCustom}
          />
        )}
      </QueryStatus>

      <QueryStatus query={catalog} minHeight={640}>
        {(cat) => (
          <QueryStatus query={profile} minHeight={640}>
            {(saved) => <RuleBuilder catalog={cat} saved={saved} readOnly={!allowCustom} />}
          </QueryStatus>
        )}
      </QueryStatus>

      <section className="surface-card flex flex-col gap-4 p-6" aria-labelledby="preview-title">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <span
              className="flex size-8 items-center justify-center rounded-[10px] bg-fill-tertiary text-label-secondary"
              aria-hidden="true"
            >
              <FileCode2 className="size-4" />
            </span>
            <div className="min-w-0">
              <h2 id="preview-title" className="text-headline text-label">
                {t('preview.title')}
              </h2>
              <p className="text-footnote text-label-secondary">
                {override ? t('preview.hintOverride') : t('preview.hint')}
              </p>
            </div>
          </div>
          <Button
            variant="secondary"
            size="sm"
            aria-expanded={previewOpen}
            aria-controls="preview-panel"
            onPress={() => setPreviewOpen((v) => !v)}
          >
            {previewOpen ? t('preview.hide') : t('preview.show')}
            <ChevronDown
              className={
                previewOpen ? 'size-4 rotate-180 transition-transform' : 'size-4 transition-transform'
              }
              aria-hidden="true"
            />
          </Button>
        </div>
        {previewOpen ? (
          <div id="preview-panel">
            <Suspense fallback={<div className="h-48 animate-pulse rounded-[12px] bg-fill-quaternary" />}>
              <PreviewPanel profile={override || undefined} />
            </Suspense>
          </div>
        ) : null}
      </section>
    </PageShell>
  )
}
