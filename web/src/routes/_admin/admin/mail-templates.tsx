import { createFileRoute, Link, useBlocker, useNavigate } from '@tanstack/react-router'
import { Settings } from 'lucide-react'
import { useCallback } from 'react'
import { confirm } from '@/components/feedback/confirm'
import {
  type MailTemplateSelection,
  MailTemplatesPage,
} from '@/features/admin/mail-templates/MailTemplatesPage'
import { parseMailTemplatesSearch } from '@/features/admin/mail-templates/search'
import { loadNamespaces, t, useT } from '@/i18n'
import { buttonClass } from '@/lib/button'
import { requireAdmin } from '@/lib/guards'

export const Route = createFileRoute('/_admin/admin/mail-templates')({
  beforeLoad: requireAdmin,
  validateSearch: parseMailTemplatesSearch,
  // admin-support: the Markdown editor's strings.
  loader: () => loadNamespaces(['admin-settings', 'admin-support']),
  component: AdminMailTemplatesRoute,
})

/** Asks before leaving the page with an unsaved template (switching template/locale asks in the page). */
function LeaveGuard({ dirty }: { dirty: boolean }) {
  useBlocker({
    shouldBlockFn: async ({ current, next }) => {
      if (!dirty || current.pathname === next.pathname) return false
      const leave = await confirm({
        title: t('admin-settings', 'mailTemplates.leave.title'),
        description: t('admin-settings', 'mailTemplates.leave.hint'),
        confirmLabel: t('admin-settings', 'mailTemplates.leave.confirm'),
        tone: 'danger',
      })
      return !leave
    },
    enableBeforeUnload: dirty,
  })
  return null
}

function AdminMailTemplatesRoute() {
  const tr = useT('admin-settings')
  const { name, locale } = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const onSelect = useCallback(
    (next: MailTemplateSelection) => void navigate({ search: next, replace: true }),
    [navigate],
  )
  return (
    <MailTemplatesPage
      name={name}
      locale={locale}
      onSelect={onSelect}
      renderGuard={(dirty) => <LeaveGuard dirty={dirty} />}
      settingsLink={
        <Link
          to="/admin/settings"
          search={{ group: 'email' }}
          className={buttonClass({ variant: 'secondary', size: 'sm' })}
        >
          <Settings className="size-4" aria-hidden />
          {tr('mailTemplates.settingsLink')}
        </Link>
      }
    />
  )
}
