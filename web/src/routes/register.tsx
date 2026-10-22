import { createFileRoute, Link } from '@tanstack/react-router'
import { UserX } from '@/components/icons/inline'
import { useRef, useState } from 'react'
import { EmailCodeButton, type EmailCodeResult } from '@/components/auth/EmailCodeButton'
import type { CaptchaHandle } from '@/components/Captcha'
import { LazyCaptcha } from '@/components/LazyCaptcha'
import { FormError } from '@/components/FormError'
import { AuthLayout } from '@/components/layout/AuthLayout'
import { NativeButton } from '@/components/native/NativeButton'
import { NativeField } from '@/components/native/NativeField'
import { useFinishAuth } from '@/hooks/useFinishAuth'
import { useLiteForm } from '@/hooks/useLiteForm'
import { loadNamespaces, useT } from '@/i18n'
import { register } from '@/lib/api/passport'
import { getGuestConfig, withDeadline } from '@/lib/publicData'
import { splitAuthError } from '@/lib/authErrors'
import { buttonClass } from '@/lib/button'
import { redirectIfAuthenticated } from '@/lib/guards'
import { validateEmail, validatePassword, validateRequired } from '@/lib/validators'

interface RegisterSearch {
  /** Invite code from a shared link (`/register?code=…`). */
  code?: string
  redirect?: string
}

export const Route = createFileRoute('/register')({
  validateSearch: (search: Record<string, unknown>): RegisterSearch => ({
    ...(typeof search.code === 'string' ? { code: search.code } : {}),
    ...(typeof search.redirect === 'string' ? { redirect: search.redirect } : {}),
  }),
  beforeLoad: ({ search }) => redirectIfAuthenticated({ search }),
  loader: async () => {
    const [config] = await Promise.all([withDeadline(getGuestConfig()), loadNamespaces(['auth'])])
    return { config }
  },
  component: RegisterPage,
})

function RegisterPage() {
  const t = useT(['auth', 'core'])
  const search = Route.useSearch()
  const { config } = Route.useLoaderData()
  const finish = useFinishAuth(search.redirect)
  const [formError, setFormError] = useState<string | undefined>()
  const [captchaToken, setCaptchaToken] = useState<string | null>(null)
  const [codeSentTo, setCodeSentTo] = useState<string>()
  const captchaRef = useRef<CaptchaHandle>(null)

  const whitelist = config?.email_whitelist_suffix ?? []
  const emailVerify = Boolean(config?.email_verify)
  const inviteForce = Boolean(config?.invite_force)
  const captcha = config?.captcha
  const captchaEnabled = Boolean(captcha && captcha.provider !== 'none' && captcha.site_key)

  const form = useLiteForm(
    { email: '', email_code: '', password: '', invite_code: search.code ?? '' },
    {
      email: (v) => validateEmail(v, whitelist),
      email_code: (v) => (emailVerify ? validateRequired(v) : undefined),
      password: (v) => validatePassword(v),
      invite_code: (v) => (inviteForce ? validateRequired(v, 'validation.inviteRequired') : undefined),
    },
  )

  if (config && !config.register_enabled) {
    return (
      <AuthLayout
        title={t('register.closedTitle')}
        subtitle={t('register.closedHint')}
        icon={<UserX strokeWidth={1.5} />}
      >
        <div className="flex flex-col items-center gap-5 text-center">
          <div className="flex w-full flex-col gap-2">
            <Link to="/login" className={buttonClass({ size: 'lg', fullWidth: true })}>
              {t('register.login')}
            </Link>
            <Link to="/" className={buttonClass({ variant: 'tertiary', fullWidth: true })}>
              {t('register.home')}
            </Link>
          </div>
        </div>
      </AuthLayout>
    )
  }

  const submit = async (values: typeof form.values) => {
    setFormError(undefined)
    if (captchaEnabled && !captchaToken) {
      setFormError(t('captcha.required'))
      return
    }
    try {
      const result = await register({
        email: values.email.trim(),
        password: values.password,
        ...(values.invite_code.trim() ? { invite_code: values.invite_code.trim() } : {}),
        ...(emailVerify ? { email_code: values.email_code.trim() } : {}),
        ...(captchaToken ? { captcha: captchaToken } : {}),
      })
      finish(result, t('messages.registerSuccess'))
    } catch (error) {
      const { fields, message } = splitAuthError(error)
      for (const [name, msg] of Object.entries(fields))
        if (name in values) form.setError(name as 'email', msg)
      setFormError(message)
      captchaRef.current?.reset()
    }
  }

  /** Validates the email field before sending a code; returns it when valid. */
  const emailForCode = () => (form.validateField('email') ? undefined : form.values.email.trim())

  const onCodeResult = (result: EmailCodeResult) => {
    if (result.ok) {
      setCodeSentTo(result.email)
      form.setError('email_code', undefined)
    } else {
      setCodeSentTo(undefined)
      form.setError('email_code', result.message)
    }
  }

  /** Whitelist chip: completes the address with that domain (keeps what was typed before "@"). */
  const pickDomain = (domain: string) => {
    const local = form.values.email.split('@')[0]?.trim() ?? ''
    form.setValues((v) => ({ ...v, email: local ? `${local}@${domain}` : '' }))
    form.setError('email', undefined)
    document.getElementById(local ? 'register-password' : 'register-email')?.focus()
  }

  return (
    <AuthLayout
      title={t('register.title')}
      subtitle={t('register.subtitle')}
      footer={
        <>
          {t('register.hasAccount')}{' '}
          <Link
            to="/login"
            search={search.redirect ? { redirect: search.redirect } : {}}
            className="font-semibold text-label-blue hover:underline"
          >
            {t('register.login')}
          </Link>
        </>
      }
    >
      <form noValidate className="flex flex-col gap-4" onSubmit={(e) => void form.handleSubmit(submit)(e)}>
        <FormError message={formError} />
        <NativeField
          {...form.field('email')}
          id="register-email"
          label={t('fields.email')}
          type="email"
          inputMode="email"
          autoComplete="username"
          autoCapitalize="none"
          spellCheck={false}
          placeholder={t('fields.emailPlaceholder')}
          required
        />
        {whitelist.length > 0 ? (
          <div
            role="group"
            aria-label={t('register.whitelistLabel')}
            className="-mt-2 flex flex-wrap items-center gap-1.5 px-0.5 text-footnote text-label-secondary"
          >
            <span aria-hidden="true">{t('register.whitelistLabel')}</span>
            {whitelist.map((domain) => (
              <button
                key={domain}
                type="button"
                aria-label={t('register.useDomain', { domain })}
                // Keep focus in the email field (no blur validation between press and click).
                onMouseDown={(e) => e.preventDefault()}
                onClick={() => pickDomain(domain)}
                className="rounded-full bg-fill-tertiary px-2 py-0.5 font-medium text-label transition-colors hover:bg-fill-secondary"
              >
                @{domain}
              </button>
            ))}
          </div>
        ) : null}
        {captchaEnabled && captcha ? (
          <LazyCaptcha
            ref={captchaRef}
            provider={captcha.provider}
            siteKey={captcha.site_key}
            onToken={setCaptchaToken}
          />
        ) : null}
        {emailVerify ? (
          <NativeField
            {...form.field('email_code')}
            label={t('fields.emailCode')}
            inputMode="numeric"
            autoComplete="one-time-code"
            maxLength={12}
            required
            description={
              codeSentTo ? (
                <span className="text-label-green">{t('code.sentTo', { email: codeSentTo })}</span>
              ) : undefined
            }
            end={
              <EmailCodeButton
                purpose="register"
                getEmail={emailForCode}
                captcha={captchaToken}
                onSent={() => captchaRef.current?.reset()}
                onResult={onCodeResult}
              />
            }
          />
        ) : null}
        <NativeField
          {...form.field('password')}
          id="register-password"
          label={t('fields.password')}
          type="password"
          autoComplete="new-password"
          description={t('fields.passwordHint')}
          required
        />
        <NativeField
          {...form.field('invite_code')}
          label={inviteForce ? t('fields.inviteCode') : t('fields.inviteCodeOptional')}
          autoComplete="off"
          required={inviteForce}
        />
        <NativeButton type="submit" size="lg" fullWidth pending={form.submitting}>
          {t('register.submit')}
        </NativeButton>
        {config?.tos_url ? (
          <p className="text-center text-footnote text-label-secondary">
            {t('register.agree')}{' '}
            <a
              href={config.tos_url}
              target="_blank"
              rel="noopener noreferrer"
              className="text-label-blue hover:underline"
            >
              {t('register.tos')}
            </a>
          </p>
        ) : null}
      </form>
    </AuthLayout>
  )
}
