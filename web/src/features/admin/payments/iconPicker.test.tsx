import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import type { Payment, PaymentGateway } from '@/lib/api/types'
import { useUiStore } from '@/stores/ui'
import { defaultIconPreset } from './gateways'
import { PaymentSheet } from './PaymentSheet'

const gateways: PaymentGateway[] = [
  {
    name: 'epay',
    label: 'EPay',
    fields: [
      { key: 'url', label: 'Gateway URL', description: '', type: 'text', required: true },
      { key: 'pid', label: 'Merchant ID', description: '', type: 'text', required: true },
      { key: 'key', label: 'Merchant key', description: '', type: 'password', required: true },
      { key: 'type', label: 'Type', description: '', type: 'text', required: false },
    ],
  },
  { name: 'alipay_f2f', label: 'Alipay F2F', fields: [] },
  { name: 'stripe', label: 'Stripe', fields: [] },
]

const existing: Payment = {
  id: 3,
  uuid: 'b'.repeat(32),
  gateway: 'epay',
  name: 'EPay',
  icon: 'https://cdn.example.com/old.png',
  config: { url: 'https://pay.example.com', pid: '1', key: 'k' },
  notify_domain: '',
  handling_fee_fixed: 0,
  handling_fee_percent: 0,
  enabled: true,
  sort: 1,
  notify_url: 'https://panel.example.com/api/v1/guest/payment/notify/epay/bbbb',
  created_at: '2026-10-22T00:00:00Z',
  updated_at: '2026-10-22T00:00:00Z',
}

function json(data: unknown, status = 200) {
  return new Response(JSON.stringify(status < 300 ? { data } : data), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}
const urlOf = (input: RequestInfo | URL) => (input instanceof Request ? input.url : input.toString())
const bodyOf = (init?: RequestInit) => (typeof init?.body === 'string' ? init.body : '')

describe('defaultIconPreset', () => {
  it('follows the gateway and what its config says is paid with', () => {
    expect(defaultIconPreset('epay')).toBe('epay')
    expect(defaultIconPreset('epay', { type: 'alipay' })).toBe('alipay')
    expect(defaultIconPreset('epay', { type: 'wxpay' })).toBe('wechat')
    expect(defaultIconPreset('epay', { type: 'usdt.tron' })).toBe('usdt')
    expect(defaultIconPreset('epusdt', { currency_type: 'usdt.trc20' })).toBe('usdt')
    expect(defaultIconPreset('epusdt', { currency_type: 'tron.trx' })).toBe('crypto')
    expect(defaultIconPreset('stripe', { payment_methods: 'auto' })).toBe('stripe')
    expect(defaultIconPreset('stripe', { payment_methods: 'card' })).toBe('card')
    expect(defaultIconPreset('alipay_f2f')).toBe('alipay')
    expect(defaultIconPreset('coinbase')).toBe('coinbase')
    expect(defaultIconPreset('btcpay')).toBe('btcpay')
    expect(defaultIconPreset('custom-gateway')).toBeNull()
  })
})

describe('PaymentSheet icon picker', () => {
  const fetchMock = vi.fn<typeof fetch>()
  let sent: Record<string, unknown> | null = null

  beforeAll(async () => {
    useUiStore.setState({ locale: 'en' })
    await loadNamespaces(['admin-payments', 'admin-settings', 'common', 'core'], 'en')
  })
  beforeEach(() => {
    sent = null
    fetchMock.mockImplementation((input, init) => {
      const url = urlOf(input)
      if (url.includes('/admin/payments') && (init?.method === 'POST' || init?.method === 'PATCH')) {
        sent = JSON.parse(bodyOf(init)) as Record<string, unknown>
        return Promise.resolve(json({ ...existing, ...sent, id: 9 }))
      }
      return Promise.resolve(json({ code: 'not_found', message: 'nope' }, 404))
    })
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => vi.unstubAllGlobals())

  function renderSheet(props: { defaultGateway?: string; payment?: Payment }) {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    return render(
      <QueryClientProvider client={qc}>
        <PaymentSheet
          isOpen
          onOpenChange={() => undefined}
          gateways={gateways}
          currency="CNY"
          appUrl="https://panel.example.com"
          {...props}
        />
      </QueryClientProvider>,
    )
  }

  it('preselects the gateway default and submits the picked preset as `preset:<id>`', async () => {
    const user = userEvent.setup()
    renderSheet({ defaultGateway: 'alipay_f2f' })
    await screen.findByRole('dialog')
    const group = screen.getByRole('radiogroup', { name: 'Icon' })
    expect(group).toBeInTheDocument()
    // 16 presets + "custom URL"
    expect(screen.getAllByRole('radio')).toHaveLength(17)
    expect(screen.getByRole('radio', { name: 'Alipay' })).toBeChecked()

    await user.click(screen.getByRole('radio', { name: 'Coinbase' }))
    expect(screen.getByRole('radio', { name: 'Coinbase' })).toBeChecked()
    await user.click(screen.getByRole('radio', { name: 'Alipay' }))
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => expect(sent).not.toBeNull())
    expect(sent).toMatchObject({ gateway: 'alipay_f2f', icon: 'preset:alipay' })
  })

  it('updates the suggested icon with the EPay type until the admin picks one', async () => {
    const user = userEvent.setup()
    renderSheet({ defaultGateway: 'epay' })
    await screen.findByRole('dialog')
    expect(screen.getByRole('radio', { name: 'EPay' })).toBeChecked()

    const type = screen.getByRole('combobox', { name: 'Type' })
    await user.type(type, 'wxpay')
    await user.keyboard('{Escape}') // close the suggestions popover (it makes the rest inert)
    expect(screen.getByRole('radio', { name: 'WeChat Pay' })).toBeChecked()

    await user.click(screen.getByRole('radio', { name: 'Card' }))
    await user.clear(type)
    await user.type(type, 'alipay')
    await user.keyboard('{Escape}')
    expect(screen.getByRole('radio', { name: 'Card' })).toBeChecked()
  })

  it('keeps custom URLs working: existing value, validation and submit', async () => {
    const user = userEvent.setup()
    renderSheet({ payment: existing })
    await screen.findByRole('dialog')
    expect(screen.getByRole('radio', { name: 'Custom URL' })).toBeChecked()
    const field = screen.getByRole('textbox', { name: 'Icon URL' })
    expect(field).toHaveValue('https://cdn.example.com/old.png')

    await user.clear(field)
    await user.type(field, 'not an icon')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    expect(
      await screen.findByText('Enter an image URL starting with https:// or /, or a single emoji'),
    ).toBeInTheDocument()
    expect(sent).toBeNull()

    await user.clear(field)
    await user.type(field, 'https://cdn.example.com/new.svg')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).not.toBeNull())
    expect(sent).toMatchObject({ icon: 'https://cdn.example.com/new.svg' })
  })

  it('switching from a preset to custom reveals the URL field', async () => {
    const user = userEvent.setup()
    renderSheet({ payment: { ...existing, icon: 'preset:usdt' } })
    await screen.findByRole('dialog')
    expect(screen.getByRole('radio', { name: 'USDT' })).toBeChecked()
    expect(screen.queryByRole('textbox', { name: 'Icon URL' })).not.toBeInTheDocument()

    await user.click(screen.getByRole('radio', { name: 'Custom URL' }))
    await user.type(screen.getByRole('textbox', { name: 'Icon URL' }), '💳')
    await user.click(screen.getByRole('button', { name: 'Save' }))
    await waitFor(() => expect(sent).not.toBeNull())
    expect(sent).toMatchObject({ icon: '💳' })
  })
})
