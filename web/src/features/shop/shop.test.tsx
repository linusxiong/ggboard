import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it } from 'vitest'
import { bodyOf, callsTo, makePlan, mockApi, renderApp, reply, resetApp, testUser } from './testing'

const plans = [
  makePlan({ id: 1, name: 'Lite', prices: { monthly: 1500, quarterly: 4000 } }),
  makePlan({ id: 2, name: 'Pro', prices: { monthly: 3000, reset_traffic: 500 } }),
  makePlan({ id: 3, name: 'Holiday', capacity_limit: 0, prices: { onetime: 9900 } }),
]
const user = { ...testUser, plan_id: 2, plan: { id: 2, name: 'Pro' }, balance: 1000 }

describe('shop', () => {
  afterEach(resetApp)

  it('checks a coupon, creates the order and navigates to it', async () => {
    const fetchMock = mockApi(
      {
        'GET /user/plans': () => plans,
        'POST /user/coupons/check': ({ body }) =>
          (body as { code: string }).code === 'SAVE10'
            ? { id: 1, code: 'SAVE10', name: '九折', type: 2, value: 10 }
            : reply(400, { code: 'validation_failed', message: 'bad', errors: { code: ['优惠码无效'] } }),
        'POST /user/orders': () => reply(201, { trade_no: 'T20261022' }),
        'GET /user/orders/T20261022': () => ({
          id: 9,
          trade_no: 'T20261022',
          user_id: 1,
          plan_id: 1,
          plan: { id: 1, name: 'Lite' },
          type: 3,
          period: 'quarterly',
          status: 0,
          commission_status: 0,
          total_amount: 2600,
          handling_amount: 0,
          discount_amount: 400,
          surplus_amount: 0,
          refund_amount: 0,
          balance_amount: 1000,
          payment_id: null,
          payment: null,
          callback_no: null,
          paid_at: null,
          created_at: new Date().toISOString(),
          updated_at: new Date().toISOString(),
        }),
        'GET /user/orders/T20261022/status': () => ({ status: 0 }),
        'GET /user/payment-methods': () => [],
      },
      user,
    )
    const { router } = renderApp('/shop', user)
    const u = userEvent.setup()

    expect(
      await screen.findByRole('heading', { level: 1, name: '购买套餐' }, { timeout: 4000 }),
    ).toBeInTheDocument()
    // Current plan, sold-out plan and the reset-traffic add-on.
    expect(screen.getByText('当前套餐')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: '立即购买 Holiday' })).toBeDisabled()
    expect(screen.getByRole('heading', { name: '流量重置包' })).toBeInTheDocument()

    // Pick the quarterly price on Lite → per-month equivalent and saving.
    await u.click(screen.getByRole('radio', { name: /季付/ }))
    expect(screen.getByText('约 ¥13.33/月')).toBeInTheDocument()
    expect(screen.getByText('省 11%')).toBeInTheDocument()
    await u.click(screen.getByRole('button', { name: '立即购买 Lite' }))

    const dialog = await screen.findByRole('dialog', {}, { timeout: 4000 })
    expect(within(dialog).getByText('季付')).toBeInTheDocument()

    // Invalid coupon → field error; valid one → discount line.
    await u.type(within(dialog).getByLabelText('优惠码'), 'NOPE')
    await u.click(within(dialog).getByRole('button', { name: '使用' }))
    expect(await within(dialog).findByText('优惠码无效')).toBeInTheDocument()
    await u.clear(within(dialog).getByLabelText('优惠码'))
    await u.type(within(dialog).getByLabelText('优惠码'), 'SAVE10')
    await u.click(within(dialog).getByRole('button', { name: '使用' }))
    expect(await within(dialog).findByText('九折')).toBeInTheDocument()
    // 4000 − 10 % = 3600, − balance 1000 = 2600
    const dd = (text: string) => (_: string, el: Element | null) =>
      el?.tagName === 'DD' && el.textContent === text
    expect(within(dialog).getByText(dd('−¥4.00'))).toBeInTheDocument()
    expect(within(dialog).getByText(dd('−¥10.00'))).toBeInTheDocument()
    expect(within(dialog).getByText(dd('¥26.00'))).toBeInTheDocument()

    await u.click(within(dialog).getByRole('button', { name: '提交订单' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/orders/T20261022'))
    expect(bodyOf(callsTo(fetchMock, 'POST', '/user/orders')[0])).toEqual({
      plan_id: 1,
      period: 'quarterly',
      coupon_code: 'SAVE10',
    })
    expect(
      await screen.findByRole('heading', { level: 1, name: '订单详情' }, { timeout: 4000 }),
    ).toBeInTheDocument()
  })

  it('offers to open or cancel the pending order on 409', async () => {
    let pendingCancelled = false
    mockApi(
      {
        'GET /user/plans': () => plans,
        'POST /user/orders': () =>
          pendingCancelled
            ? reply(201, { trade_no: 'NEW1' })
            : reply(409, { code: 'conflict', message: 'pending', errors: { trade_no: ['OLD1'] } }),
        'POST /user/orders/OLD1/cancel': () => {
          pendingCancelled = true
          return true
        },
        'GET /user/orders/NEW1': () => reply(404, { code: 'not_found', message: 'gone' }),
      },
      user,
    )
    const { router } = renderApp('/shop', user)
    const u = userEvent.setup()

    await u.click(await screen.findByRole('button', { name: '立即购买 Lite' }, { timeout: 4000 }))
    const dialog = await screen.findByRole('dialog', {}, { timeout: 4000 })
    await u.click(within(dialog).getByRole('button', { name: '提交订单' }))

    expect(await within(dialog).findByText('你有一个未支付的订单')).toBeInTheDocument()
    expect(within(dialog).getByRole('link', { name: '前往该订单' })).toHaveAttribute('href', '/orders/OLD1')
    await u.click(within(dialog).getByRole('button', { name: '取消旧订单并继续' }))
    await waitFor(() => expect(router.state.location.pathname).toBe('/orders/NEW1'))
  })
})
