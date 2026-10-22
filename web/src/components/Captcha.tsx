/**
 * Captcha widget for Turnstile / reCAPTCHA v2 / hCaptcha. The provider script is injected only
 * when this component mounts with a real provider (`guest/config.captcha.provider != "none"`),
 * so pages without captcha never load third-party code.
 *
 *   const captcha = useRef<CaptchaHandle>(null)
 *   <Captcha ref={captcha} provider={cfg.captcha.provider} siteKey={cfg.captcha.site_key} onToken={setToken} />
 *   captcha.current?.reset()   // tokens are single-use — reset after each submit
 *
 * Note: the backend CSP must allow the provider's origin when captcha is enabled.
 */
import { type Ref, useEffect, useImperativeHandle, useRef, useState } from 'react'
import type { CaptchaProvider } from '@/lib/api/types'
import { useT } from '@/i18n'
import { useResolvedTheme } from '@/lib/theme'

export interface CaptchaHandle {
  reset: () => void
}

interface WidgetApi {
  render: (el: HTMLElement, opts: Record<string, unknown>) => string | number
  reset: (id?: string | number) => void
  remove?: (id?: string | number) => void
}

interface ProviderDef {
  src: (onload: string) => string
  global: string
}

const PROVIDERS: Record<Exclude<CaptchaProvider, 'none'>, ProviderDef> = {
  turnstile: {
    src: (onload) => `https://challenges.cloudflare.com/turnstile/v0/api.js?render=explicit&onload=${onload}`,
    global: 'turnstile',
  },
  // recaptcha.net is reachable from mainland China, unlike google.com.
  recaptcha: {
    src: (onload) => `https://www.recaptcha.net/recaptcha/api.js?render=explicit&onload=${onload}`,
    global: 'grecaptcha',
  },
  hcaptcha: {
    src: (onload) => `https://js.hcaptcha.com/1/api.js?render=explicit&recaptchacompat=off&onload=${onload}`,
    global: 'hcaptcha',
  },
}

const scripts = new Map<string, Promise<WidgetApi>>()

function loadProvider(provider: Exclude<CaptchaProvider, 'none'>): Promise<WidgetApi> {
  const def = PROVIDERS[provider]
  let promise = scripts.get(provider)
  if (promise) return promise
  promise = new Promise<WidgetApi>((resolve, reject) => {
    const callback = `__ggCaptcha_${provider}`
    const w = window as unknown as Record<string, unknown>
    w[callback] = () => resolve(w[def.global] as WidgetApi)
    const script = document.createElement('script')
    script.src = def.src(callback)
    script.async = true
    script.defer = true
    script.onerror = () => {
      scripts.delete(provider)
      reject(new Error(`Failed to load ${provider}`))
    }
    document.head.appendChild(script)
  })
  scripts.set(provider, promise)
  return promise
}

interface CaptchaProps {
  provider: CaptchaProvider
  siteKey: string
  /** Called with a token when solved, `null` when expired/errored/reset. */
  onToken: (token: string | null) => void
  ref?: Ref<CaptchaHandle>
  className?: string
}

export function Captcha({ provider, siteKey, onToken, ref, className }: CaptchaProps) {
  const t = useT('auth')
  const theme = useResolvedTheme()
  const container = useRef<HTMLDivElement>(null)
  const widget = useRef<{ api: WidgetApi; id: string | number } | null>(null)
  const onTokenRef = useRef(onToken)
  const [state, setState] = useState<'loading' | 'ready' | 'error'>('loading')

  useEffect(() => {
    onTokenRef.current = onToken
  }, [onToken])

  useImperativeHandle(ref, () => ({
    reset: () => {
      if (widget.current) widget.current.api.reset(widget.current.id)
      onTokenRef.current(null)
    },
  }))

  useEffect(() => {
    if (provider === 'none' || !siteKey) return
    let cancelled = false
    loadProvider(provider)
      .then((api) => {
        if (cancelled || !container.current) return
        const id = api.render(container.current, {
          sitekey: siteKey,
          theme,
          callback: (token: string) => onTokenRef.current(token),
          'expired-callback': () => onTokenRef.current(null),
          'error-callback': () => onTokenRef.current(null),
        })
        widget.current = { api, id }
        setState('ready')
      })
      .catch(() => {
        if (!cancelled) setState('error')
      })
    return () => {
      cancelled = true
      const current = widget.current
      widget.current = null
      try {
        current?.api.remove?.(current.id)
      } catch {
        /* provider already gone */
      }
    }
  }, [provider, siteKey, theme])

  if (provider === 'none' || !siteKey) return null
  return (
    <div className={className}>
      <p className="mb-1.5 text-footnote font-medium text-label-secondary">{t('captcha.label')}</p>
      <div ref={container} className="min-h-[65px]" />
      {state === 'loading' ? <p className="text-footnote text-label-secondary">{t('captcha.loading')}</p> : null}
      {state === 'error' ? (
        <p role="alert" className="text-footnote text-label-red">
          {t('captcha.failed')}
        </p>
      ) : null}
    </div>
  )
}
