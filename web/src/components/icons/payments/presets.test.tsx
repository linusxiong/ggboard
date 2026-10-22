import { render } from '@testing-library/react'
import { describe, expect, it } from 'vitest'
import {
  findPaymentIconPreset,
  isValidPaymentIcon,
  PAYMENT_ICON_IDS,
  PAYMENT_ICON_PRESETS,
  parsePaymentIcon,
  PaymentIcon,
  presetForGateway,
  presetIconValue,
} from '.'

describe('payment icon presets', () => {
  it('registers every preset id once, in order, with names and a 24 × 24 decorative SVG', () => {
    const ids = [
      'alipay',
      'wechat',
      'stripe',
      'card',
      'paypal',
      'crypto',
      'btc',
      'usdt',
      'eth',
      'bank',
      'qrcode',
      'wallet',
      'epay',
      'epusdt',
      'btcpay',
      'coinbase',
    ]
    expect(PAYMENT_ICON_PRESETS.map((p) => p.id)).toEqual(ids)
    expect([...PAYMENT_ICON_IDS]).toEqual(ids)
    for (const preset of PAYMENT_ICON_PRESETS) {
      expect(preset.name['zh-CN']).toBeTruthy()
      expect(preset.name.en).toBeTruthy()
      expect(findPaymentIconPreset(preset.id)).toBe(preset)
      const { container, unmount } = render(<preset.Component width={32} height={32} />)
      const svg = container.querySelector('svg')
      expect(svg).toHaveAttribute('viewBox', '0 0 24 24')
      expect(svg).toHaveAttribute('aria-hidden', 'true')
      expect(svg).toHaveAttribute('width', '32')
      unmount()
    }
  })

  it('maps gateways to their default preset', () => {
    expect(presetForGateway('alipay_f2f')?.id).toBe('alipay')
    expect(presetForGateway('epusdt')?.id).toBe('usdt')
    expect(presetForGateway('stripe')?.id).toBe('stripe')
    expect(presetForGateway('epay')?.id).toBe('epay')
    expect(presetForGateway('coinbase')?.id).toBe('coinbase')
    expect(presetForGateway('btcpay')?.id).toBe('btcpay')
    expect(presetForGateway('unknown')).toBeUndefined()
    // Each gateway has at most one default.
    const defaults = PAYMENT_ICON_PRESETS.flatMap((p) => p.defaultFor ?? [])
    expect(new Set(defaults).size).toBe(defaults.length)
  })

  it('parses Payment.icon values', () => {
    expect(presetIconValue('alipay')).toBe('preset:alipay')
    expect(parsePaymentIcon('preset:alipay')).toMatchObject({ kind: 'preset', id: 'alipay' })
    expect(parsePaymentIcon('preset:future-icon')).toEqual({
      kind: 'preset',
      id: 'future-icon',
      preset: undefined,
    })
    expect(parsePaymentIcon('https://cdn.example.com/a.svg')).toEqual({
      kind: 'url',
      src: 'https://cdn.example.com/a.svg',
    })
    expect(parsePaymentIcon('/pay.png').kind).toBe('url')
    expect(parsePaymentIcon('//cdn.example.com/a.png').kind).toBe('url')
    expect(parsePaymentIcon('data:image/png;base64,AAAA').kind).toBe('url')
    expect(parsePaymentIcon('💳')).toEqual({ kind: 'emoji', text: '💳' })
    expect(parsePaymentIcon('👨‍👩‍👧').kind).toBe('emoji')
    for (const v of [
      '',
      null,
      undefined,
      '  ',
      'alipay',
      'alipay.svg',
      'preset:Bad',
      '💳💳',
      'javascript:x',
    ]) {
      expect(parsePaymentIcon(v).kind).toBe('none')
    }
  })

  it('validates icons like the backend', () => {
    for (const ok of [
      '',
      'preset:alipay',
      'preset:x_1-y',
      'https://a.example.com/i.png',
      '/i.png',
      '💳',
      '¥',
    ]) {
      expect(isValidPaymentIcon(ok)).toBe(true)
    }
    for (const bad of [
      'preset:',
      'preset:A',
      'alipay',
      'ftp://a.example.com/i.png',
      'https://',
      'https://a.example.com/a b.png',
      'javascript:alert(1)',
      `https://a.example.com/${'a'.repeat(255)}`,
    ]) {
      expect(isValidPaymentIcon(bad)).toBe(false)
    }
  })
})

describe('<PaymentIcon>', () => {
  it('renders presets, URLs, emoji and the fallback at a fixed size', () => {
    const { container, rerender } = render(<PaymentIcon icon="preset:wechat" size={20} />)
    expect(container.querySelector('svg[data-payment-icon="wechat"]')).toHaveAttribute('width', '20')

    rerender(<PaymentIcon icon="https://cdn.example.com/x.png" size={32} />)
    const img = container.querySelector('img')
    expect(img).toHaveAttribute('src', 'https://cdn.example.com/x.png')
    expect(img).toHaveAttribute('width', '32')
    expect(img).toHaveAttribute('alt', '')

    rerender(<PaymentIcon icon="💳" />)
    expect(container).toHaveTextContent('💳')

    for (const icon of ['', 'preset:unknown', 'alipay']) {
      rerender(<PaymentIcon icon={icon} />)
      expect(container.querySelector('[data-payment-icon="fallback"]')).toBeInTheDocument()
    }
    // Everything is decorative.
    expect(container.firstElementChild).toHaveAttribute('aria-hidden', 'true')
  })
})
