/**
 * TOTP two-factor authentication: enable wizard (setup → QR + manual secret → verify → backup codes),
 * regenerate backup codes, disable (password + code).
 */
import { Button, Chip, Modal } from '@heroui/react'
import { useQueryClient } from '@tanstack/react-query'
import { ShieldCheck, Smartphone } from 'lucide-react'
import { type FormEvent, lazy, type ReactNode, Suspense, useEffect, useRef, useState } from 'react'
import { CopyButton } from '@/components/CopyButton'
import { DateTime } from '@/components/Formatters'
import { FormError } from '@/components/FormError'
import { HelpTip, TitleWithHelp } from '@/components/HelpTip'
import { useT } from '@/i18n'
import type { SecurityOverview, TotpSetup } from '@/lib/api/types'
import { regenerateBackupCodes, totpDisable, totpEnable, totpSetup } from '@/lib/api/user'
import { toast } from '@/lib/toast'
import { CODE_OR_BACKUP_RE, securityKey, TOTP_RE, useSecurityError } from './api'
import { BackupCodesView, PlainField, TotpField, UsernameHint } from './shared'
import { ModalClose } from '@/components/feedback/ModalClose'

const QRCode = lazy(() => import('@/components/QRCode').then((m) => ({ default: m.QRCode })))

type Dialog = 'enable' | 'disable' | 'regenerate' | null

export function TwoFactorCard({
  overview,
  email,
  appName,
}: {
  overview: SecurityOverview
  email: string
  appName: string
}) {
  const t = useT(['security', 'common'])
  const [dialog, setDialog] = useState<Dialog>(null)
  const close = () => setDialog(null)

  return (
    <section className="surface-card flex flex-col gap-4 p-6" aria-labelledby="totp-title">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <span
            className="flex size-9 shrink-0 items-center justify-center rounded-[10px] bg-fill-tertiary text-label-secondary"
            aria-hidden="true"
          >
            <Smartphone className="size-5" />
          </span>
          <div className="min-w-0">
            <TitleWithHelp help={t('totp.hint')} subject={t('totp.title')}>
              <h2 id="totp-title" className="flex flex-wrap items-center gap-2 text-headline text-label">
                {t('totp.title')}
                <Chip size="sm" variant="soft" color={overview.totp_enabled ? 'success' : 'default'}>
                  {overview.totp_enabled ? t('totp.on') : t('totp.off')}
                </Chip>
              </h2>
            </TitleWithHelp>
            {overview.totp_enabled && overview.totp_enabled_at ? (
              <p className="mt-1 text-footnote text-label-secondary">
                {t('totp.enabledSince')} <DateTime value={overview.totp_enabled_at} format="date" />
              </p>
            ) : null}
          </div>
        </div>
        {overview.totp_enabled ? (
          <Button size="sm" variant="danger-soft" onPress={() => setDialog('disable')}>
            {t('totp.disable')}
          </Button>
        ) : (
          <Button size="sm" onPress={() => setDialog('enable')}>
            <ShieldCheck className="size-4" aria-hidden="true" />
            {t('totp.enable')}
          </Button>
        )}
      </div>

      {overview.totp_enabled ? (
        <div className="flex flex-wrap items-center justify-between gap-3 rounded-[12px] bg-fill-quaternary px-4 py-3">
          <div className="min-w-0">
            <p className="flex items-center gap-0.5 text-subhead text-label">
              {t('backup.remaining', { count: overview.backup_codes_remaining })}
              <HelpTip subject={t('backup.codes')}>{t('backup.about')}</HelpTip>
            </p>
            {overview.backup_codes_remaining <= 3 ? (
              <p className="text-footnote text-label-orange">{t('backup.low')}</p>
            ) : null}
          </div>
          <Button size="sm" variant="secondary" onPress={() => setDialog('regenerate')}>
            {t('backup.regenerate')}
          </Button>
        </div>
      ) : null}

      {dialog === 'enable' ? <EnableDialog email={email} appName={appName} onClose={close} /> : null}
      {dialog === 'disable' ? <DisableDialog email={email} onClose={close} /> : null}
      {dialog === 'regenerate' ? <RegenerateDialog email={email} appName={appName} onClose={close} /> : null}
    </section>
  )
}

function Frame({
  title,
  onClose,
  dismissable = true,
  children,
  footer,
  onSubmit,
}: {
  title: string
  onClose: () => void
  dismissable?: boolean
  children: ReactNode
  footer: ReactNode
  onSubmit?: (event: FormEvent) => void
}) {
  const body = (
    <>
      <Modal.Body className="flex flex-col gap-4">{children}</Modal.Body>
      <Modal.Footer>{footer}</Modal.Footer>
    </>
  )
  return (
    <Modal.Backdrop
      isOpen
      onOpenChange={(open) => {
        if (!open) onClose()
      }}
      isDismissable={dismissable}
      isKeyboardDismissDisabled={!dismissable}
    >
      <Modal.Container size="md" scroll="inside">
        <Modal.Dialog>
          {dismissable ? <ModalClose /> : null}
          <Modal.Header>
            <Modal.Heading>{title}</Modal.Heading>
          </Modal.Header>
          {onSubmit ? (
            <form noValidate onSubmit={onSubmit} className="contents">
              {body}
            </form>
          ) : (
            body
          )}
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}

/** Groups a base32 secret in fours for reading aloud / typing. */
const groupSecret = (secret: string) =>
  secret
    .replace(/\s+/g, '')
    .replace(/(.{4})/g, '$1 ')
    .trim()

function EnableDialog({ email, appName, onClose }: { email: string; appName: string; onClose: () => void }) {
  const t = useT(['security', 'common'])
  const queryClient = useQueryClient()
  const mapError = useSecurityError()
  const [setup, setSetup] = useState<TotpSetup | null>(null)
  const [codes, setCodes] = useState<string[] | null>(null)
  const [code, setCode] = useState('')
  const [errors, setErrors] = useState<ReturnType<typeof mapError>>({})
  const [pending, setPending] = useState(false)
  const [saved, setSaved] = useState(false)
  const started = useRef(false)
  // The field auto-submits at 6 digits; Enter (or a password manager filling + submitting) must not POST
  // the same code again — the second call answers 409 "already enabled" (or 400 "step reused").
  const inFlight = useRef(false)

  useEffect(() => {
    if (started.current) return
    started.current = true
    totpSetup()
      .then(setSetup)
      .catch((error: unknown) => {
        setErrors(mapError(error))
        void queryClient.invalidateQueries({ queryKey: securityKey() })
      })
  }, [mapError, queryClient])

  const verify = async (value = code) => {
    if (inFlight.current || codes) return
    if (!TOTP_RE.test(value)) {
      setErrors({ code: t('totp.codeInvalid') })
      return
    }
    inFlight.current = true
    setPending(true)
    try {
      const result = await totpEnable(value)
      setCodes(result.backup_codes)
      toast.success(t('totp.enabled'))
      await queryClient.invalidateQueries({ queryKey: securityKey() })
    } catch (error) {
      setErrors(mapError(error))
      setCode('')
    } finally {
      inFlight.current = false
      setPending(false)
    }
  }

  if (codes) {
    return (
      <Frame
        title={t('backup.title')}
        onClose={onClose}
        dismissable={false}
        footer={
          <div className="flex w-full flex-wrap items-center justify-between gap-3">
            <label className="flex items-center gap-2 text-subhead text-label">
              <input
                type="checkbox"
                checked={saved}
                onChange={(e) => setSaved(e.currentTarget.checked)}
                className="size-4 accent-tint"
              />
              {t('backup.confirmSaved')}
            </label>
            <Button isDisabled={!saved} onPress={onClose}>
              {t('common:actions.close')}
            </Button>
          </div>
        }
      >
        <BackupCodesView codes={codes} email={email} appName={appName} />
      </Frame>
    )
  }

  return (
    <Frame
      title={t('totp.setupTitle')}
      onClose={onClose}
      onSubmit={(e) => {
        e.preventDefault()
        void verify()
      }}
      footer={
        <>
          <Button variant="tertiary" onPress={onClose}>
            {t('common:actions.cancel')}
          </Button>
          <Button type="submit" isPending={pending} isDisabled={!setup}>
            {t('totp.verify')}
          </Button>
        </>
      }
    >
      <FormError message={errors.form} />
      <ol className="flex flex-col gap-4 text-subhead text-label">
        <li className="flex flex-col gap-3">
          <p className="flex items-center gap-0.5">
            <span>
              <span className="font-semibold">1.</span> {t('totp.step1')}
            </span>
            <HelpTip subject={t('totp.apps')}>{t('totp.appsHint')}</HelpTip>
          </p>
          <div className="flex flex-col items-center gap-4 sm:flex-row sm:items-start">
            {setup ? (
              <Suspense fallback={<div className="size-[168px] shimmer rounded-[12px] bg-fill-quaternary" />}>
                <QRCode value={setup.otpauth_url} size={168} label={t('totp.qrLabel')} className="shrink-0" />
              </Suspense>
            ) : (
              <div
                className="size-[168px] shrink-0 shimmer rounded-[12px] bg-fill-quaternary"
                aria-busy="true"
              />
            )}
            <div className="flex min-w-0 flex-col gap-2">
              <p className="text-footnote text-label-secondary">{t('totp.manual')}</p>
              <div className="flex items-center gap-2">
                <code className="min-w-0 rounded-[8px] bg-fill-quaternary px-2 py-1 font-mono text-footnote break-all text-label">
                  {setup ? groupSecret(setup.secret) : '•••• •••• •••• ••••'}
                </code>
                {setup ? <CopyButton value={setup.secret} /> : null}
              </div>
              {setup ? (
                <a href={setup.otpauth_url} className="text-footnote text-label-blue hover:underline">
                  {t('totp.openApp')}
                </a>
              ) : null}
            </div>
          </div>
        </li>
        <li className="flex flex-col gap-3">
          <p>
            <span className="font-semibold">2.</span> {t('totp.step2')}
          </p>
          <TotpField
            value={code}
            autoFocus
            error={errors.code}
            onChange={(v) => {
              setCode(v)
              setErrors({})
              if (TOTP_RE.test(v)) void verify(v)
            }}
          />
        </li>
      </ol>
    </Frame>
  )
}

function DisableDialog({ email, onClose }: { email: string; onClose: () => void }) {
  const t = useT(['security', 'common'])
  const queryClient = useQueryClient()
  const mapError = useSecurityError()
  const [password, setPassword] = useState('')
  const [code, setCode] = useState('')
  const [errors, setErrors] = useState<ReturnType<typeof mapError>>({})
  const [pending, setPending] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const local: ReturnType<typeof mapError> = {}
    if (!password) local.password = t('password.oldRequired')
    if (!CODE_OR_BACKUP_RE.test(code)) local.code = t('totp.codeOrBackupInvalid')
    setErrors(local)
    if (Object.keys(local).length > 0) return
    setPending(true)
    try {
      await totpDisable({ password, code })
      toast.success(t('totp.disabled'))
      await queryClient.invalidateQueries({ queryKey: securityKey() })
      onClose()
    } catch (error) {
      setErrors(mapError(error))
    } finally {
      setPending(false)
    }
  }

  return (
    <Frame
      title={t('totp.disableTitle')}
      onClose={onClose}
      onSubmit={(e) => void submit(e)}
      footer={
        <>
          <Button variant="tertiary" onPress={onClose}>
            {t('common:actions.cancel')}
          </Button>
          <Button type="submit" variant="danger" isPending={pending}>
            {t('totp.disable')}
          </Button>
        </>
      }
    >
      <p className="text-subhead text-label-secondary">{t('totp.disableHint')}</p>
      <FormError message={errors.form} />
      <UsernameHint email={email} />
      <PlainField
        name="password"
        type="password"
        label={t('password.current')}
        value={password}
        onChange={(v) => {
          setPassword(v)
          setErrors((e) => ({ ...e, password: undefined, form: undefined }))
        }}
        autoComplete="current-password"
        autoFocus
        error={errors.password}
      />
      <TotpField
        value={code}
        allowBackup
        error={errors.code}
        onChange={(v) => {
          setCode(v)
          setErrors((e) => ({ ...e, code: undefined, form: undefined }))
        }}
      />
    </Frame>
  )
}

function RegenerateDialog({
  email,
  appName,
  onClose,
}: {
  email: string
  appName: string
  onClose: () => void
}) {
  const t = useT(['security', 'common'])
  const queryClient = useQueryClient()
  const mapError = useSecurityError()
  const [code, setCode] = useState('')
  const [codes, setCodes] = useState<string[] | null>(null)
  const [errors, setErrors] = useState<ReturnType<typeof mapError>>({})
  const [pending, setPending] = useState(false)
  const [saved, setSaved] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!CODE_OR_BACKUP_RE.test(code)) {
      setErrors({ code: t('totp.codeOrBackupInvalid') })
      return
    }
    setPending(true)
    try {
      const result = await regenerateBackupCodes(code)
      setCodes(result.backup_codes)
      await queryClient.invalidateQueries({ queryKey: securityKey() })
    } catch (error) {
      setErrors(mapError(error))
    } finally {
      setPending(false)
    }
  }

  if (codes) {
    return (
      <Frame
        title={t('backup.title')}
        onClose={onClose}
        dismissable={false}
        footer={
          <div className="flex w-full flex-wrap items-center justify-between gap-3">
            <label className="flex items-center gap-2 text-subhead text-label">
              <input
                type="checkbox"
                checked={saved}
                onChange={(e) => setSaved(e.currentTarget.checked)}
                className="size-4 accent-tint"
              />
              {t('backup.confirmSaved')}
            </label>
            <Button isDisabled={!saved} onPress={onClose}>
              {t('common:actions.close')}
            </Button>
          </div>
        }
      >
        <BackupCodesView codes={codes} email={email} appName={appName} />
      </Frame>
    )
  }

  return (
    <Frame
      title={t('backup.regenerateTitle')}
      onClose={onClose}
      onSubmit={(e) => void submit(e)}
      footer={
        <>
          <Button variant="tertiary" onPress={onClose}>
            {t('common:actions.cancel')}
          </Button>
          <Button type="submit" isPending={pending}>
            {t('backup.regenerate')}
          </Button>
        </>
      }
    >
      <p className="text-subhead text-label-secondary">{t('backup.regenerateHint')}</p>
      <FormError message={errors.form} />
      <TotpField
        value={code}
        allowBackup
        autoFocus
        error={errors.code}
        onChange={(v) => {
          setCode(v)
          setErrors({})
        }}
      />
    </Frame>
  )
}
