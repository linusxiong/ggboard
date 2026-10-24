/** `/admin/plans` — plans: inline show/sell/renew toggles, reorder, create/edit sheet, delete (409). */
import { Button, Chip } from '@heroui/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { Package, Pencil, Plus, Trash2 } from 'lucide-react'
import { useMemo, useState } from 'react'
import { EmptyState } from '@/components/EmptyState'
import { createDataTableColumns, DataTable } from '@/components/data-table'
import { confirm } from '@/components/feedback/confirm'
import { PageShell } from '@/components/PageShell'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import { adminGroupsQuery, adminPlansQuery, qk } from '@/lib/api/queries'
import { type AdminPlan, PLAN_PERIODS } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { InlineSwitch } from '../shared/InlineSwitch'
import { moveId } from '../shared/sort'
import { SortButtons } from '../shared/SortButtons'
import { useAdminMutation } from '../shared/useAdminMutation'
import { useFormatters } from '../shared/useFormatters'
import { PlanSheet } from './PlanSheet'

const col = createDataTableColumns<AdminPlan>()
type Flag = 'show' | 'sell' | 'renew'

export function PlansPage() {
  const t = useT(['admin-plans', 'common'])
  const fmt = useFormatters()
  const queryClient = useQueryClient()
  const plans = useQuery(adminPlansQuery())
  const groups = useQuery(adminGroupsQuery())
  const [sheet, setSheet] = useState<{ plan: AdminPlan | null } | null>(null)
  const groupName = useMemo(() => new Map((groups.data ?? []).map((g) => [g.id, g.name])), [groups.data])

  /** Optimistic flag toggle (PATCH), rolled back on error. */
  const { mutate: toggleMutate } = useMutation({
    mutationFn: (v: { id: number; flag: Flag; value: boolean }) =>
      adminApi.updatePlan(v.id, { [v.flag]: v.value }),
    onMutate: async (v) => {
      await queryClient.cancelQueries({ queryKey: qk.admin.plans() })
      const previous = queryClient.getQueryData<AdminPlan[]>(qk.admin.plans())
      queryClient.setQueryData<AdminPlan[]>(qk.admin.plans(), (rows) =>
        rows?.map((p) => (p.id === v.id ? { ...p, [v.flag]: v.value } : p)),
      )
      return { previous }
    },
    onError: (error, _v, ctx) => {
      if (ctx?.previous) queryClient.setQueryData(qk.admin.plans(), ctx.previous)
      toast.danger(errorMessage(error))
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: qk.admin.plans() }),
  })

  /** Optimistic reorder, then `POST /admin/plans/sort` with the full id list. */
  const { mutate: reorderMutate, isPending: reorderPending } = useMutation({
    mutationFn: (ids: number[]) => adminApi.sortPlans(ids),
    onMutate: async (ids) => {
      await queryClient.cancelQueries({ queryKey: qk.admin.plans() })
      const previous = queryClient.getQueryData<AdminPlan[]>(qk.admin.plans())
      if (previous) {
        const byId = new Map(previous.map((p) => [p.id, p]))
        queryClient.setQueryData(
          qk.admin.plans(),
          ids.flatMap((id, i) => {
            const p = byId.get(id)
            return p ? [{ ...p, sort: i + 1 }] : []
          }),
        )
      }
      return { previous }
    },
    onError: (error, _ids, ctx) => {
      if (ctx?.previous) queryClient.setQueryData(qk.admin.plans(), ctx.previous)
      toast.danger(errorMessage(error))
    },
    onSettled: () => queryClient.invalidateQueries({ queryKey: qk.admin.plans() }),
  })

  const remove = useAdminMutation({
    mutationFn: (id: number) => adminApi.deletePlan(id),
    success: t('deleted'),
    invalidate: [qk.admin.plans(), qk.admin.groups()],
  })

  const rows = plans.data
  const ids = useMemo(() => (rows ?? []).map((p) => p.id), [rows])

  const columns = useMemo(
    () =>
      col.columns([
        col.display({
          id: 'order',
          header: () => <span className="sr-only">{t('columns.order')}</span>,
          meta: { hideOnMobile: true, className: 'w-20' },
          cell: (c) => (
            <SortButtons
              name={c.row.original.name}
              isFirst={c.row.index === 0}
              isLast={c.row.index === ids.length - 1}
              isDisabled={reorderPending}
              onMove={(dir) => reorderMutate(moveId(ids, c.row.original.id, dir))}
              upLabel={(name) => t('moveUp', { name })}
              downLabel={(name) => t('moveDown', { name })}
            />
          ),
        }),
        col.accessor('name', {
          header: t('columns.name'),
          meta: { isPrimary: true, isRowHeader: true },
          cell: (c) => (
            <span className="flex flex-col gap-1">
              <span className="font-medium text-label">{c.getValue()}</span>
              {c.row.original.tags.length ? (
                <span className="flex flex-wrap gap-1">
                  {c.row.original.tags.map((tag) => (
                    <Chip key={tag} size="sm" variant="soft">
                      {tag}
                    </Chip>
                  ))}
                </span>
              ) : null}
            </span>
          ),
        }),
        col.accessor('group_id', {
          header: t('columns.group'),
          cell: (c) => groupName.get(c.getValue()) ?? `#${c.getValue()}`,
        }),
        col.accessor('transfer_enable', {
          header: t('columns.traffic'),
          meta: { align: 'end' },
          cell: (c) => <span className="tabular">{fmt.bytes(c.getValue() * 1024 ** 3)}</span>,
        }),
        col.display({
          id: 'prices',
          header: t('columns.prices'),
          cell: (c) => {
            const offered = PLAN_PERIODS.filter((p) => typeof c.row.original.prices[p] === 'number')
            if (offered.length === 0) return <span className="text-label-secondary">{t('notForSale')}</span>
            return (
              <span className="flex flex-col text-footnote">
                {offered.slice(0, 3).map((p) => (
                  <span key={p} className="tabular">
                    {t(`common:periods.${p}`)} · {fmt.money(c.row.original.prices[p] ?? 0)}
                  </span>
                ))}
                {offered.length > 3 ? (
                  <span className="text-label-secondary">
                    {t('morePrices', { count: offered.length - 3 })}
                  </span>
                ) : null}
              </span>
            )
          },
        }),
        col.accessor('user_count', {
          header: t('columns.users'),
          meta: { align: 'end' },
          cell: (c) => <span className="tabular">{fmt.count(c.getValue())}</span>,
        }),
        ...(['show', 'sell', 'renew'] as const).map((flag) =>
          col.accessor(flag, {
            header: t(`columns.${flag}`),
            meta: { align: 'center' },
            cell: (c) => (
              <InlineSwitch
                aria-label={t(`toggle.${flag}`, { name: c.row.original.name })}
                isSelected={c.getValue()}
                onChange={(value) => toggleMutate({ id: c.row.original.id, flag, value })}
              />
            ),
          }),
        ),
      ]),
    [t, fmt, groupName, ids, reorderMutate, reorderPending, toggleMutate],
  )

  return (
    <PageShell
      title={t('title')}
      help={t('help')}
      actions={
        <Button onPress={() => setSheet({ plan: null })}>
          <Plus className="size-4" aria-hidden="true" />
          {t('create')}
        </Button>
      }
    >
      <DataTable
        aria-label={t('title')}
        columns={columns}
        data={rows}
        isLoading={plans.isPending}
        isFetching={plans.isFetching && !plans.isPending}
        getRowId={(p) => String(p.id)}
        emptyState={<EmptyState compact icon={<Package />} title={t('empty')} />}
        rowActions={(p) => (
          <>
            <Button
              size="sm"
              variant="ghost"
              isIconOnly
              aria-label={t('editNamed', { name: p.name })}
              onPress={() => setSheet({ plan: p })}
            >
              <Pencil className="size-4" aria-hidden="true" />
            </Button>
            <Button
              size="sm"
              variant="ghost"
              isIconOnly
              aria-label={t('deleteNamed', { name: p.name })}
              onPress={() =>
                void confirm({
                  title: t('deleteTitle', { name: p.name }),
                  description: p.user_count > 0 ? t('deleteInUse', { count: p.user_count }) : t('deleteHint'),
                  confirmLabel: t('common:actions.delete'),
                  tone: 'danger',
                }).then((yes) => yes && remove.mutate(p.id))
              }
            >
              <Trash2 className="size-4 text-label-red" aria-hidden="true" />
            </Button>
          </>
        )}
      />
      {sheet ? (
        <PlanSheet
          key={sheet.plan?.id ?? 'new'}
          plan={sheet.plan}
          isOpen
          onOpenChange={(open) => {
            if (!open) setSheet(null)
          }}
        />
      ) : null}
    </PageShell>
  )
}
