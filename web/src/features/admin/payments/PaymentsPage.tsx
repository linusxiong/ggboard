import { Button, Chip, Dropdown, Label, Skeleton } from '@heroui/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { CreditCard, Ellipsis, Plus } from 'lucide-react'
import { type ReactNode, useMemo, useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { EmptyState } from '@/components/EmptyState'
import { confirm } from '@/components/feedback/confirm'
import { Money } from '@/components/Formatters'
import { PageShell } from '@/components/PageShell'
import { moveId } from '@/features/admin/content/reorder'
import { ShowSwitch } from '@/features/admin/content/ShowSwitch'
import { SortButtons } from '@/features/admin/content/SortButtons'
import { useCurrency } from '@/features/admin/orders/useCurrency'
import { useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { adminGatewaysQuery, adminPaymentsQuery, guestConfigQuery, qk } from '@/lib/api/queries'
import type { ID, Payment } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { formatNumber } from '@/lib/format'
import { toast } from '@/lib/toast'
import { useLocale } from '@/i18n'
import { notifyUrlFor, WEBHOOK_GATEWAYS } from './gateways'
import { PaymentSheet } from './PaymentSheet'

export function PaymentsPage() {
  const t = useT(['admin-payments', 'common'])
  const locale = useLocale()
  const qc = useQueryClient()
  const { currency, symbol } = useCurrency()
  const config = useQuery(guestConfigQuery())
  const payments = useQuery(adminPaymentsQuery())
  const gateways = useQuery(adminGatewaysQuery())
  const [sheet, setSheet] = useState<{ open: boolean; payment: Payment | null }>({
    open: false,
    payment: null,
  })

  const list = useMemo(
    () => [...(payments.data ?? [])].sort((a, b) => a.sort - b.sort || a.id - b.id),
    [payments.data],
  )
  const labelOf = useMemo(() => new Map((gateways.data ?? []).map((g) => [g.name, g.label])), [gateways.data])
  const appUrl = config.data?.app_url ?? ''

  const setList = (next: Payment[]) => qc.setQueryData(qk.admin.payments(), next)

  const sortMutation = useMutation({
    mutationFn: (ids: ID[]) => adminApi.sortPayments(ids),
    onMutate: (ids) => {
      const prev = qc.getQueryData<Payment[]>(qk.admin.payments())
      setList(ids.map((id, i) => ({ ...(list.find((p) => p.id === id) as Payment), sort: i + 1 })))
      return { prev }
    },
    onError: (e, _ids, ctx) => {
      if (ctx?.prev) setList(ctx.prev)
      toast.danger(errorMessage(e))
    },
    onSettled: () => qc.invalidateQueries({ queryKey: qk.admin.payments() }),
  })

  const toggle = useMutation({
    mutationFn: ({ id, enabled }: { id: ID; enabled: boolean }) => adminApi.updatePayment(id, { enabled }),
    onMutate: ({ id, enabled }) => {
      const prev = qc.getQueryData<Payment[]>(qk.admin.payments())
      if (prev) setList(prev.map((p) => (p.id === id ? { ...p, enabled } : p)))
      return { prev }
    },
    onError: (e, _v, ctx) => {
      if (ctx?.prev) setList(ctx.prev)
      toast.danger(errorMessage(e))
    },
    onSuccess: (_d, v) => toast.success(v.enabled ? t('list.enabled') : t('list.disabled')),
    onSettled: () => qc.invalidateQueries({ queryKey: qk.admin.payments() }),
  })

  const remove = async (p: Payment) => {
    const ok = await confirm({
      title: t('list.deleteTitle', { name: p.name }),
      description: t('list.deleteHint'),
      confirmLabel: t('common:actions.delete'),
      tone: 'danger',
    })
    if (!ok) return
    try {
      await adminApi.deletePayment(p.id)
      toast.success(t('list.deleted'))
      await qc.invalidateQueries({ queryKey: qk.admin.payments() })
    } catch (e) {
      toast.danger(errorMessage(e))
    }
  }

  const ids = list.map((p) => p.id)
  const fee = (p: Payment) => {
    const parts: ReactNode[] = []
    if (p.handling_fee_fixed)
      parts.push(<Money key="f" cents={p.handling_fee_fixed} currency={currency} symbol={symbol} />)
    if (p.handling_fee_percent)
      parts.push(
        <span key="p">{formatNumber(p.handling_fee_percent, locale, { maximumFractionDigits: 2 })}%</span>,
      )
    return parts.length ? (
      <span className="inline-flex flex-wrap items-center gap-1">
        {parts.flatMap((x, i) => (i ? [<span key={`s${i}`}>+</span>, x] : [x]))}
      </span>
    ) : (
      t('list.noFee')
    )
  }

  return (
    <PageShell
      title={t('title')}
      description={t('description')}
      actions={
        <Button isDisabled={!gateways.data?.length} onPress={() => setSheet({ open: true, payment: null })}>
          <Plus className="size-4" aria-hidden />
          {t('list.add')}
        </Button>
      }
    >
      {payments.isPending ? (
        <div className="surface-card flex flex-col gap-3 p-4" aria-busy="true">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-14 w-full rounded-[12px]" />
          ))}
        </div>
      ) : list.length === 0 ? (
        <div className="surface-card">
          <EmptyState
            icon={<CreditCard className="size-7" strokeWidth={1.75} />}
            title={t('list.emptyTitle')}
            description={t('list.emptyHint')}
            action={
              <Button
                isDisabled={!gateways.data?.length}
                onPress={() => setSheet({ open: true, payment: null })}
              >
                {t('list.add')}
              </Button>
            }
          />
        </div>
      ) : (
        <ul aria-label={t('title')} className="inset-group">
          {list.map((p, i) => {
            const url = notifyUrlFor(p, appUrl)
            return (
              <li key={p.id} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
                <SortButtons
                  itemLabel={p.name}
                  canUp={i > 0}
                  canDown={i < list.length - 1}
                  isDisabled={sortMutation.isPending}
                  upLabel={t('list.moveUp')}
                  downLabel={t('list.moveDown')}
                  onMove={(dir) => {
                    const next = moveId(ids, p.id, dir)
                    if (next) sortMutation.mutate(next)
                  }}
                />
                <span className="flex size-10 shrink-0 items-center justify-center overflow-hidden rounded-[10px] bg-fill-tertiary">
                  {p.icon ? (
                    <img
                      src={p.icon}
                      alt=""
                      width={40}
                      height={40}
                      loading="lazy"
                      className="size-10 object-contain"
                    />
                  ) : (
                    <CreditCard className="size-5 text-label-secondary" aria-hidden />
                  )}
                </span>
                <div className="min-w-0 flex-1 basis-48">
                  <p className="flex flex-wrap items-center gap-2 text-headline text-label">
                    {p.name}
                    <Chip size="sm" variant="soft">
                      {labelOf.get(p.gateway) ?? p.gateway}
                    </Chip>
                    {WEBHOOK_GATEWAYS.has(p.gateway) ? (
                      <Chip size="sm" variant="soft" color="warning">
                        {t('list.webhook')}
                      </Chip>
                    ) : null}
                  </p>
                  <p className="mt-0.5 flex min-w-0 items-center gap-1 text-footnote text-label-secondary">
                    <span className="shrink-0">{t('list.fee')}:</span> {fee(p)}
                  </p>
                  <p className="flex min-w-0 items-center gap-1 text-footnote text-label-secondary">
                    <span className="shrink-0">{t('list.notifyUrl')}:</span>
                    <span className="truncate font-mono" title={url}>
                      {url}
                    </span>
                    <CopyButton value={url} variant="ghost" />
                  </p>
                </div>
                <div className="flex items-center gap-2">
                  <ShowSwitch
                    isSelected={p.enabled}
                    aria-label={t('list.enableFor', { name: p.name })}
                    onChange={(enabled) => toggle.mutate({ id: p.id, enabled })}
                  />
                  <Button
                    size="sm"
                    variant="tertiary"
                    aria-label={`${t('common:actions.edit')}: ${p.name}`}
                    onPress={() => setSheet({ open: true, payment: p })}
                  >
                    {t('common:actions.edit')}
                  </Button>
                  <Dropdown>
                    <Button
                      size="sm"
                      variant="ghost"
                      isIconOnly
                      aria-label={t('list.moreFor', { name: p.name })}
                    >
                      <Ellipsis className="size-4" aria-hidden />
                    </Button>
                    <Dropdown.Popover placement="bottom end">
                      <Dropdown.Menu onAction={(key) => key === 'delete' && void remove(p)}>
                        <Dropdown.Item id="delete" textValue={t('common:actions.delete')} variant="danger">
                          <Label>{t('common:actions.delete')}</Label>
                        </Dropdown.Item>
                      </Dropdown.Menu>
                    </Dropdown.Popover>
                  </Dropdown>
                </div>
              </li>
            )
          })}
        </ul>
      )}

      <PaymentSheet
        isOpen={sheet.open}
        onOpenChange={(open) => setSheet((s) => ({ ...s, open }))}
        payment={sheet.payment}
        gateways={gateways.data ?? []}
        currency={currency}
        appUrl={appUrl}
      />
    </PageShell>
  )
}
