/** Shared components for the profile → security panels: form inputs and the backup-codes view. */
import { Button, Description, FieldError, Input, Label, TextField } from '@heroui/react'
import { Download } from 'lucide-react'
import { type ReactNode, useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { useT } from '@/i18n'

/** HeroUI text input with a visible label, `name`/`id` and native autocomplete hints (password managers). */
export function PlainField({
  name,
  label,
  value,
  onChange,
  type = 'text',
  autoComplete,
  inputMode,
  pattern,
  maxLength,
  autoFocus,
  error,
  description,
  isReadOnly,
  placeholder,
  className,
  inputClassName,
}: {
  name: string
  label: ReactNode
  value: string
  onChange?: (value: string) => void
  type?: 'text' | 'password' | 'email'
  autoComplete: string
  inputMode?: 'text' | 'numeric' | 'email'
  pattern?: string
  maxLength?: number
  autoFocus?: boolean
  error?: string | undefined
  description?: ReactNode
  isReadOnly?: boolean
  placeholder?: string
  className?: string
  inputClassName?: string
}) {
  return (
    <TextField
      name={name}
      id={name}
      type={type}
      value={value}
      onChange={onChange}
      isInvalid={Boolean(error)}
      isReadOnly={isReadOnly}
      autoFocus={autoFocus}
      validationBehavior="aria"
      variant="secondary"
      fullWidth
      className={className}
    >
      <Label>{label}</Label>
      <Input
        autoComplete={autoComplete}
        inputMode={inputMode}
        pattern={pattern}
        maxLength={maxLength}
        placeholder={placeholder}
        spellCheck={false}
        autoCapitalize="none"
        className={inputClassName}
      />
      {description && !error ? <Description>{description}</Description> : null}
      <FieldError>{error}</FieldError>
    </TextField>
  )
}

/** 6-digit TOTP input (1Password / Bitwarden autofill `one-time-code`). */
export function TotpField({
  value,
  onChange,
  error,
  label,
  autoFocus,
  allowBackup = false,
}: {
  value: string
  onChange: (value: string) => void
  error?: string | undefined
  label?: string
  autoFocus?: boolean
  /** Also accept a backup code `xxxxx-xxxxx` (disable / regenerate). */
  allowBackup?: boolean
}) {
  const t = useT('security')
  return (
    <PlainField
      name="code"
      label={label ?? t('totp.code')}
      value={value}
      onChange={(v) => onChange(allowBackup ? v.trim().slice(0, 11) : v.replace(/\D/g, '').slice(0, 6))}
      autoComplete="one-time-code"
      inputMode={allowBackup ? 'text' : 'numeric'}
      pattern={allowBackup ? undefined : '[0-9]*'}
      maxLength={allowBackup ? 11 : 6}
      autoFocus={autoFocus}
      error={error}
      description={allowBackup ? t('totp.codeOrBackup') : undefined}
      inputClassName="font-mono tracking-widest"
    />
  )
}

/** Hidden-from-AT username field so password managers know which login a password field belongs to. */
export function UsernameHint({ email }: { email: string }) {
  return (
    <input
      type="email"
      name="username"
      autoComplete="username"
      value={email}
      readOnly
      tabIndex={-1}
      aria-hidden="true"
      className="sr-only"
    />
  )
}

/** One-time display of backup codes with copy and `.txt` download. */
export function BackupCodesView({
  codes,
  email,
  appName,
}: {
  codes: readonly string[]
  email: string
  appName: string
}) {
  const t = useT(['security', 'common'])
  const [downloaded, setDownloaded] = useState(false)
  const text = [
    `${appName} — ${t('backup.fileTitle')}`,
    email,
    new Date().toISOString().slice(0, 10),
    '',
    ...codes,
    '',
    t('backup.fileHint'),
  ].join('\n')

  const download = () => {
    const url = URL.createObjectURL(new Blob([text], { type: 'text/plain;charset=utf-8' }))
    const a = document.createElement('a')
    a.href = url
    a.download = `${appName.replace(/[^\w.-]+/g, '-').toLowerCase() || 'ggbond'}-backup-codes.txt`
    document.body.appendChild(a)
    a.click()
    a.remove()
    URL.revokeObjectURL(url)
    setDownloaded(true)
  }

  return (
    <div className="flex flex-col gap-3">
      <p className="text-subhead text-label">{t('backup.once')}</p>
      <ol
        aria-label={t('backup.title')}
        className="grid grid-cols-2 gap-x-4 gap-y-1.5 rounded-[12px] bg-fill-quaternary p-4 font-mono text-subhead text-label"
      >
        {codes.map((code) => (
          <li key={code} className="tabular select-all">
            {code}
          </li>
        ))}
      </ol>
      <div className="flex flex-wrap gap-2">
        <CopyButton value={codes.join('\n')} size="sm">
          {t('backup.copy')}
        </CopyButton>
        <Button size="sm" variant="secondary" onPress={download}>
          <Download className="size-4" aria-hidden="true" />
          {downloaded ? t('backup.downloaded') : t('backup.download')}
        </Button>
      </div>
    </div>
  )
}
