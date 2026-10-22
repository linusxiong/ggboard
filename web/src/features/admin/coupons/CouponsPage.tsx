import { Button, Chip, Dropdown, Label, Modal } from '@heroui/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ClipboardCopy, Ellipsis, Plus } from 'lucide-react'
import { useMemo, useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { createDataTableColumns, DataTable } from '@/components/data-table'
import { confirm } from '@/components/feedback/confirm'
import { DateTime, Money } from '@/components/Formatters'
import { PageShell } from '@/components/PageShell'
import { ShowSwitch } from '@/features/admin/content/ShowSwitch'
import { useCurrency } from '@/features/admin/orders/useCurrency'
import { useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { adminCouponsQuery, adminPlansQuery, qk } from '@/lib/api/queries'
import type { AdminCoupon, Coupon, ID, Page } from '@/lib/api/types'
import { copyText } from '@/lib/clipboard'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { CouponFormDialog } from './CouponFormDialog'
import { validity } from './datetime'
import type { CouponSearch } from './search'
import { ModalClose } from '@/components/feedback/ModalClose'

const col = createDataTableColumns<AdminCoupon>()
const VALIDITY_COLOR = { active: 'success', upcoming: 'accent', expired: 'default' } as const

interface CouponsPageProps {
  search: CouponSearch
  onSearchChange: (patch: Partial<CouponSearch>) => void
}

export function CouponsPage({ search, onSearchChange }: CouponsPageProps) {
  const t = useT(['admin-orders', 'common'])
  const qc = useQueryClient()
  const { currency, symbol } = useCurrency()
  const query = { page: search.page, page_size: search.page_size, sort: search.sort ?? '-created_at' }
  const coupons = useQuery(adminCouponsQuery(query))
  const plans = useQuery(adminPlansQuery())
  const [editing, setEditing] = useState<AdminCoupon | null>(null)
  const [formOpen, setFormOpen] = useState(false)
  const [generated, setGenerated] = useState<Coupon[] | null>(null)

  const planName = useMemo(() => new Map((plans.data ?? []).map((p) => [p.id, p.name])), [plans.data])

  const { mutate: toggleShow } = useMutation({
    mutationFn: ({ id, show }: { id: ID; show: boolean }) => adminApi.updateCoupon(id, { show }),
    onMutate: async ({ id, show }) => {
      const key = qk.admin.coupons(query)
      await qc.cancelQueries({ queryKey: key })
      const prev = qc.getQueryData<Page<AdminCoupon>>(key)
      qc.setQueryData<Page<AdminCoupon>>(key, (p) =>
        p ? { ...p, items: p.items.map((c) => (c.id === id ? { ...c, show } : c)) } : p,
      )
      return { prev, key }
    },
    onError: (e, _v, ctx) => {
      if (ctx?.prev) qc.setQueryData(ctx.key, ctx.prev)
      toast.danger(errorMessage(e))
    },
    onSettled: () => qc.invalidateQueries({ queryKey: qk.admin.coupons() }),
  })

  const remove = async (c: AdminCoupon) => {
    const ok = await confirm({
      title: t('coupons.deleteTitle', { name: c.name }),
      description: t('coupons.deleteHint', { code: c.code }),
      confirmLabel: t('common:actions.delete'),
      tone: 'danger',
    })
    if (!ok) return
    try {
      await adminApi.deleteCoupon(c.id)
      toast.success(t('coupons.deleted'))
      await qc.invalidateQueries({ queryKey: qk.admin.coupons() })
    } catch (e) {
      toast.danger(errorMessage(e))
    }
  }

  const copyCodes = async (codes: string[]) => {
    if (codes.length === 0) return
    if (await copyText(codes.join('\n'))) toast.success(t('coupons.codesCopied', { count: codes.length }))
    else toast.danger(t('common:copy.failed'))
  }

  const columns = useMemo(
    () =>
      col.columns([
        col.accessor('name', {
          header: t('coupons.fields.name'),
          meta: { isPrimary: true },
          cell: (c) => (
            <span className="flex flex-col">
              <span className="text-label">{c.getValue()}</span>
              <span className="inline-flex items-center gap-1 font-mono text-footnote text-label-secondary">
                {c.row.original.code}
              </span>
            </span>
          ),
        }),
        col.accessor('value', {
          header: t('coupons.fields.value'),
          enableSorting: false,
          cell: (c) =>
            c.row.original.type === 1 ? (
              <span>
                {t('coupons.minus')} <Money cents={c.getValue()} currency={currency} symbol={symbol} />
              </span>
            ) : (
              t('coupons.percentOff', { value: c.getValue() })
            ),
        }),
        col.accessor('used_count', {
          header: t('coupons.fields.usage'),
          meta: { align: 'end' },
          cell: (c) => (
            <span className="tabular">
              {c.getValue()} / {c.row.original.limit_use ?? '∞'}
              {c.row.original.limit_use_with_user ? (
                <span className="block text-footnote text-label-secondary">
                  {t('coupons.perUser', { count: c.row.original.limit_use_with_user })}
                </span>
              ) : null}
            </span>
          ),
        }),
        col.accessor('started_at', {
          header: t('coupons.fields.validity'),
          cell: (c) => {
            const v = validity(c.row.original.started_at, c.row.original.ended_at)
            return (
              <span className="flex flex-col items-start gap-1">
                <Chip size="sm" variant="soft" color={VALIDITY_COLOR[v]}>
                  {t(`coupons.validity.${v}`)}
                </Chip>
                <span className="text-footnote text-label-secondary">
                  <DateTime value={c.row.original.started_at} format="date" /> –{' '}
                  <DateTime value={c.row.original.ended_at} format="date" />
                </span>
              </span>
            )
          },
        }),
        col.accessor((cp) => cp.limit_plan_ids, {
          id: 'limits',
          header: t('coupons.fields.limits'),
          enableSorting: false,
          meta: { hideOnMobile: true },
          cell: (c) => {
            const plansText = c.row.original.limit_plan_ids?.length
              ? c.row.original.limit_plan_ids.map((id) => planName.get(id) ?? `#${id}`).join(', ')
              : t('coupons.anyPlan')
            const periodsText = c.row.original.limit_periods?.length
              ? c.row.original.limit_periods.map((p) => t(`common:periods.${p}`)).join(', ')
              : t('coupons.anyPeriod')
            return (
              <span className="flex max-w-56 flex-col text-footnote">
                <span className="truncate" title={plansText}>
                  {plansText}
                </span>
                <span className="truncate text-label-secondary" title={periodsText}>
                  {periodsText}
                </span>
              </span>
            )
          },
        }),
        col.accessor('show', {
          header: t('coupons.fields.show'),
          cell: (c) => (
            <ShowSwitch
              isSelected={c.getValue()}
              aria-label={t('coupons.showFor', { name: c.row.original.name })}
              onChange={(show) => toggleShow({ id: c.row.original.id, show })}
            />
          ),
        }),
      ]),
    [t, currency, symbol, planName, toggleShow],
  )

  const pageCodes = (coupons.data?.items ?? []).map((c) => c.code)

  return (
    <PageShell
      title={t('coupons.title')}
      description={t('coupons.description')}
      actions={
        <>
          <Button
            variant="secondary"
            isDisabled={pageCodes.length === 0}
            onPress={() => void copyCodes(pageCodes)}
          >
            <ClipboardCopy className="size-4" aria-hidden />
            {t('coupons.copyPage')}
          </Button>
          <Button
            onPress={() => {
              setEditing(null)
              setFormOpen(true)
            }}
          >
            <Plus className="size-4" aria-hidden />
            {t('coupons.generate')}
          </Button>
        </>
      }
    >
      <DataTable
        aria-label={t('coupons.title')}
        columns={columns}
        data={coupons.data?.items}
        total={coupons.data?.total}
        isLoading={coupons.isPending}
        isFetching={coupons.isFetching}
        page={search.page}
        pageSize={search.page_size}
        sort={query.sort}
        onPageChange={(p) => onSearchChange({ page: p })}
        onSortChange={(sort) => onSearchChange({ sort, page: 1 })}
        getRowId={(c) => String(c.id)}
        rowActions={(c) => (
          <div className="flex items-center gap-1">
            <CopyButton value={c.code} variant="ghost" />
            <Dropdown>
              <Button
                size="sm"
                variant="ghost"
                isIconOnly
                aria-label={t('coupons.actionsFor', { name: c.name })}
              >
                <Ellipsis className="size-4" aria-hidden />
              </Button>
              <Dropdown.Popover placement="bottom end">
                <Dropdown.Menu
                  onAction={(key) => {
                    if (key === 'edit') {
                      setEditing(c)
                      setFormOpen(true)
                    } else if (key === 'delete') void remove(c)
                  }}
                >
                  <Dropdown.Item id="edit" textValue={t('common:actions.edit')}>
                    <Label>{t('common:actions.edit')}</Label>
                  </Dropdown.Item>
                  <Dropdown.Item id="delete" textValue={t('common:actions.delete')} variant="danger">
                    <Label>{t('common:actions.delete')}</Label>
                  </Dropdown.Item>
                </Dropdown.Menu>
              </Dropdown.Popover>
            </Dropdown>
          </div>
        )}
      />

      <CouponFormDialog
        isOpen={formOpen}
        onOpenChange={setFormOpen}
        coupon={editing}
        plans={plans.data ?? []}
        currency={currency}
        onCreated={(list) => {
          if (list.length > 1) setGenerated(list)
        }}
      />

      <Modal.Backdrop isOpen={generated !== null} onOpenChange={(open) => !open && setGenerated(null)}>
        <Modal.Container size="md" scroll="inside">
          <Modal.Dialog>
            <ModalClose />
            <Modal.Header>
              <Modal.Heading>{t('coupons.generatedTitle', { count: generated?.length ?? 0 })}</Modal.Heading>
            </Modal.Header>
            <Modal.Body>
              <ul className="inset-group max-h-80 overflow-auto font-mono text-subhead">
                {generated?.map((c) => (
                  <li key={c.id} className="flex items-center justify-between px-4 py-2">
                    {c.code}
                    <CopyButton value={c.code} variant="ghost" notify={false} />
                  </li>
                ))}
              </ul>
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="secondary">
                {t('common:actions.close')}
              </Button>
              <Button onPress={() => void copyCodes((generated ?? []).map((c) => c.code))}>
                <ClipboardCopy className="size-4" aria-hidden />
                {t('coupons.copyAll')}
              </Button>
            </Modal.Footer>
          </Modal.Dialog>
        </Modal.Container>
      </Modal.Backdrop>
    </PageShell>
  )
}
