import { createFileRoute, Link } from '@tanstack/react-router'
import { lazy, Suspense, useCallback, useEffect, useRef, useState } from 'react'
import type { CaptchaHandle } from '@/components/Captcha'
import { LazyCaptcha } from '@/components/LazyCaptcha'
import { FormError } from '@/components/FormError'
import { AuthLayout } from '@/components/layout/AuthLayout'
import { Spinner } from '@/components/LoadingPage'
import { NativeButton } from '@/components/native/NativeButton'
import { NativeField } from '@/components/native/NativeField'
import { waitText } from '@/features/security/wait'
import {
  loadWebAuthn,
  supportsWebAuthn,
  supportsWebAuthnAutofill,
  webAuthnFailure,
} from '@/features/security/webauthn'
import { useFinishAuth } from '@/hooks/useFinishAuth'
import { useLiteForm } from '@/hooks/useLiteForm'
import { loadNamespaces, useLocale, useT } from '@/i18n'
import { isApiError } from '@/lib/api/client'
import {
  isMfaChallenge,
  login,
  loginWithToken,
  passkeyLoginBegin,
  passkeyLoginFinish,
} from '@/lib/api/passport'
import type { AuthResult, MfaChallenge } from '@/lib/api/types'
import { splitAuthError } from '@/lib/authErrors'
import { errorMessage } from '@/lib/errors'
import { redirectIfAuthenticated } from '@/lib/guards'
import { onIdle } from '@/lib/motion'
import { getGuestConfig, withDeadline } from '@/lib/publicData'
import { EMAIL_RE, validateEmail } from '@/lib/validators'
import type { MfaRestartReason } from './-login/MfaStep'

const MfaStep = lazy(() => import('./-login/MfaStep'))

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
  const locale = useLocale()
  const search = Route.useSearch()
  const { config } = Route.useLoaderData()
  const finishAuth = useFinishAuth(search.redirect)
  const [formError, setFormError] = useState<string | undefined>()
  const [notice, setNotice] = useState<string | undefined>()
  const [captchaToken, setCaptchaToken] = useState<string | null>(null)
  const [mfa, setMfa] = useState<MfaChallenge | null>(null)
  const [passkeyPending, setPasskeyPending] = useState(false)
  /** Bumped to restart the passkey autofill request after an explicit ceremony ended. */
  const [autofillRun, setAutofillRun] = useState(0)
  const captchaRef = useRef<CaptchaHandle>(null)
  const captcha = config?.captcha
  const captchaEnabled = Boolean(captcha && captcha.provider !== 'none' && captcha.site_key)
  const passkeyEnabled = Boolean(config?.passkey_enabled) && supportsWebAuthn()
  const [tokenState, setTokenState] = useState<'idle' | 'pending' | 'failed'>(
    search.token ? 'pending' : 'idle',
  )
  const doneRef = useRef(false)
  const finish = useCallback(
    (result: AuthResult) => {
      doneRef.current = true
      finishAuth(result, t('messages.loginSuccess'))
    },
    [finishAuth, t],
  )

  // Admin "login as" one-time link. Tokens are single use: guard against StrictMode double effects.
  const token = search.token
  const usedToken = useRef<string | null>(null)
  useEffect(() => {
    if (!token || usedToken.current === token) return
    usedToken.current = token
    loginWithToken(token)
      .then(finish)
      .catch(() => setTokenState('failed'))
  }, [token, finish])

  const form = useLiteForm(
    { email: '', password: '' },
    {
      email: (v) => validateEmail(v),
      password: (v) => (v ? undefined : t('validation.passwordRequired')),
    },
  )

  /** Localized text for login failures (401 / 403 / 429 + Retry-After), else the generic split. */
  const describeError = (error: unknown): { fields: Record<string, string>; message: string | undefined } => {
    if (isApiError(error)) {
      if (error.status === 429)
        return {
          fields: {},
          message:
            error.retryAfter !== null
              ? t('lockout.wait', { wait: waitText(error.retryAfter, locale) })
              : t('lockout.generic'),
        }
      if (error.status === 401 && !error.isValidation) return { fields: {}, message: t('loginError.invalid') }
      if (error.status === 403) return { fields: {}, message: t('loginError.banned') }
    }
    return splitAuthError(error)
  }

  const submit = async (values: typeof form.values) => {
    setFormError(undefined)
    setNotice(undefined)
    if (captchaEnabled && !captchaToken) {
      setFormError(t('captcha.required'))
      return
    }
    void import('./-login/MfaStep') // warm the step-2 chunk while the request is in flight
    try {
      const result = await login({
        email: values.email.trim(),
        password: values.password,
        ...(captchaToken ? { captcha: captchaToken } : {}),
      })
      if (isMfaChallenge(result)) setMfa(result)
      else finish(result)
    } catch (error) {
      const { fields, message } = describeError(error)
      for (const [name, msg] of Object.entries(fields))
        if (name in values) form.setError(name as 'email', msg)
      setFormError(message)
      captchaRef.current?.reset()
    }
  }

  const passkeyMessage = (error: unknown): string => {
    switch (webAuthnFailure(error)) {
      case 'unsupported':
        return t('passkey.unsupported')
      case 'security':
        return t('passkey.security')
      default:
        return isApiError(error) && error.status === 401 ? t('passkey.notRecognized') : errorMessage(error)
    }
  }

  /** Explicit "Sign in with a passkey" (usernameless, or scoped to the typed email). */
  const signInWithPasskey = async () => {
    if (passkeyPending) return
    setFormError(undefined)
    setNotice(undefined)
    setPasskeyPending(true)
    try {
      const { startAuthentication } = await loadWebAuthn()
      const email = form.values.email.trim()
      const begin = await passkeyLoginBegin(EMAIL_RE.test(email) ? email : undefined)
      const credential = await startAuthentication({ optionsJSON: begin.options })
      finish(await passkeyLoginFinish({ challenge_id: begin.challenge_id, credential }))
    } catch (error) {
      if (webAuthnFailure(error) === 'cancelled') setNotice(t('passkey.cancelled'))
      else setFormError(passkeyMessage(error))
      setAutofillRun((n) => n + 1)
    } finally {
      setPasskeyPending(false)
    }
  }

  // Passkey autofill (conditional mediation): offered in the email field's autofill menu.
  const showPassword = tokenState !== 'pending' && mfa === null
  useEffect(() => {
    if (!passkeyEnabled || !showPassword) return
    let cancelled = false
    const cancelIdle = onIdle(() => {
      void (async () => {
        try {
          if (!(await supportsWebAuthnAutofill()) || cancelled) return
          const { startAuthentication } = await loadWebAuthn()
          const begin = await passkeyLoginBegin()
          if (cancelled) return
          const credential = await startAuthentication({
            optionsJSON: begin.options,
            useBrowserAutofill: true,
          })
          if (cancelled) return
          finish(await passkeyLoginFinish({ challenge_id: begin.challenge_id, credential }))
        } catch (error) {
          if (cancelled || webAuthnFailure(error) === 'cancelled') return
          // Begin failures (backend down, passkeys disabled) stay silent: the password form still works.
          if (!isApiError(error)) setFormError(passkeyMessage(error))
        }
      })()
    }, 1500)
    return () => {
      cancelled = true
      cancelIdle()
      if (!doneRef.current)
        void loadWebAuthn()
          .then((lib) => lib.WebAuthnAbortService.cancelCeremony())
          .catch(() => undefined)
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps -- restart only on these triggers
  }, [passkeyEnabled, showPassword, autofillRun])

  const restartFromMfa = (reason?: MfaRestartReason) => {
    setMfa(null)
    form.setValues((v) => ({ ...v, password: '' }))
    setFormError(reason ? t(reason === 'expired' ? 'mfa.expired' : 'mfa.tooMany') : undefined)
  }

  return (
    <AuthLayout
      title={mfa ? t('mfa.title') : t('login.title')}
      subtitle={mfa ? undefined : t('login.subtitle')}
      footer={
        mfa || config?.register_enabled === false ? null : (
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
        <div
          className="flex items-center justify-center gap-3 py-6 text-subhead text-label-secondary"
          role="status"
        >
          <Spinner />
          {t('login.tokenSigningIn')}
        </div>
      ) : mfa ? (
        <Suspense
          fallback={
            <div className="flex min-h-[220px] items-center justify-center" role="status">
              <Spinner />
            </div>
          }
        >
          <MfaStep
            challenge={mfa}
            onSuccess={finish}
            onRestart={restartFromMfa}
            onPasskey={mfa.has_passkey && passkeyEnabled ? () => void signInWithPasskey() : undefined}
            passkeyPending={passkeyPending}
          />
        </Suspense>
      ) : (
        <form
          noValidate
          className="flex flex-col gap-4"
          onSubmit={(e) => void form.handleSubmit(submit)(e)}
          aria-label={t('login.title')}
        >
          <FormError message={tokenState === 'failed' ? t('login.tokenFailed') : formError} />
          {notice ? (
            <p role="status" className="text-center text-footnote text-label-secondary">
              {notice}
            </p>
          ) : null}
          <NativeField
            {...form.field('email')}
            label={t('fields.email')}
            type="email"
            inputMode="email"
            autoComplete="username webauthn"
            autoCapitalize="none"
            spellCheck={false}
            placeholder={t('fields.emailPlaceholder')}
            required
          />
          <NativeField
            {...form.field('password')}
            label={t('fields.password')}
            type="password"
            autoComplete="current-password"
            required
          />
          <div className="-mt-1 flex justify-end">
            <Link to="/forgot" className="text-footnote text-label-blue hover:underline">
              {t('login.forgot')}
            </Link>
          </div>
          {captchaEnabled && captcha ? (
            <LazyCaptcha
              ref={captchaRef}
              provider={captcha.provider}
              siteKey={captcha.site_key}
              onToken={setCaptchaToken}
            />
          ) : null}
          <NativeButton type="submit" size="lg" fullWidth pending={form.submitting}>
            {t('login.submit')}
          </NativeButton>
          {passkeyEnabled ? (
            <>
              <div className="flex items-center gap-3 text-footnote text-label-secondary" aria-hidden="true">
                <span className="h-px flex-1 bg-hairline" />
                {t('passkey.or')}
                <span className="h-px flex-1 bg-hairline" />
              </div>
              <NativeButton
                variant="secondary"
                size="lg"
                fullWidth
                pending={passkeyPending}
                onClick={() => void signInWithPasskey()}
              >
                <PasskeyIcon />
                {t('passkey.signIn')}
              </NativeButton>
            </>
          ) : null}
        </form>
      )}
    </AuthLayout>
  )
}

/** Passkey glyph (lucide "key-round", ISC) inline: the public pages don't load lucide. */
function PasskeyIcon() {
  return (
    <svg
      xmlns="http://www.w3.org/2000/svg"
      width="18"
      height="18"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      <path d="M2.586 17.414A2 2 0 0 0 2 18.828V21a1 1 0 0 0 1 1h3a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h1a1 1 0 0 0 1-1v-1a1 1 0 0 1 1-1h.172a2 2 0 0 0 1.414-.586l.814-.814a6.5 6.5 0 1 0-4-4z" />
      <circle cx="16.5" cy="7.5" r=".5" fill="currentColor" />
    </svg>
  )
}
