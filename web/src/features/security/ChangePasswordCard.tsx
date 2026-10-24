/**
 * Change password (`POST /user/password`). Semantic form for password managers: read-only username
 * (`autocomplete="username"`), `current-password`, `new-password`. The change bumps `password_version`, so
 * the session ends: sign out and go to the login page.
 */
import { Button } from '@heroui/react'
import { useNavigate } from '@tanstack/react-router'
import { type FormEvent, useState } from 'react'
import { FormError } from '@/components/FormError'
import { TitleWithHelp } from '@/components/HelpTip'
import { useT } from '@/i18n'
import { changePassword } from '@/lib/api/user'
import { PASSWORD_MIN } from '@/lib/validators'
import { toast } from '@/lib/toast'
import { useAuthStore } from '@/stores/auth'
import { useSecurityError } from './api'
import { PlainField } from './shared'

export function ChangePasswordCard({ email }: { email: string }) {
  const t = useT(['security', 'common'])
  const navigate = useNavigate()
  const mapError = useSecurityError()
  const [oldPassword, setOldPassword] = useState('')
  const [newPassword, setNewPassword] = useState('')
  const [errors, setErrors] = useState<ReturnType<typeof mapError>>({})
  const [pending, setPending] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const local: ReturnType<typeof mapError> = {}
    if (!oldPassword) local.old_password = t('password.oldRequired')
    if (newPassword.length < PASSWORD_MIN) local.new_password = t('password.tooShort', { min: PASSWORD_MIN })
    else if (newPassword === oldPassword) local.new_password = t('password.same')
    setErrors(local)
    if (Object.keys(local).length > 0) return
    setPending(true)
    try {
      await changePassword({ old_password: oldPassword, new_password: newPassword })
      toast.success(t('password.changed'))
      useAuthStore.getState().logout() // the query cache follows the session (`bindSessionCache`)
      await navigate({ to: '/login', replace: true })
    } catch (error) {
      setErrors(mapError(error))
    } finally {
      setPending(false)
    }
  }

  return (
    <section id="password" className="surface-card flex flex-col gap-4 p-6" aria-labelledby="password-title">
      <TitleWithHelp help={t('password.hint')} subject={t('password.title')}>
        <h2 id="password-title" className="text-headline text-label">
          {t('password.title')}
        </h2>
      </TitleWithHelp>
      <form
        noValidate
        onSubmit={(e) => void submit(e)}
        className="flex max-w-md flex-col gap-4"
        aria-labelledby="password-title"
      >
        <FormError message={errors.form} />
        <PlainField
          name="username"
          type="email"
          label={t('password.account')}
          value={email}
          isReadOnly
          autoComplete="username"
        />
        <PlainField
          name="old_password"
          type="password"
          label={t('password.current')}
          value={oldPassword}
          onChange={(v) => {
            setOldPassword(v)
            setErrors((e) => ({ ...e, old_password: undefined, form: undefined }))
          }}
          autoComplete="current-password"
          error={errors.old_password}
        />
        <PlainField
          name="new_password"
          type="password"
          label={t('password.new')}
          value={newPassword}
          onChange={(v) => {
            setNewPassword(v)
            setErrors((e) => ({ ...e, new_password: undefined, form: undefined }))
          }}
          autoComplete="new-password"
          description={t('password.newHint', { min: PASSWORD_MIN })}
          error={errors.new_password}
        />
        <div>
          <Button type="submit" isPending={pending}>
            {t('password.submit')}
          </Button>
        </div>
      </form>
    </section>
  )
}
