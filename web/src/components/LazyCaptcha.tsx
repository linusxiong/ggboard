import { lazy, type Ref, Suspense } from 'react'
import type { CaptchaProvider } from '@/lib/api/types'
import type { CaptchaHandle } from './Captcha'

const Captcha = lazy(() => import('./Captcha').then((m) => ({ default: m.Captcha })))

interface LazyCaptchaProps {
  provider: CaptchaProvider
  siteKey: string
  onToken: (token: string | null) => void
  ref?: Ref<CaptchaHandle>
}

/** `<Captcha>` loaded on demand: pages without captcha never download its code. */
export function LazyCaptcha(props: LazyCaptchaProps) {
  if (props.provider === 'none' || !props.siteKey) return null
  return (
    <Suspense fallback={<div className="min-h-[88px]" aria-hidden="true" />}>
      <Captcha {...props} />
    </Suspense>
  )
}
