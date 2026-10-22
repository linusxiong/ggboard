/** `/admin/groups` — server groups: inline create / rename, delete (409 when still in use). */
import { Button, Input, Label, TextField } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { Check, Layers, Pencil, Plus, Trash2, X } from 'lucide-react'
import { type FormEvent, useMemo, useState } from 'react'
import { EmptyState } from '@/components/EmptyState'
import { createDataTableColumns, DataTable } from '@/components/data-table'
import { confirm } from '@/components/feedback/confirm'
import { PageShell } from '@/components/PageShell'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import { adminGroupsQuery, qk } from '@/lib/api/queries'
import type { ServerGroup } from '@/lib/api/types'
import { useAdminMutation } from '../shared/useAdminMutation'
import { useFormatters } from '../shared/useFormatters'

const col = createDataTableColumns<ServerGroup>()
const INVALIDATE = [qk.admin.groups(), qk.admin.plans(), qk.admin.nodes()]

export function GroupsPage() {
  const t = useT(['admin-nodes', 'common'])
  const fmt = useFormatters()
  const groups = useQuery(adminGroupsQuery())
  const [name, setName] = useState('')
  const [editing, setEditing] = useState<{ id: number; name: string } | null>(null)

  const create = useAdminMutation({
    mutationFn: (value: string) => adminApi.createGroup(value),
    success: t('groups.created'),
    invalidate: INVALIDATE,
    onSuccess: () => setName(''),
  })
  const rename = useAdminMutation({
    mutationFn: (v: { id: number; name: string }) => adminApi.updateGroup(v.id, v.name),
    success: t('groups.renamed'),
    invalidate: INVALIDATE,
    onSuccess: () => setEditing(null),
  })
  const remove = useAdminMutation({
    mutationFn: (id: number) => adminApi.deleteGroup(id),
    success: t('groups.deleted'),
    invalidate: INVALIDATE,
  })

  const onCreate = (event: FormEvent) => {
    event.preventDefault()
    const value = name.trim()
    if (value) create.mutate(value)
  }
  const saveRename = () => {
    if (!editing) return
    const value = editing.name.trim()
    if (value) rename.mutate({ id: editing.id, name: value })
  }

  const columns = useMemo(
    () =>
      col.columns([
        col.accessor('name', {
          header: t('groups.name'),
          enableSorting: false,
          meta: { isPrimary: true, isRowHeader: true },
          cell: (c) =>
            editing?.id === c.row.original.id ? (
              <TextField
                aria-label={t('groups.rename')}
                value={editing.name}
                onChange={(value) => setEditing({ id: c.row.original.id, name: value })}
                autoFocus
                onKeyDown={(e) => {
                  if (e.key === 'Enter') saveRename()
                  if (e.key === 'Escape') setEditing(null)
                }}
                className="max-w-64"
              >
                <Input />
              </TextField>
            ) : (
              <span className="font-medium">{c.getValue()}</span>
            ),
        }),
        col.accessor('user_count', {
          header: t('groups.users'),
          enableSorting: false,
          meta: { align: 'end' },
          cell: (c) => <span className="tabular">{fmt.count(c.getValue())}</span>,
        }),
        col.accessor('plan_count', {
          header: t('groups.plans'),
          enableSorting: false,
          meta: { align: 'end' },
          cell: (c) => <span className="tabular">{fmt.count(c.getValue())}</span>,
        }),
        col.accessor('node_count', {
          header: t('groups.nodes'),
          enableSorting: false,
          meta: { align: 'end' },
          cell: (c) => <span className="tabular">{fmt.count(c.getValue())}</span>,
        }),
      ]),
    // saveRename reads `editing`, which is already a dependency
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [t, fmt, editing],
  )

  return (
    <PageShell title={t('groups.title')} description={t('groups.description')}>
      <form onSubmit={onCreate} className="surface-card flex flex-wrap items-end gap-3 p-4">
        <TextField value={name} onChange={setName} className="min-w-0 flex-1 sm:max-w-sm" name="group-name">
          <Label>{t('groups.create')}</Label>
          <Input placeholder={t('groups.placeholder')} />
        </TextField>
        <Button type="submit" isPending={create.isPending} isDisabled={!name.trim()}>
          <Plus className="size-4" aria-hidden="true" />
          {t('groups.add')}
        </Button>
      </form>
      <DataTable
        aria-label={t('groups.title')}
        columns={columns}
        data={groups.data}
        isLoading={groups.isPending}
        isFetching={groups.isFetching && !groups.isPending}
        getRowId={(g) => String(g.id)}
        emptyState={<EmptyState compact icon={<Layers />} title={t('groups.empty')} />}
        rowActions={(g) =>
          editing?.id === g.id ? (
            <>
              <Button
                size="sm"
                variant="primary"
                isIconOnly
                aria-label={t('common:actions.save')}
                isPending={rename.isPending}
                onPress={saveRename}
              >
                <Check className="size-4" aria-hidden="true" />
              </Button>
              <Button
                size="sm"
                variant="ghost"
                isIconOnly
                aria-label={t('common:actions.cancel')}
                onPress={() => setEditing(null)}
              >
                <X className="size-4" aria-hidden="true" />
              </Button>
            </>
          ) : (
            <>
              <Button
                size="sm"
                variant="ghost"
                isIconOnly
                aria-label={t('groups.renameNamed', { name: g.name })}
                onPress={() => setEditing({ id: g.id, name: g.name })}
              >
                <Pencil className="size-4" aria-hidden="true" />
              </Button>
              <Button
                size="sm"
                variant="ghost"
                isIconOnly
                aria-label={t('groups.deleteNamed', { name: g.name })}
                onPress={() =>
                  void confirm({
                    title: t('groups.deleteTitle', { name: g.name }),
                    description: t('groups.deleteHint'),
                    confirmLabel: t('common:actions.delete'),
                    tone: 'danger',
                  }).then((yes) => yes && remove.mutate(g.id))
                }
              >
                <Trash2 className="size-4 text-label-red" aria-hidden="true" />
              </Button>
            </>
          )
        }
      />
    </PageShell>
  )
}
