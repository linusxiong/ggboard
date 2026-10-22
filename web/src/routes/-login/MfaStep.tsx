/**
 * Login step 2: TOTP or backup code (`POST /passport/login/mfa`). Lazy-loaded by the login page so the
 * first-load budget only pays for it when an account actually has 2FA.
 *
 * Password managers: a real <form>, the TOTP input has `autocomplete="one-time-code"` +
 * `inputmode="numeric"` + `pattern="[0-9]*"` (1Password / Bitwarden fill TOTP there); the backup-code
 * input is `autocomplete="off"`.
 */
import { type FormEvent, useState } from 'react'
import { FormError } from '@/components/FormError'
import { NativeButton } from '@/components/native/NativeButton'
import { NativeField } from '@/components/native/NativeField'
import { useT } from '@/i18n'
import { isApiError } from '@/lib/api/client'
import { loginMfa } from '@/lib/api/passport'
import type { AuthResult, MfaChallenge } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'

export type MfaRestartReason = 'expired' | 'tooMany'

export interface MfaStepProps {
  challenge: MfaChallenge
  onSuccess: (result: AuthResult) => void
  /** The MFA token is no longer usable: back to the password step with a message. */
  onRestart: (reason?: MfaRestartReason) => void
  /** Shown when the account has a passkey and the browser supports WebAuthn. */
  onPasskey?: (() => void) | undefined
  passkeyPending?: boolean
}

const TOTP_RE = /^\d{6}$/
const BACKUP_RE = /^[a-z0-9]{5}-?[a-z0-9]{5}$/i

export default function MfaStep({
  challenge,
  onSuccess,
  onRestart,
  onPasskey,
  passkeyPending,
}: MfaStepProps) {
  const t = useT(['auth', 'core'])
  const [mode, setMode] = useState<'totp' | 'backup'>('totp')
  const [code, setCode] = useState('')
  const [fieldError, setFieldError] = useState<string>()
  const [formError, setFormError] = useState<string>()
  const [pending, setPending] = useState(false)

  const submit = async (value: string) => {
    if (pending) return
    const normalized = value.replace(/\s+/g, '')
    if (!(mode === 'totp' ? TOTP_RE : BACKUP_RE).test(normalized)) {
      setFieldError(mode === 'totp' ? t('mfa.codeInvalid') : t('mfa.backupInvalid'))
      return
    }
    setPending(true)
    setFormError(undefined)
    try {
      onSuccess(await loginMfa({ mfa_token: challenge.mfa_token, code: normalized }))
    } catch (error) {
      if (isApiError(error) && error.status === 429) return onRestart('tooMany')
      if (isApiError(error) && error.code === 'mfa_expired') return onRestart('expired')
      if (isApiError(error) && (error.status === 401 || error.code === 'mfa_invalid')) {
        setFieldError(mode === 'totp' ? t('mfa.invalid') : t('mfa.backupWrong'))
        setCode('')
      } else {
        setFormError(errorMessage(error))
      }
    } finally {
      setPending(false)
    }
  }

  const onSubmit = (event: FormEvent) => {
    event.preventDefault()
    void submit(code)
  }

  const switchMode = () => {
    setMode((m) => (m === 'totp' ? 'backup' : 'totp'))
    setCode('')
    setFieldError(undefined)
  }

  return (
    <form noValidate className="flex flex-col gap-4" onSubmit={onSubmit} aria-label={t('mfa.title')}>
      <p className="text-center text-subhead text-label-secondary">
        {mode === 'totp' ? t('mfa.subtitle') : t('mfa.subtitleBackup')}
      </p>
      <FormError message={formError} />
      {mode === 'totp' ? (
        <NativeField
          key="totp"
          name="code"
          label={t('mfa.code')}
          value={code}
          error={fieldError}
          autoComplete="one-time-code"
          inputMode="numeric"
          pattern="[0-9]*"
          maxLength={6}
          autoFocus
          required
          className="[&_input]:text-center [&_input]:font-mono [&_input]:text-title3 [&_input]:tracking-[0.4em]"
          onChange={(e) => {
            const next = e.currentTarget.value.replace(/\D/g, '').slice(0, 6)
            setCode(next)
            setFieldError(undefined)
            // Autofill (or typing the last digit) submits right away.
            if (TOTP_RE.test(next)) void submit(next)
          }}
        />
      ) : (
        <NativeField
          key="backup"
          name="backup_code"
          label={t('mfa.backupCode')}
          value={code}
          error={fieldError}
          autoComplete="off"
          autoCapitalize="none"
          autoCorrect="off"
          spellCheck={false}
          placeholder="xxxxx-xxxxx"
          maxLength={11}
          autoFocus
          required
          className="[&_input]:text-center [&_input]:font-mono"
          onChange={(e) => {
            setCode(e.currentTarget.value.slice(0, 11))
            setFieldError(undefined)
          }}
        />
      )}
      <NativeButton type="submit" size="lg" fullWidth pending={pending}>
        {t('mfa.submit')}
      </NativeButton>
      <div className="flex flex-wrap items-center justify-between gap-2 text-footnote">
        <button type="button" className="text-label-blue hover:underline" onClick={switchMode}>
          {mode === 'totp' ? t('mfa.useBackup') : t('mfa.useTotp')}
        </button>
        <button type="button" className="text-label-blue hover:underline" onClick={() => onRestart()}>
          {t('mfa.back')}
        </button>
      </div>
      {onPasskey ? (
        <NativeButton variant="secondary" fullWidth pending={passkeyPending} onClick={onPasskey}>
          {t('passkey.useInstead')}
        </NativeButton>
      ) : null}
    </form>
  )
}
