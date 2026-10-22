import { useEffect, useState } from 'react'
import { useT } from '@/i18n'
import { cx } from '@/lib/cn'

interface QRCodeProps {
  value: string
  /** CSS pixel size (square). */
  size?: number
  /** Accessible description; defaults to "二维码：<value>". */
  label?: string
  className?: string
}

/**
 * QR code rendered as inline SVG. The `qrcode` library (~25 KB) is loaded on demand.
 * Always black-on-white with a quiet zone so every scanner can read it in dark mode too.
 */
export function QRCode({ value, size = 192, label, className }: QRCodeProps) {
  const t = useT('common')
  const [svg, setSvg] = useState<string | null>(null)
  const [failed, setFailed] = useState(false)

  useEffect(() => {
    let alive = true
    import('qrcode')
      .then((mod) =>
        mod.toString(value, { type: 'svg', margin: 2, errorCorrectionLevel: 'M', color: { dark: '#000000', light: '#ffffff' } }),
      )
      .then((markup) => {
        if (alive) {
          setSvg(markup)
          setFailed(false)
        }
      })
      .catch(() => {
        if (alive) setFailed(true)
      })
    return () => {
      alive = false
    }
  }, [value])

  return (
    <div
      role="img"
      aria-label={label ?? t('qr.label', { value })}
      className={cx('overflow-hidden rounded-[12px] bg-white [&>svg]:block [&>svg]:size-full', className)}
      style={{ width: size, height: size }}
      // `qrcode` emits a self-contained <svg> of <path>s; the encoded value never appears as markup.
      dangerouslySetInnerHTML={svg ? { __html: svg } : undefined}
    >
      {svg ? undefined : (
        <div className={cx('size-full', failed ? 'bg-fill-tertiary' : 'animate-pulse bg-fill-quaternary')} />
      )}
    </div>
  )
}
