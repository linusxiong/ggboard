/** `/admin/routes` — node routes (XrayR routes / soga audit rules). */
import { Button, Chip } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { Pencil, Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { createDataTableColumns, DataTable } from '@/components/data-table'
import { confirm } from '@/components/feedback/confirm'
import { PageShell } from '@/components/PageShell'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import { adminRoutesQuery, qk } from '@/lib/api/queries'
import type { NodeRoute } from '@/lib/api/types'
import { useAdminMutation } from '../shared/useAdminMutation'
import { RouteDialog } from './RouteDialog'

const col = createDataTableColumns<NodeRoute>()

export function RoutesPage() {
  const t = useT(['admin-nodes', 'common'])
  const routes = useQuery(adminRoutesQuery())
  const [dialog, setDialog] = useState<{ route: NodeRoute | null } | null>(null)
  const remove = useAdminMutation({
    mutationFn: (id: number) => adminApi.deleteRoute(id),
    success: t('routes.deleted'),
    invalidate: [qk.admin.routes(), qk.admin.nodes()],
  })

  const columns = useMemo(
    () =>
      col.columns([
        col.accessor('id', {
          header: 'ID',
          enableSorting: false,
          meta: { hideOnMobile: true, className: 'w-16' },
        }),
        col.accessor('remarks', {
          header: t('routes.remarks'),
          enableSorting: false,
          meta: { isPrimary: true, isRowHeader: true },
          cell: (c) => <span className="font-medium">{c.getValue() || t('routes.untitled')}</span>,
        }),
        col.accessor('match', {
          header: t('routes.match'),
          enableSorting: false,
          cell: (c) => {
            const match = c.getValue()
            return (
              <span className="flex flex-wrap items-center gap-1">
                {match.slice(0, 2).map((m, i) => (
                  <code
                    key={i}
                    className="max-w-56 truncate rounded-[6px] bg-fill-tertiary px-1.5 py-0.5 font-mono text-footnote"
                  >
                    {m}
                  </code>
                ))}
                {match.length > 2 ? (
                  <span className="text-footnote text-label-secondary">
                    {t('routes.more', { count: match.length - 2 })}
                  </span>
                ) : null}
              </span>
            )
          },
        }),
        col.accessor('action', {
          header: t('routes.action'),
          enableSorting: false,
          cell: (c) => (
            <Chip size="sm" variant="soft" color={c.getValue() === 'block' ? 'danger' : 'accent'}>
              {t(`routes.actions.${c.getValue()}`)}
            </Chip>
          ),
        }),
        col.accessor('action_value', {
          header: t('routes.actionValue'),
          enableSorting: false,
          cell: (c) => (c.getValue() ? <code className="font-mono text-footnote">{c.getValue()}</code> : '—'),
        }),
      ]),
    [t],
  )

  return (
    <PageShell
      title={t('routes.title')}
      description={t('routes.description')}
      actions={
        <Button onPress={() => setDialog({ route: null })}>
          <Plus className="size-4" aria-hidden="true" />
          {t('routes.create')}
        </Button>
      }
    >
      <DataTable
        aria-label={t('routes.title')}
        columns={columns}
        data={routes.data}
        isLoading={routes.isPending}
        isFetching={routes.isFetching && !routes.isPending}
        getRowId={(r) => String(r.id)}
        emptyState={<p className="py-8 text-center text-subhead text-label-secondary">{t('routes.empty')}</p>}
        rowActions={(r) => (
          <>
            <Button
              size="sm"
              variant="ghost"
              isIconOnly
              aria-label={t('routes.editNamed', { name: r.remarks || `#${r.id}` })}
              onPress={() => setDialog({ route: r })}
            >
              <Pencil className="size-4" aria-hidden="true" />
            </Button>
            <Button
              size="sm"
              variant="ghost"
              isIconOnly
              aria-label={t('routes.deleteNamed', { name: r.remarks || `#${r.id}` })}
              onPress={() =>
                void confirm({
                  title: t('routes.deleteTitle', { name: r.remarks || `#${r.id}` }),
                  description: t('routes.deleteHint'),
                  confirmLabel: t('common:actions.delete'),
                  tone: 'danger',
                }).then((yes) => yes && remove.mutate(r.id))
              }
            >
              <Trash2 className="size-4 text-label-red" aria-hidden="true" />
            </Button>
          </>
        )}
      />
      {dialog ? (
        <RouteDialog
          key={dialog.route?.id ?? 'new'}
          route={dialog.route}
          isOpen
          onOpenChange={(open) => {
            if (!open) setDialog(null)
          }}
        />
      ) : null}
    </PageShell>
  )
}
