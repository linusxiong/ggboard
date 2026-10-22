import { createFileRoute, useNavigate } from '@tanstack/react-router'
import { useCallback } from 'react'
import { optEnum } from '@/features/admin/orders/search'
import { RULESET_TABS, type RulesetTab } from '@/features/admin/rulesets/tabs'
import { RulesetsPage } from '@/features/admin/rulesets/RulesetsPage'
import { loadNamespaces } from '@/i18n'
import { requireAdmin } from '@/lib/guards'

export const Route = createFileRoute('/_admin/admin/rulesets')({
  beforeLoad: requireAdmin,
  validateSearch: (raw: Record<string, unknown>): { tab?: RulesetTab } => {
    const tab = optEnum(raw.tab, RULESET_TABS)
    return tab ? { tab } : {}
  },
  loader: () => loadNamespaces(['admin-settings']),
  component: AdminRulesetsRoute,
})

function AdminRulesetsRoute() {
  const { tab } = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const onTabChange = useCallback(
    (next: RulesetTab) => void navigate({ search: { tab: next }, replace: true }),
    [navigate],
  )
  return <RulesetsPage tab={tab ?? 'sources'} onTabChange={onTabChange} />
}
