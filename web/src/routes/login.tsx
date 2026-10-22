import { createFileRoute, Link } from '@tanstack/react-router'
import { useEffect, useRef, useState } from 'react'
import type { CaptchaHandle } from '@/components/Captcha'
import { LazyCaptcha } from '@/components/LazyCaptcha'
import { FormError } from '@/components/FormError'
import { AuthLayout } from '@/components/layout/AuthLayout'
import { Spinner } from '@/components/LoadingPage'
import { NativeButton } from '@/components/native/NativeButton'
import { NativeField } from '@/components/native/NativeField'
import { useFinishAuth } from '@/hooks/useFinishAuth'
import { useLiteForm } from '@/hooks/useLiteForm'
import { loadNamespaces, useT } from '@/i18n'
import { login, loginWithToken } from '@/lib/api/passport'
import { getGuestConfig, withDeadline } from '@/lib/publicData'
import { splitAuthError } from '@/lib/authErrors'
import { redirectIfAuthenticated } from '@/lib/guards'
import { validateEmail } from '@/lib/validators'

interface LoginSearch {
  /** Where to go after signing in (same-origin path). */
  redirect?: string
  /** One-time login token from an admin "login as" URL (`/login?token=…`). */
  token?: string
}

export const Route = createFileRoute('/login')({
  validateSearch: (search: Record<string, unknown>): LoginSearch => ({
    ...(typeof search.redirect === 'string' ? { redirect: search.redirect } : {}),
    ...(typeof search.token === 'string' ? { token: search.token } : {}),
  }),
  beforeLoad: ({ search }) => redirectIfAuthenticated({ search }),
  loader: async () => {
    const [config] = await Promise.all([withDeadline(getGuestConfig()), loadNamespaces(['auth'])])
    return { config }
  },
  component: LoginPage,
})

function LoginPage() {
  const t = useT(['auth', 'core'])
  const search = Route.useSearch()
  const { config } = Route.useLoaderData()
  const finish = useFinishAuth(search.redirect)
  const [formError, setFormError] = useState<string | undefined>()
  const [captchaToken, setCaptchaToken] = useState<string | null>(null)
  const captchaRef = useRef<CaptchaHandle>(null)
  const captcha = config?.captcha
  const captchaEnabled = Boolean(captcha && captcha.provider !== 'none' && captcha.site_key)
  const [tokenState, setTokenState] = useState<'idle' | 'pending' | 'failed'>(search.token ? 'pending' : 'idle')

  // Admin "login as" one-time link. Tokens are single use: guard against StrictMode double effects.
  const token = search.token
  const usedToken = useRef<string | null>(null)
  useEffect(() => {
    if (!token || usedToken.current === token) return
    usedToken.current = token
    loginWithToken(token)
      .then((result) => finish(result, t('messages.loginSuccess')))
      .catch(() => setTokenState('failed'))
  }, [token, t, finish])

  const form = useLiteForm(
    { email: '', password: '' },
    {
      email: (v) => validateEmail(v),
      password: (v) => (v ? undefined : t('validation.passwordRequired')),
    },
  )

  const onSubmit = form.handleSubmit(async (values) => {
    setFormError(undefined)
    if (captchaEnabled && !captchaToken) {
      setFormError(t('captcha.required'))
      return
    }
    try {
      const result = await login({
        email: values.email.trim(),
        password: values.password,
        ...(captchaToken ? { captcha: captchaToken } : {}),
      })
      finish(result, t('messages.loginSuccess'))
    } catch (error) {
      const { fields, message } = splitAuthError(error)
      for (const [name, msg] of Object.entries(fields)) if (name in values) form.setError(name as 'email', msg)
      setFormError(message)
      captchaRef.current?.reset()
    }
  })

  return (
    <AuthLayout
      title={t('login.title')}
      subtitle={t('login.subtitle')}
      footer={
        config?.register_enabled === false ? null : (
          <>
            {t('login.noAccount')}{' '}
            <Link
              to="/register"
              search={search.redirect ? { redirect: search.redirect } : {}}
              className="font-semibold text-label-blue hover:underline"
            >
              {t('login.register')}
            </Link>
          </>
        )
      }
    >
      {tokenState === 'pending' ? (
        <div className="flex items-center justify-center gap-3 py-6 text-subhead text-label-secondary" role="status">
          <Spinner />
          {t('login.tokenSigningIn')}
        </div>
      ) : (
        <form noValidate className="flex flex-col gap-4" onSubmit={(e) => void onSubmit(e)}>
          <FormError message={tokenState === 'failed' ? t('login.tokenFailed') : formError} />
          <NativeField
            {...form.field('email')}
            label={t('fields.email')}
            type="email"
            inputMode="email"
            autoComplete="username"
            placeholder={t('fields.emailPlaceholder')}
            required
          />
          <NativeField {...form.field('password')} label={t('fields.password')} type="password" autoComplete="current-password" required />
          <div className="-mt-1 flex justify-end">
            <Link to="/forgot" className="text-footnote text-label-blue hover:underline">
              {t('login.forgot')}
            </Link>
          </div>
          {captchaEnabled && captcha ? (
            <LazyCaptcha ref={captchaRef} provider={captcha.provider} siteKey={captcha.site_key} onToken={setCaptchaToken} />
          ) : null}
          <NativeButton type="submit" size="lg" fullWidth pending={form.submitting}>
            {t('login.submit')}
          </NativeButton>
        </form>
      )}
    </AuthLayout>
  )
}
