import { screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, describe, expect, it, vi } from 'vitest'
import type { Order } from '@/lib/api/types'
import { bodyOf, callsTo, mockApi, renderApp, resetApp, testUser } from '@/features/shop/testing'
import { redirectToGateway } from './redirect'

vi.mock('./redirect', () => ({ redirectToGateway: vi.fn(() => true) }))

function order(overrides: Partial<Order> = {}): Order {
  return {
    id: 1,
    trade_no: 'T1',
    user_id: 1,
    plan_id: 2,
    plan: { id: 2, name: 'Pro' },
    type: 1,
    period: 'monthly',
    status: 0,
    commission_status: 0,
    total_amount: 3000,
    handling_amount: 0,
    discount_amount: 0,
    surplus_amount: 0,
    refund_amount: 0,
    balance_amount: 0,
    payment_id: null,
    payment: null,
    callback_no: null,
    paid_at: null,
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  }
}

const methods = [
  { id: 1, name: '支付宝', icon: '', handling_fee_fixed: 0, handling_fee_percent: 0 },
  { id: 2, name: 'Stripe', icon: '', handling_fee_fixed: 0, handling_fee_percent: 10 },
]

describe('order page checkout', () => {
  afterEach(() => {
    resetApp()
    vi.mocked(redirectToGateway).mockClear()
  })

  it('redirects to the gateway for a `url` result (with the selected method and fee)', async () => {
    const fetchMock = mockApi({
      'GET /user/orders/T1': () => order(),
      'GET /user/orders/T1/status': () => ({ status: 0 }),
      'GET /user/payment-methods': () => methods,
      'POST /user/orders/T1/checkout': () => ({ type: 'url', data: 'https://pay.example.com/session/1' }),
    })
    renderApp('/orders/T1')
    const u = userEvent.setup()

    expect(await screen.findByRole('heading', { name: '等待支付' }, { timeout: 4000 })).toBeInTheDocument()
    expect(screen.getByText(/请在以下时间内完成支付/)).toBeInTheDocument()
    await u.click(await screen.findByRole('radio', { name: /Stripe/ }))
    expect(screen.getByText('应付金额（含手续费）')).toBeInTheDocument()
    await u.click(screen.getByRole('button', { name: '去支付' }))

    await waitFor(() => expect(redirectToGateway).toHaveBeenCalledWith('https://pay.example.com/session/1'))
    expect(bodyOf(callsTo(fetchMock, 'POST', '/user/orders/T1/checkout')[0])).toEqual({ payment_id: 2 })
    expect(await screen.findByText('正在前往支付页面…')).toBeInTheDocument()
  })

  it('renders a QR code and the "I have paid" check for a `qrcode` result', async () => {
    let paid = false
    mockApi({
      'GET /user/orders/T1': () => order(paid ? { status: 3, paid_at: new Date().toISOString() } : {}),
      'GET /user/orders/T1/status': () => ({ status: paid ? 3 : 0 }),
      'GET /user/payment-methods': () => methods,
      'POST /user/orders/T1/checkout': () => ({ type: 'qrcode', data: 'https://qr.example.com/abc' }),
    })
    renderApp('/orders/T1')
    const u = userEvent.setup()

    await u.click(await screen.findByRole('button', { name: '去支付' }, { timeout: 4000 }))
    expect(await screen.findByRole('img', { name: '支付宝支付二维码' })).toBeInTheDocument()
    expect(screen.getByText(/请使用支付宝扫描二维码完成支付/)).toBeInTheDocument()
    expect(redirectToGateway).not.toHaveBeenCalled()

    paid = true
    await u.click(screen.getByRole('button', { name: '我已支付' }))
    expect(await screen.findByRole('heading', { name: '支付成功' }, { timeout: 4000 })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: '查看订阅' })).toHaveAttribute('href', '/subscription')
  })

  it('activates a fully covered order without a payment method (`paid`)', async () => {
    let paid = false
    const fetchMock = mockApi({
      'GET /user/orders/T1': () =>
        order({ total_amount: 0, balance_amount: 3000, ...(paid ? { status: 3 } : {}) }),
      'GET /user/orders/T1/status': () => ({ status: paid ? 3 : 0 }),
      'POST /user/orders/T1/checkout': () => {
        paid = true
        return { type: 'paid', data: '' }
      },
    })
    renderApp('/orders/T1')
    const u = userEvent.setup()

    expect(
      await screen.findByText('余额与优惠已覆盖全部金额，无需额外付款。', {}, { timeout: 4000 }),
    ).toBeInTheDocument()
    await u.click(screen.getByRole('button', { name: '确认开通' }))
    expect(await screen.findByRole('heading', { name: '支付成功' }, { timeout: 4000 })).toBeInTheDocument()
    expect(bodyOf(callsTo(fetchMock, 'POST', '/user/orders/T1/checkout')[0])).toEqual({})
    expect(callsTo(fetchMock, 'GET', '/user/payment-methods')).toHaveLength(0)
  })

  it('shows the amounts breakdown and lets the user cancel a pending order', async () => {
    let cancelled = false
    mockApi({
      'GET /user/orders/T1': () =>
        order({
          discount_amount: 500,
          balance_amount: 1000,
          total_amount: 1500,
          ...(cancelled ? { status: 2 } : {}),
        }),
      'GET /user/orders/T1/status': () => ({ status: cancelled ? 2 : 0 }),
      'GET /user/payment-methods': () => methods,
      'POST /user/orders/T1/cancel': () => {
        cancelled = true
        return true
      },
    })
    renderApp('/orders/T1', { ...testUser })
    const u = userEvent.setup()

    expect(await screen.findByText('套餐价格', {}, { timeout: 4000 })).toBeInTheDocument()
    // price = total + discount + balance = 3000
    expect(screen.getAllByText('¥30.00').length).toBeGreaterThan(0)
    await u.click(screen.getByRole('button', { name: '取消订单' }))
    const dialog = await screen.findByRole('alertdialog', {}, { timeout: 4000 })
    expect(within(dialog).getByText('取消这个订单？')).toBeInTheDocument()
    await u.click(within(dialog).getByRole('button', { name: '取消订单' }))
    expect(await screen.findByRole('heading', { name: '订单已取消' }, { timeout: 4000 })).toBeInTheDocument()
  })
})
