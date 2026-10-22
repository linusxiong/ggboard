import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { useRef, useState } from 'react'
import { EmailCodeButton } from '@/components/auth/EmailCodeButton'
import type { CaptchaHandle } from '@/components/Captcha'
import { LazyCaptcha } from '@/components/LazyCaptcha'
import { FormError } from '@/components/FormError'
import { AuthLayout } from '@/components/layout/AuthLayout'
import { NativeButton } from '@/components/native/NativeButton'
import { NativeField } from '@/components/native/NativeField'
import { useLiteForm } from '@/hooks/useLiteForm'
import { loadNamespaces, useT } from '@/i18n'
import { forgot } from '@/lib/api/passport'
import { getGuestConfig, withDeadline } from '@/lib/publicData'
import { splitAuthError } from '@/lib/authErrors'
import { toast } from '@/lib/toast'
import { validateEmail, validatePassword, validateRequired } from '@/lib/validators'

export const Route = createFileRoute('/forgot')({
  loader: async () => {
    const [config] = await Promise.all([withDeadline(getGuestConfig()), loadNamespaces(['auth'])])
    return { config }
  },
  component: ForgotPage,
})

function ForgotPage() {
  const t = useT(['auth', 'core'])
  const navigate = useNavigate()
  const { config } = Route.useLoaderData()
  const [formError, setFormError] = useState<string | undefined>()
  const [captchaToken, setCaptchaToken] = useState<string | null>(null)
  const captchaRef = useRef<CaptchaHandle>(null)
  const captcha = config?.captcha
  const captchaEnabled = Boolean(captcha && captcha.provider !== 'none' && captcha.site_key)

  const form = useLiteForm(
    { email: '', email_code: '', password: '' },
    {
      email: (v) => validateEmail(v),
      email_code: (v) => validateRequired(v),
      password: (v) => validatePassword(v),
    },
  )

  const onSubmit = form.handleSubmit(async (values) => {
    setFormError(undefined)
    try {
      await forgot({ email: values.email.trim(), email_code: values.email_code.trim(), password: values.password })
      toast.success(t('forgot.success'))
      await navigate({ to: '/login', replace: true })
    } catch (error) {
      const { fields, message } = splitAuthError(error)
      for (const [name, msg] of Object.entries(fields)) if (name in values) form.setError(name as 'email', msg)
      setFormError(message)
    }
  })

  const emailForCode = () => (form.validateField('email') ? undefined : form.values.email.trim())

  return (
    <AuthLayout
      title={t('forgot.title')}
      subtitle={t('forgot.subtitle')}
      footer={
        <Link to="/login" className="font-semibold text-label-blue hover:underline">
          {t('forgot.backToLogin')}
        </Link>
      }
    >
      <form noValidate className="flex flex-col gap-4" onSubmit={(e) => void onSubmit(e)}>
        <FormError message={formError} />
        <NativeField
          {...form.field('email')}
          label={t('fields.email')}
          type="email"
          inputMode="email"
          autoComplete="username"
          placeholder={t('fields.emailPlaceholder')}
          required
        />
        {captchaEnabled && captcha ? (
          <LazyCaptcha ref={captchaRef} provider={captcha.provider} siteKey={captcha.site_key} onToken={setCaptchaToken} />
        ) : null}
        <NativeField
          {...form.field('email_code')}
          label={t('fields.emailCode')}
          inputMode="numeric"
          autoComplete="one-time-code"
          maxLength={12}
          required
          end={
            <EmailCodeButton
              purpose="forgot"
              getEmail={emailForCode}
              captcha={captchaToken}
              onSent={() => captchaRef.current?.reset()}
            />
          }
        />
        <NativeField
          {...form.field('password')}
          label={t('fields.newPassword')}
          type="password"
          autoComplete="new-password"
          description={t('fields.passwordHint')}
          required
        />
        <NativeButton type="submit" size="lg" fullWidth pending={form.submitting}>
          {t('forgot.submit')}
        </NativeButton>
      </form>
    </AuthLayout>
  )
}
