import { useState } from 'react'
import { NativeButton } from '@/components/native/NativeButton'
import { useCooldown } from '@/hooks/useCooldown'
import { useT } from '@/i18n'
import { EMAIL_CODE_COOLDOWN, sendEmailCode } from '@/lib/api/passport'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'

interface EmailCodeButtonProps {
  /** Purpose namespace for the cooldown (register / forgot). */
  purpose: string
  /** Returns the email to send to, or `undefined` if invalid (the caller shows the field error). */
  getEmail: () => string | undefined
  /** Captcha token when captcha is enabled (single use). */
  captcha?: string | null
  /** Called after a send attempt so the caller can reset the captcha. */
  onSent?: () => void
}

/** "Send code" with a 60s cooldown (matches the backend's per-email limit). */
export function EmailCodeButton({ purpose, getEmail, captcha, onSent }: EmailCodeButtonProps) {
  const t = useT('auth')
  const { remaining, start } = useCooldown(`${purpose}-code`, EMAIL_CODE_COOLDOWN)
  const [pending, setPending] = useState(false)

  const send = async () => {
    const email = getEmail()
    if (!email) return
    setPending(true)
    try {
      await sendEmailCode({ email, ...(captcha ? { captcha } : {}) })
      start()
      toast.success(t('code.sent'))
    } catch (error) {
      toast.danger(errorMessage(error))
    } finally {
      setPending(false)
      onSent?.()
    }
  }

  return (
    <NativeButton
      variant="secondary"
      className="h-auto shrink-0 whitespace-nowrap"
      disabled={remaining > 0}
      pending={pending}
      onClick={() => void send()}
    >
      {remaining > 0 ? t('code.resend', { seconds: remaining }) : t('code.send')}
    </NativeButton>
  )
}
