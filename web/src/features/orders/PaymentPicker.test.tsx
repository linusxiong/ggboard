import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import type { Order, PaymentMethod } from '@/lib/api/types'
import { useUiStore } from '@/stores/ui'
import { mockApi, renderApp, resetApp } from '@/features/shop/testing'
import { PaymentPicker } from './PaymentPicker'

const methods: PaymentMethod[] = [
  { id: 1, name: '支付宝', icon: 'preset:alipay', handling_fee_fixed: 0, handling_fee_percent: 0 },
  {
    id: 2,
    name: 'Logo',
    icon: 'https://cdn.example.com/logo.png',
    handling_fee_fixed: 0,
    handling_fee_percent: 0,
  },
  { id: 3, name: '余额', icon: '💰', handling_fee_fixed: 0, handling_fee_percent: 0 },
  { id: 4, name: '其他', icon: '', handling_fee_fixed: 0, handling_fee_percent: 0 },
  { id: 5, name: '新图标', icon: 'preset:not-yet', handling_fee_fixed: 0, handling_fee_percent: 0 },
]

describe('PaymentPicker icons', () => {
  beforeAll(async () => {
    useUiStore.setState({ locale: 'zh-CN' })
    await loadNamespaces(['orders', 'common', 'core'], 'zh-CN')
  })
  afterEach(() => vi.unstubAllGlobals())

  it('renders preset, URL, emoji and fallback icons at 32 px', () => {
    vi.stubGlobal(
      'fetch',
      vi.fn(() => Promise.resolve(new Response('{}', { status: 404 }))),
    )
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={qc}>
        <PaymentPicker methods={methods} value={1} onChange={() => undefined} total={1000} />
      </QueryClientProvider>,
    )
    const row = (name: string) => screen.getByRole('radio', { name: new RegExp(name) }).closest('label')!

    expect(row('支付宝').querySelector('svg[data-payment-icon="alipay"]')).toHaveAttribute('width', '32')
    expect(row('Logo').querySelector('img')).toHaveAttribute('src', 'https://cdn.example.com/logo.png')
    expect(within(row('余额')).getByText('💰')).toBeInTheDocument()
    expect(row('其他').querySelector('[data-payment-icon="fallback"]')).toBeInTheDocument()
    expect(row('新图标').querySelector('[data-payment-icon="fallback"]')).toBeInTheDocument()
  })
})

describe('order page payment method', () => {
  afterEach(() => resetApp())

  const order = (overrides: Partial<Order> = {}): Order => ({
    id: 1,
    trade_no: 'T9',
    user_id: 1,
    plan_id: 2,
    plan: { id: 2, name: 'Pro' },
    type: 1,
    period: 'monthly',
    status: 3,
    commission_status: 0,
    total_amount: 3000,
    handling_amount: 0,
    discount_amount: 0,
    surplus_amount: 0,
    refund_amount: 0,
    balance_amount: 0,
    payment_id: 1,
    payment: { id: 1, name: '支付宝' },
    callback_no: 'cb',
    paid_at: new Date().toISOString(),
    created_at: new Date().toISOString(),
    updated_at: new Date().toISOString(),
    ...overrides,
  })

  it('shows the paid method with its icon in the summary', async () => {
    mockApi({
      'GET /user/orders/T9': () => order(),
      'GET /user/payment-methods': () => methods,
    })
    renderApp('/orders/T9')
    const summary = await screen.findByRole('complementary', { name: '订单信息' }, { timeout: 4000 })
    const value = (await within(summary).findByText('支付宝', {}, { timeout: 4000 })).parentElement!
    await vi.waitFor(() => expect(value.querySelector('svg[data-payment-icon="alipay"]')).toBeInTheDocument())
  })

  it('shows the chosen method with its icon above the QR code', async () => {
    mockApi({
      'GET /user/orders/T9': () => order({ status: 0, payment_id: null, payment: null, paid_at: null }),
      'GET /user/orders/T9/status': () => ({ status: 0 }),
      'GET /user/payment-methods': () => methods,
      'POST /user/orders/T9/checkout': () => ({ type: 'qrcode', data: 'https://qr.example.com/abc' }),
    })
    renderApp('/orders/T9')
    const u = userEvent.setup()
    await u.click(await screen.findByRole('button', { name: '去支付' }, { timeout: 4000 }))
    const qr = await screen.findByRole('img', { name: '支付宝支付二维码' }, { timeout: 4000 })
    const card = qr.closest('section')!
    expect(card.querySelector('svg[data-payment-icon="alipay"]')).toBeInTheDocument()
    expect(within(card).getByText('支付宝')).toBeInTheDocument()
  })
})
