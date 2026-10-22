/**
 * Built-in payment-method icons (original artwork, GPL-3.0 like the rest of the app).
 *
 * Every mark is drawn by hand on a 24 × 24 grid: a brand-coloured tile (rounded square) or coin
 * (circle) with a white glyph, so it reads the same on light and dark backgrounds and scales
 * cleanly to 20 / 24 / 32 / 40 px. They are simple generic glyphs (a "支", a chat bubble, an "S",
 * ₿, ₮ …), not copies of any company's logo files. Brand colours are part of the artwork (like
 * `Logo.tsx`); the generic marks (card, bank, QR code, wallet, crypto, EPay) use the Apple system
 * colours from the theme instead, so they follow light / dark mode.
 */
import type { ReactNode, SVGProps } from 'react'

export type PaymentGlyphProps = SVGProps<SVGSVGElement>

/** Brand colours used by the artwork below (the only raw colours in this folder). */
const BRAND = {
  alipay: '#1677FF',
  wechat: '#07C160',
  stripe: '#635BFF',
  paypal: '#003087',
  bitcoin: '#F7931A',
  tether: '#26A17B',
  ether: '#627EEA',
  coinbase: '#0052FF',
  white: '#FFFFFF',
} as const

const STROKE = {
  fill: 'none',
  stroke: BRAND.white,
  strokeLinecap: 'round',
  strokeLinejoin: 'round',
} as const

function Svg({ children, ...props }: PaymentGlyphProps & { children: ReactNode }) {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      viewBox="0 0 24 24"
      width="24"
      height="24"
      aria-hidden="true"
      focusable="false"
      {...props}
    >
      {children}
    </svg>
  )
}

/** Rounded-square background: a brand colour, or a theme class (`fill-system-*`). */
function Tile({ color, className }: { color?: string; className?: string }) {
  return <rect width="24" height="24" rx="6" fill={color} className={className} />
}

function Coin({ color, className }: { color?: string; className?: string }) {
  return <circle cx="12" cy="12" r="12" fill={color} className={className} />
}

/** ₿: a "B" with the two ticks above and below (tilted like the usual bitcoin sign when `tilt`). */
function BitcoinSign({ tilt }: { tilt?: boolean }) {
  return (
    <g {...STROKE} strokeWidth={1.8} transform={tilt ? 'rotate(12 12 12)' : undefined}>
      <path d="M9.4 6.8v10.4M8.2 6.8h4.9c1.5 0 2.5 1 2.5 2.5s-1 2.6-2.5 2.6H9.4m0 0h4.2c1.6 0 2.7 1 2.7 2.6s-1.1 2.7-2.7 2.7H8.2" />
      <path d="M10.9 5.1v1.7M13.1 5.1v1.7M10.9 17.2v1.7M13.1 17.2v1.7" />
    </g>
  )
}

/** ₮-style sign: a "T" with a flat ring around the stem. */
function TetherSign() {
  return (
    <g {...STROKE}>
      <path d="M7.2 7.3h9.6M12 7.3v11" strokeWidth={2.1} />
      <ellipse cx="12" cy="11.7" rx="5.2" ry="1.7" strokeWidth={1.4} />
    </g>
  )
}

export function AlipayGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Tile color={BRAND.alipay} />
      {/* 支 = 十 over 又 */}
      <path
        {...STROKE}
        strokeWidth={1.9}
        d="M6.5 7.6h11M12 4.8v6.1M7.6 11h8.6c-1.5 3.7-4.5 6.6-9.4 8.2M9.7 13.4c2 2.8 4.6 4.8 7.9 5.9"
      />
    </Svg>
  )
}

export function WechatGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Tile color={BRAND.wechat} />
      <path
        fill={BRAND.white}
        d="M12 5.6c-3.9 0-7 2.6-7 5.8 0 1.8 1 3.4 2.6 4.4l-.6 2.6 2.9-1.5c.7.2 1.4.3 2.1.3 3.9 0 7-2.6 7-5.8s-3.1-5.8-7-5.8Z"
      />
      <path {...STROKE} stroke={BRAND.wechat} strokeWidth={1.9} d="m9 11.4 2.1 2.1 4-4.1" />
    </Svg>
  )
}

export function StripeGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Tile color={BRAND.stripe} />
      <path
        {...STROKE}
        strokeWidth={2.2}
        d="M15.3 8.3c-.7-.8-1.9-1.3-3.4-1.3-1.9 0-3.2.9-3.2 2.3 0 3.3 6.9 1.8 6.9 5.3 0 1.5-1.4 2.5-3.5 2.5-1.6 0-3-.6-3.8-1.6"
      />
    </Svg>
  )
}

export function CardGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Tile className="fill-system-indigo" />
      <rect x="4.5" y="6.8" width="15" height="10.4" rx="2" {...STROKE} strokeWidth={1.6} />
      <path {...STROKE} strokeLinecap="butt" strokeWidth={2.2} d="M4.6 10.1h14.8" />
      <path {...STROKE} strokeWidth={1.5} d="M7.2 14.3h3.2" />
    </Svg>
  )
}

export function PaypalGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Tile color={BRAND.paypal} />
      <path
        {...STROKE}
        strokeWidth={2}
        d="M8.6 18.3 10.5 6.4h3.9c2.3 0 3.7 1.3 3.4 3.3-.3 2.2-2.1 3.6-4.6 3.6h-2.4l-.8 5"
      />
    </Svg>
  )
}

export function CryptoGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Coin className="fill-system-purple" />
      {/* a block (cube) */}
      <g {...STROKE} strokeWidth={1.6}>
        <path d="M12 5.6 17.5 8.8v6.4L12 18.4l-5.5-3.2V8.8Z" />
        <path d="M6.6 8.9 12 12l5.4-3.1M12 12v6.3" />
      </g>
    </Svg>
  )
}

export function BitcoinGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Coin color={BRAND.bitcoin} />
      <BitcoinSign tilt />
    </Svg>
  )
}

export function UsdtGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Coin color={BRAND.tether} />
      <TetherSign />
    </Svg>
  )
}

export function EthGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Coin color={BRAND.ether} />
      <path fill={BRAND.white} d="M12 4.6 7.4 12.3 12 15l4.6-2.7Z" />
      <path fill={BRAND.white} d="M12 16 7.4 13.3 12 19.4l4.6-6.1Z" />
      <path fill={BRAND.ether} fillOpacity={0.3} d="M12 4.6V15l4.6-2.7ZM12 16v3.4l4.6-6.1Z" />
    </Svg>
  )
}

export function BankGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Tile className="fill-system-teal" />
      <path
        fill={BRAND.white}
        stroke={BRAND.white}
        strokeWidth={1.2}
        strokeLinejoin="round"
        d="M5.4 9.6 12 5.6l6.6 4Z"
      />
      <path {...STROKE} strokeWidth={1.7} d="M7.6 11.7v4.5M10.5 11.7v4.5M13.5 11.7v4.5M16.4 11.7v4.5" />
      <path {...STROKE} strokeWidth={1.9} d="M5.4 18.4h13.2" />
    </Svg>
  )
}

export function QrcodeGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Tile className="fill-system-gray" />
      <path {...STROKE} strokeWidth={1.4} d="M5.5 5.5h5v5h-5zM13.5 5.5h5v5h-5zM5.5 13.5h5v5h-5z" />
      <path
        fill={BRAND.white}
        d="M7 7h2v2H7zM15 7h2v2h-2zM7 15h2v2H7zM13.4 13.4h2.2v2.2h-2.2zM16.6 13.4h1.9v1.6h-1.9zM15.6 15.6h1.4v1.4h-1.4zM13.4 16.8h1.7v1.7h-1.7zM17 17h1.5v1.5H17z"
      />
    </Svg>
  )
}

export function WalletGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Tile className="fill-system-brown" />
      <g {...STROKE} strokeWidth={1.6}>
        <path d="m6.8 8.2 7.8-2.4c.8-.2 1.4.3 1.4 1.1v1.3" strokeWidth={1.5} />
        <rect x="5" y="8.2" width="14" height="10" rx="2.2" />
        <path d="M19 11.4h-3a1.8 1.8 0 0 0 0 3.6h3" />
      </g>
      <circle cx="16.1" cy="13.2" r=".9" fill={BRAND.white} />
    </Svg>
  )
}

export function EpayGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Tile className="fill-system-red" />
      {/* lightning: "易" = quick */}
      <path
        fill={BRAND.white}
        stroke={BRAND.white}
        strokeWidth={0.8}
        strokeLinejoin="round"
        d="M13.4 4.6 7.2 13.2h4.4l-1.1 6.2 6.3-8.7h-4.4Z"
      />
    </Svg>
  )
}

export function EpusdtGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Tile color={BRAND.tether} />
      <TetherSign />
    </Svg>
  )
}

export function BtcpayGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Tile color={BRAND.bitcoin} />
      <BitcoinSign />
    </Svg>
  )
}

export function CoinbaseGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <Tile color={BRAND.coinbase} />
      <path {...STROKE} strokeWidth={2.4} d="M15.8 9.3a4.7 4.7 0 1 0 0 5.4" />
    </Svg>
  )
}

/** Neutral mark for "no icon" / unknown values: a card outline in the secondary label colour. */
export function FallbackGlyph(props: PaymentGlyphProps) {
  return (
    <Svg {...props}>
      <g fill="none" stroke="currentColor" strokeLinecap="round" strokeLinejoin="round">
        <rect x="4.5" y="6.8" width="15" height="10.4" rx="2" strokeWidth={1.6} />
        <path d="M4.6 10.1h14.8" strokeWidth={1.6} />
        <path d="M7.2 14.3h3.2" strokeWidth={1.5} />
      </g>
    </Svg>
  )
}
