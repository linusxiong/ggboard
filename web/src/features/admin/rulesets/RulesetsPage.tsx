import { Alert, Chip, Skeleton, Tabs } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { useMemo } from 'react'
import { PageShell } from '@/components/PageShell'
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
    <PageShell title={t('rulesets.title')} description={t('rulesets.description')}>
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
        <Tabs selectedKey={tab} onSelectionChange={(k) => onTabChange(String(k) as RulesetTab)}>
          <Tabs.ListContainer>
            <Tabs.List aria-label={t('rulesets.title')}>
              {RULESET_TABS.map((id) => (
                <Tabs.Tab key={id} id={id} className="gap-1.5 whitespace-nowrap">
                  {t(`rulesets.tabs.${id}`)}
                  {counts[id] !== null ? (
                    <span className="text-caption tabular text-label-secondary">{counts[id]}</span>
                  ) : Object.values(customIds).some((s) => s.size > 0) ? (
                    <Chip size="sm" variant="soft" color="warning">
                      {t('rulesets.active')}
                    </Chip>
                  ) : null}
                  <Tabs.Indicator />
                </Tabs.Tab>
              ))}
            </Tabs.List>
          </Tabs.ListContainer>
          {meta(q.data?.builtin) ? (
            <p className="mt-3 text-footnote text-label-secondary">{meta(q.data?.builtin)}</p>
          ) : null}
          <Tabs.Panel id="sources" className="pt-4">
            <SourcesView sources={catalog.sources} customIds={customIds.sources} locale={locale} />
          </Tabs.Panel>
          <Tabs.Panel id="categories" className="pt-4">
            <CategoriesView
              categories={catalog.categories}
              customIds={customIds.categories}
              locale={locale}
            />
          </Tabs.Panel>
          <Tabs.Panel id="templates" className="pt-4">
            <TemplatesView templates={catalog.templates} customIds={customIds.templates} locale={locale} />
          </Tabs.Panel>
          <Tabs.Panel id="mirrors" className="pt-4">
            <MirrorsView mirrors={catalog.mirrors ?? []} customIds={customIds.mirrors} locale={locale} />
          </Tabs.Panel>
          <Tabs.Panel id="custom" className="pt-4">
            <CustomEditor custom={custom} catalog={catalog} />
          </Tabs.Panel>
        </Tabs>
      )}
    </PageShell>
  )

  function meta(builtin: RulesetCatalogFull | undefined) {
    const generated = builtin?.meta?.generated_at
    return generated
      ? t('rulesets.builtinMeta', { date: generated.slice(0, 10), version: builtin?.meta?.version ?? 1 })
      : null
  }
}
