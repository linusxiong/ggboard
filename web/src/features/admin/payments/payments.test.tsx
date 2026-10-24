import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from 'vitest'
import { loadNamespaces } from '@/i18n'
import type { Payment, PaymentGateway } from '@/lib/api/types'
import { useUiStore } from '@/stores/ui'
import { defaultConfig, fieldSuggestions, notifyUrlFor, validateConfig } from './gateways'
import { PaymentSheet } from './PaymentSheet'

const gateways: PaymentGateway[] = [
  {
    name: 'epay',
    label: 'EPay',
    fields: [
      {
        key: 'url',
        label: 'Gateway URL',
        description: 'Base URL without /submit.php',
        type: 'text',
        required: true,
      },
      { key: 'pid', label: 'Merchant ID', description: '', type: 'text', required: true },
      { key: 'key', label: 'Merchant key', description: '', type: 'password', required: true },
      {
        key: 'type',
        label: 'Type',
        description: '',
        type: 'select',
        options: ['alipay', 'wxpay'],
        required: false,
      },
    ],
  },
  {
    name: 'stripe',
    label: 'Stripe',
    fields: [
      { key: 'secret_key', label: 'Secret key', description: '', type: 'password', required: true },
      {
        key: 'exchange_rate',
        label: 'Exchange rate',
        description: '',
        type: 'number',
        default: '1',
        required: false,
      },
      { key: 'notes', label: 'Notes', description: 'Free text', type: 'textarea', required: false },
    ],
  },
]

function json(data: unknown, status = 200) {
  return new Response(JSON.stringify(status < 300 ? { data } : data), {
    status,
    headers: { 'Content-Type': 'application/json' },
  })
}

const created: Payment = {
  id: 9,
  uuid: 'a'.repeat(32),
  gateway: 'epay',
  name: 'EPay',
  icon: '',
  config: {},
  notify_domain: '',
  handling_fee_fixed: 0,
  handling_fee_percent: 0,
  enabled: true,
  sort: 1,
  notify_url: 'https://panel.example.com/api/v1/guest/payment/notify/epay/aaaa',
  created_at: '2026-10-22T00:00:00Z',
  updated_at: '2026-10-22T00:00:00Z',
}

const urlOf = (input: RequestInfo | URL) => (input instanceof Request ? input.url : input.toString())
const bodyOf = (init?: RequestInit) => (typeof init?.body === 'string' ? init.body : '')

describe('gateway helpers', () => {
  it('derives defaults, suggestions, validation and the notify URL', () => {
    expect(defaultConfig(gateways[1])).toEqual({ secret_key: '', exchange_rate: '1', notes: '' })
    expect(fieldSuggestions('epay', gateways[0]!.fields[3]!)).toEqual(['alipay', 'wxpay'])
    expect(
      fieldSuggestions('epusdt', { key: 'currency_type', label: '', description: '', type: 'text' }),
    ).toContain('usdt.trc20')
    expect(validateConfig(gateways[1]!.fields, { secret_key: '', exchange_rate: 'abc' })).toEqual({
      secret_key: 'required',
      exchange_rate: 'number',
    })
    expect(
      notifyUrlFor(
        { gateway: 'stripe', uuid: 'u1', notify_domain: 'https://pay.example.com/' },
        'https://x.example.com',
      ),
    ).toBe('https://pay.example.com/api/v1/guest/payment/notify/stripe/u1')
  })
})

describe('PaymentSheet (dynamic gateway form)', () => {
  const fetchMock = vi.fn<typeof fetch>()
  let posted: Record<string, unknown> | null = null

  beforeAll(async () => {
    useUiStore.setState({ locale: 'en' })
    await loadNamespaces(['admin-payments', 'admin-settings', 'common', 'core'], 'en')
  })
  beforeEach(() => {
    posted = null
    fetchMock.mockImplementation((input, init) => {
      const url = urlOf(input)
      if (url.endsWith('/admin/payments') && init?.method === 'POST') {
        posted = JSON.parse(bodyOf(init)) as Record<string, unknown>
        return Promise.resolve(json(created))
      }
      return Promise.resolve(json({ code: 'not_found', message: 'nope' }, 404))
    })
    vi.stubGlobal('fetch', fetchMock)
  })
  afterEach(() => vi.unstubAllGlobals())

  function renderSheet(defaultGateway: string) {
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    return render(
      <QueryClientProvider client={qc}>
        <PaymentSheet
          isOpen
          onOpenChange={() => undefined}
          gateways={gateways}
          currency="CNY"
          appUrl="https://panel.example.com"
          defaultGateway={defaultGateway}
        />
      </QueryClientProvider>,
    )
  }

  it('renders the config fields from `fields` (types, required marks, descriptions)', async () => {
    renderSheet('stripe')
    expect(await screen.findByRole('dialog')).toBeInTheDocument()
    const secret = screen.getByLabelText(/Secret key/)
    expect(secret).toHaveAttribute('type', 'password')
    expect(secret).toBeRequired()
    expect(screen.getByRole('textbox', { name: 'Exchange rate' })).toHaveValue('1')
    const notes = screen.getByRole('textbox', { name: 'Notes' })
    expect(notes.tagName).toBe('TEXTAREA')
    expect(screen.getByText('Free text')).toBeInTheDocument()
    // per-gateway setup guide, collapsed until opened
    expect(screen.getByRole('button', { name: 'Setting up Stripe' })).toHaveAttribute(
      'aria-expanded',
      'false',
    )
  })

  it('validates required config and sends the config map', async () => {
    const user = userEvent.setup()
    renderSheet('epay')
    await screen.findByRole('dialog')
    expect(screen.getByRole('combobox', { name: 'Type' })).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: 'Create' }))
    expect(await screen.findAllByText('Required')).toHaveLength(3)
    expect(fetchMock).not.toHaveBeenCalled()

    await user.type(screen.getByRole('textbox', { name: 'Gateway URL' }), 'https://pay.example.com')
    await user.type(screen.getByRole('textbox', { name: 'Merchant ID' }), '1001')
    await user.type(screen.getByLabelText(/Merchant key/), 'k3y')
    await user.type(screen.getByRole('combobox', { name: 'Type' }), 'usdt.tron')
    await user.keyboard('{Escape}') // close the suggestions popover (it makes the rest inert)
    await user.click(screen.getByRole('button', { name: 'Create' }))

    await waitFor(() => expect(posted).not.toBeNull())
    expect(posted).toMatchObject({
      gateway: 'epay',
      name: 'EPay',
      enabled: true,
      handling_fee_fixed: 0,
      config: { url: 'https://pay.example.com', pid: '1001', key: 'k3y', type: 'usdt.tron' },
    })
    // After creating, the sheet shows the notify URL to paste into the provider.
    expect(await screen.findByDisplayValue(created.notify_url!)).toBeInTheDocument()
  })
})
