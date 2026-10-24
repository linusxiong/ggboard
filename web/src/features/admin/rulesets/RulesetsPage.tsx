import { Alert, Skeleton } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { PageShell } from '@/components/PageShell'
import { SegmentedTabs } from '@/components/SegmentedControl'
import { useLocale, useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import type { RulesetCatalogFull } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { RULESETS_FULL_KEY } from './catalog'
import { RULESET_TABS, type RulesetTab } from './tabs'
import { CategoriesView, MirrorsView, SourcesView, TemplatesView } from './CatalogViews'
import { CustomEditor } from './CustomEditor'

interface RulesetsPageProps {
  tab: RulesetTab
  onTabChange: (tab: RulesetTab) => void
}

const EMPTY: RulesetCatalogFull = { sources: {}, categories: [], templates: [], mirrors: [] }

export function RulesetsPage({ tab, onTabChange }: RulesetsPageProps) {
  const t = useT(['admin-settings', 'common'])
  const locale = useLocale()
  const q = useQuery({ queryKey: RULESETS_FULL_KEY, queryFn: ({ signal }) => adminApi.rulesetsFull(signal) })
  const catalog = q.data?.effective ?? q.data?.builtin ?? EMPTY
  const custom = q.data?.custom ?? null

  const customIds = useMemo(
    () => ({
      sources: new Set(Object.keys(custom?.sources ?? {})),
      categories: new Set((custom?.categories ?? []).map((c) => c.id)),
      templates: new Set((custom?.templates ?? []).map((c) => c.id)),
      mirrors: new Set((custom?.mirrors ?? []).map((c) => c.id)),
    }),
    [custom],
  )
  const counts: Record<RulesetTab, number | null> = {
    sources: Object.keys(catalog.sources).length,
    categories: catalog.categories.length,
    templates: catalog.templates.length,
    mirrors: catalog.mirrors?.length ?? 0,
    custom: null,
  }

  return (
    <PageShell title={t('rulesets.title')} description={meta(q.data?.builtin)} help={t('rulesets.help')}>
      {q.isError ? (
        <Alert status="danger">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>{t('rulesets.loadFailed')}</Alert.Title>
            <Alert.Description>{errorMessage(q.error)}</Alert.Description>
          </Alert.Content>
        </Alert>
      ) : q.isPending ? (
        <div className="flex flex-col gap-4" aria-busy="true">
          <Skeleton className="h-10 w-full max-w-xl rounded-[12px]" />
          <Skeleton className="h-96 w-full rounded-card" />
        </div>
      ) : (
        <SegmentedTabs value={tab} onChange={onTabChange} className="flex flex-col gap-4">
          <SegmentedTabs.List
            aria-label={t('rulesets.title')}
            items={RULESET_TABS.map((id) => ({
              id,
              label: t(`rulesets.tabs.${id}`),
              count: counts[id],
              adornment:
                counts[id] === null && Object.values(customIds).some((ids) => ids.size > 0) ? (
                  <span
                    role="img"
                    aria-label={t('rulesets.active')}
                    className="size-2 rounded-full bg-system-orange"
                  />
                ) : undefined,
            }))}
          />
          <SegmentedTabs.Panel id="sources">
            <SourcesView sources={catalog.sources} customIds={customIds.sources} locale={locale} />
          </SegmentedTabs.Panel>
          <SegmentedTabs.Panel id="categories">
            <CategoriesView
              categories={catalog.categories}
              customIds={customIds.categories}
              locale={locale}
            />
          </SegmentedTabs.Panel>
          <SegmentedTabs.Panel id="templates">
            <TemplatesView templates={catalog.templates} customIds={customIds.templates} locale={locale} />
          </SegmentedTabs.Panel>
          <SegmentedTabs.Panel id="mirrors">
            <MirrorsView mirrors={catalog.mirrors ?? []} customIds={customIds.mirrors} locale={locale} />
          </SegmentedTabs.Panel>
          <SegmentedTabs.Panel id="custom">
            <CustomEditor custom={custom} catalog={catalog} />
          </SegmentedTabs.Panel>
        </SegmentedTabs>
      )}
    </PageShell>
  )

  /** Subtitle: when the built-in catalog was generated (a fact; the explanation is in the ⓘ). */
  function meta(builtin: RulesetCatalogFull | undefined) {
    const generated = builtin?.meta?.generated_at
    return generated
      ? t('rulesets.builtinMeta', { date: generated.slice(0, 10), version: builtin?.meta?.version ?? 1 })
      : undefined
  }
}
