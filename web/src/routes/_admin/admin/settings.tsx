import { createFileRoute, Link, useBlocker, useNavigate } from '@tanstack/react-router'
import { FileText, Link2 } from 'lucide-react'
import { useCallback } from 'react'
import { confirm } from '@/components/feedback/confirm'
import { optString } from '@/features/admin/orders/search'
import { SettingsPage } from '@/features/admin/settings/SettingsPage'
import { loadNamespaces, t, useT } from '@/i18n'
import { buttonClass } from '@/lib/button'
import { requireAdmin } from '@/lib/guards'

export const Route = createFileRoute('/_admin/admin/settings')({
  beforeLoad: requireAdmin,
  validateSearch: (raw: Record<string, unknown>): { group?: string } => {
    const group = optString(raw.group)
    return group ? { group } : {}
  },
  loader: () => loadNamespaces(['admin-settings']),
  component: AdminSettingsRoute,
})

/** Asks before leaving the page with unsaved settings (switching groups keeps the drafts). */
function LeaveGuard({ dirty }: { dirty: boolean }) {
  useBlocker({
    shouldBlockFn: async ({ current, next }) => {
      if (!dirty || current.pathname === next.pathname) return false
      const leave = await confirm({
        title: t('admin-settings', 'leave.title'),
        description: t('admin-settings', 'leave.hint'),
        confirmLabel: t('admin-settings', 'leave.confirm'),
        tone: 'danger',
      })
      return !leave
    },
    enableBeforeUnload: dirty,
  })
  return null
}

function AdminSettingsRoute() {
  const tr = useT('admin-settings')
  const { group } = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const onGroupChange = useCallback(
    (next: string) => void navigate({ search: { group: next }, replace: true }),
    [navigate],
  )
  return (
    <SettingsPage
      group={group}
      onGroupChange={onGroupChange}
      renderGuard={(dirty) => <LeaveGuard dirty={dirty} />}
      rulesetsLink={
        <Link to="/admin/rulesets" className={buttonClass({ variant: 'secondary', size: 'sm' })}>
          <Link2 className="size-4" aria-hidden />
          {tr('subscribe.rulesets')}
        </Link>
      }
      mailTemplatesLink={
        <Link to="/admin/mail-templates" className={buttonClass({ variant: 'secondary', size: 'sm' })}>
          <FileText className="size-4" aria-hidden />
          {tr('email.templates')}
        </Link>
      }
    />
  )
}
