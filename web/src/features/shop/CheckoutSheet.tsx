/**
 * Purchase confirmation sheet (lazy-loaded by the shop page): plan + period summary, coupon check
 * (`POST /user/coupons/check`), estimated discounts and the automatic balance deduction, then
 * `POST /user/orders` → `/orders/$tradeNo`. A 409 (pending order exists) offers "go to that order"
 * or "cancel it and continue".
 */
import { Alert, Button, FieldError, InputGroup, Label, Modal, TextField } from '@heroui/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { TicketPercent, X } from 'lucide-react'
import { type FormEvent, type ReactNode, useState } from 'react'
import { FormError } from '@/components/FormError'
import { useT } from '@/i18n'
import { isApiError, userApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type { CouponCheck, Plan, PlanPeriod } from '@/lib/api/types'
import { buttonClass } from '@/lib/button'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { useAuthStore } from '@/stores/auth'
import { estimateCheckout } from './pricing'
import { findPendingOrder } from './pendingOrder'
import { useFormatMoney } from './site'
import { SiteMoney } from './SiteMoney'

export interface CheckoutSelection {
  plan: Plan
  period: PlanPeriod
}

interface CheckoutSheetProps {
  selection: CheckoutSelection
  isCurrent: boolean
  onClose: () => void
}

export default function CheckoutSheet({ selection, isCurrent, onClose }: CheckoutSheetProps) {
  const t = useT(['shop', 'common'])
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const user = useAuthStore((s) => s.user)
  const money = useFormatMoney()
  const { plan, period } = selection
  const price = plan.prices[period] ?? 0

  const [code, setCode] = useState('')
  const [coupon, setCoupon] = useState<CouponCheck | null>(null)
  const [couponError, setCouponError] = useState<string>()
  const [formError, setFormError] = useState<string>()
  /** Pending order that blocks a new one (409). `''` = exists but unknown trade_no. */
  const [pending, setPending] = useState<string | null>(null)

  const estimate = estimateCheckout(price, {
    userDiscountPercent: user?.discount ?? 0,
    coupon,
    balance: user?.balance ?? 0,
  })

  const check = useMutation({
    mutationFn: (value: string) => userApi.checkCoupon({ code: value, plan_id: plan.id, period }),
    onSuccess: (result) => {
      setCoupon(result)
      setCouponError(undefined)
    },
    onError: (error) => {
      setCoupon(null)
      setCouponError(
        isApiError(error) ? (error.fieldError('code') ?? errorMessage(error)) : errorMessage(error),
      )
    },
  })

  const create = useMutation({
    mutationFn: () =>
      userApi.createOrder({
        plan_id: plan.id,
        period,
        ...(coupon ? { coupon_code: coupon.code } : {}),
      }),
    onSuccess: async ({ trade_no }) => {
      await queryClient.invalidateQueries({ queryKey: qk.user.orders() })
      void queryClient.invalidateQueries({ queryKey: qk.user.stats() })
      onClose()
      await navigate({ to: '/orders/$tradeNo', params: { tradeNo: trade_no } })
    },
    onError: async (error) => {
      if (isApiError(error) && error.status === 409) {
        setFormError(undefined)
        setPending((await findPendingOrder(error)) ?? '')
        return
      }
      if (isApiError(error) && error.fieldError('coupon_code')) {
        setCoupon(null)
        setCouponError(error.fieldError('coupon_code'))
        return
      }
      setFormError(errorMessage(error))
    },
  })

  const cancelPending = useMutation({
    mutationFn: (tradeNo: string) => userApi.cancelOrder(tradeNo),
    onSuccess: async () => {
      toast.success(t('checkout.pendingCancelled'))
      setPending(null)
      await queryClient.invalidateQueries({ queryKey: qk.user.orders() })
      create.mutate()
    },
    onError: (error) => setFormError(errorMessage(error)),
  })

  const applyCoupon = (e?: FormEvent) => {
    e?.preventDefault()
    const value = code.trim()
    if (!value) {
      setCouponError(t('checkout.couponRequired'))
      return
    }
    check.mutate(value)
  }

  const removeCoupon = () => {
    setCoupon(null)
    setCode('')
    setCouponError(undefined)
  }

  const busy = create.isPending || cancelPending.isPending
  const periodLabel = t(`common:periods.${period}`)

  return (
    <Modal.Backdrop
      isOpen
      onOpenChange={(open) => {
        if (!open && !busy) onClose()
      }}
      isDismissable={!busy}
    >
      <Modal.Container placement="auto" size="md" scroll="inside">
        <Modal.Dialog>
          <Modal.CloseTrigger aria-label={t('common:actions.close')} />
          <Modal.Header>
            <Modal.Heading>{t('checkout.title')}</Modal.Heading>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-5">
            <dl className="inset-group bg-fill-quaternary" aria-label={t('checkout.summary')}>
              <Row label={t('checkout.plan')}>
                <span className="font-semibold">{plan.name}</span>
                {isCurrent ? (
                  <span className="ms-2 text-label-secondary">({t('checkout.renewal')})</span>
                ) : null}
              </Row>
              <Row label={t('checkout.period')}>{periodLabel}</Row>
              <Row label={t('checkout.price')}>
                <SiteMoney cents={price} />
              </Row>
            </dl>

            {period !== 'reset_traffic' || price > 0 ? (
              <div className="flex flex-col gap-2">
                {coupon ? (
                  <div className="flex items-center justify-between gap-3 rounded-[12px] bg-fill-quaternary px-4 py-3">
                    <div className="flex min-w-0 items-center gap-2">
                      <TicketPercent className="size-5 shrink-0 text-system-green" aria-hidden="true" />
                      <div className="min-w-0">
                        <p className="truncate text-subhead font-semibold text-label">
                          {coupon.name || coupon.code}
                        </p>
                        <p className="text-footnote text-label-secondary">
                          {coupon.type === 2
                            ? t('checkout.couponPercent', { value: coupon.value })
                            : t('checkout.couponFixed')}
                        </p>
                      </div>
                    </div>
                    <Button
                      size="sm"
                      variant="ghost"
                      isIconOnly
                      aria-label={t('checkout.removeCoupon')}
                      onPress={removeCoupon}
                      isDisabled={busy}
                    >
                      <X className="size-4" aria-hidden="true" />
                    </Button>
                  </div>
                ) : (
                  <form onSubmit={applyCoupon} noValidate>
                    <TextField
                      name="coupon_code"
                      value={code}
                      onChange={(value) => {
                        setCode(value)
                        setCouponError(undefined)
                      }}
                      isInvalid={Boolean(couponError)}
                      validationBehavior="aria"
                      fullWidth
                    >
                      <Label>{t('checkout.coupon')}</Label>
                      <InputGroup fullWidth>
                        <InputGroup.Input
                          placeholder={t('checkout.couponPlaceholder')}
                          autoComplete="off"
                          className="w-full"
                        />
                        <InputGroup.Suffix className="pe-1">
                          <Button type="submit" size="sm" variant="secondary" isPending={check.isPending}>
                            {t('checkout.apply')}
                          </Button>
                        </InputGroup.Suffix>
                      </InputGroup>
                      <FieldError>{couponError}</FieldError>
                    </TextField>
                  </form>
                )}
              </div>
            ) : null}

            <dl className="flex flex-col gap-2 text-subhead" aria-label={t('checkout.breakdown')}>
              <Line label={t('checkout.price')}>
                <SiteMoney cents={estimate.price} />
              </Line>
              {estimate.userDiscount > 0 ? (
                <Line label={t('checkout.userDiscount', { percent: user?.discount ?? 0 })}>
                  −<SiteMoney cents={estimate.userDiscount} />
                </Line>
              ) : null}
              {estimate.couponDiscount > 0 ? (
                <Line label={t('checkout.couponDiscount')}>
                  −<SiteMoney cents={estimate.couponDiscount} />
                </Line>
              ) : null}
              {estimate.balanceUsed > 0 ? (
                <Line label={t('checkout.balance')}>
                  −<SiteMoney cents={estimate.balanceUsed} />
                </Line>
              ) : null}
              <div className="mt-1 flex items-baseline justify-between gap-4 border-t-[0.5px] border-hairline pt-3">
                <dt className="text-headline text-label">{t('checkout.payable')}</dt>
                <dd className="text-title2 text-label">
                  <SiteMoney cents={estimate.payable} />
                </dd>
              </div>
            </dl>

            <p className="text-footnote text-label-secondary">
              {user && user.balance > 0
                ? `${t('checkout.balanceNote', { balance: money(user.balance) })} `
                : null}
              {user?.plan_id && !isCurrent && period !== 'reset_traffic'
                ? t('checkout.upgradeNote')
                : t('checkout.estimateNote')}
            </p>

            {pending !== null ? (
              <Alert status="warning">
                <Alert.Indicator />
                <Alert.Content>
                  <Alert.Title>{t('checkout.pendingTitle')}</Alert.Title>
                  <Alert.Description>{t('checkout.pendingBody')}</Alert.Description>
                  <div className="mt-3 flex flex-wrap gap-2">
                    {pending ? (
                      <Link
                        to="/orders/$tradeNo"
                        params={{ tradeNo: pending }}
                        className={buttonClass({ size: 'sm', variant: 'primary' })}
                        onClick={onClose}
                      >
                        {t('checkout.goToPending')}
                      </Link>
                    ) : (
                      <Link
                        to="/orders"
                        search={{ status: 0 }}
                        className={buttonClass({ size: 'sm', variant: 'primary' })}
                        onClick={onClose}
                      >
                        {t('checkout.viewOrders')}
                      </Link>
                    )}
                    {pending ? (
                      <Button
                        size="sm"
                        variant="danger-soft"
                        isPending={cancelPending.isPending}
                        onPress={() => cancelPending.mutate(pending)}
                      >
                        {t('checkout.cancelPending')}
                      </Button>
                    ) : null}
                  </div>
                </Alert.Content>
              </Alert>
            ) : null}

            <FormError message={formError} />
          </Modal.Body>
          <Modal.Footer>
            <Button variant="tertiary" onPress={onClose} isDisabled={busy}>
              {t('common:actions.cancel')}
            </Button>
            <Button
              variant="primary"
              isPending={create.isPending}
              isDisabled={busy || pending !== null}
              onPress={() => {
                setFormError(undefined)
                create.mutate()
              }}
            >
              {t('checkout.submit')}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}

function Row({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-center justify-between gap-4 px-4 py-3 text-subhead">
      <dt className="text-label-secondary">{label}</dt>
      <dd className="text-end text-label">{children}</dd>
    </div>
  )
}

function Line({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex items-baseline justify-between gap-4">
      <dt className="text-label-secondary">{label}</dt>
      <dd className="tabular text-label">{children}</dd>
    </div>
  )
}
