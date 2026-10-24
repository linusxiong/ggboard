/**
 * Passkeys: list with last use, add (name → `startRegistration` → finish), rename, delete with password.
 * The backend asks for discoverable credentials (`residentKey: required`), so 1Password / Bitwarden /
 * iCloud Keychain / Google Password Manager store a passkey usable for usernameless sign-in.
 */
import { Button, Chip, Modal } from '@heroui/react'
import { useQueryClient } from '@tanstack/react-query'
import { Cloud, KeyRound, Pencil, Plus, Trash2 } from 'lucide-react'
import { type FormEvent, type ReactNode, useState } from 'react'
import { EmptyState } from '@/components/EmptyState'
import { DateTime } from '@/components/Formatters'
import { FormError } from '@/components/FormError'
import { TitleWithHelp } from '@/components/HelpTip'
import { useT } from '@/i18n'
import { isApiError } from '@/lib/api/client'
import type { Passkey } from '@/lib/api/types'
import { deletePasskey, passkeyRegisterBegin, passkeyRegisterFinish, renamePasskey } from '@/lib/api/user'
import { toast } from '@/lib/toast'
import { guessDeviceName, loadWebAuthn, supportsWebAuthn, webAuthnFailure } from './webauthn'
import { securityKey, useSecurityError } from './api'
import { PlainField, UsernameHint } from './shared'
import { ModalClose } from '@/components/feedback/ModalClose'

type Dialog =
  { kind: 'add' } | { kind: 'rename'; passkey: Passkey } | { kind: 'delete'; passkey: Passkey } | null

const NAME_MAX = 64

export function PasskeysCard({
  passkeys,
  email,
  siteEnabled = true,
}: {
  passkeys: readonly Passkey[]
  email: string
  /** `guest/config.passkey_enabled`: false when the site has no WebAuthn RP (no domain in `app_url`). */
  siteEnabled?: boolean
}) {
  const t = useT(['security', 'common'])
  const [dialog, setDialog] = useState<Dialog>(null)
  const browserSupported = supportsWebAuthn()
  const supported = browserSupported && siteEnabled
  const close = () => setDialog(null)

  return (
    <section className="surface-card flex flex-col gap-4 p-6" aria-labelledby="passkeys-title">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div className="flex min-w-0 items-start gap-3">
          <span
            className="flex size-9 shrink-0 items-center justify-center rounded-[10px] bg-fill-tertiary text-label-secondary"
            aria-hidden="true"
          >
            <KeyRound className="size-5" />
          </span>
          <TitleWithHelp help={t('passkeys.hint')} subject={t('passkeys.title')} className="self-center">
            <h2 id="passkeys-title" className="text-headline text-label">
              {t('passkeys.title')}
            </h2>
          </TitleWithHelp>
        </div>
        <Button size="sm" isDisabled={!supported} onPress={() => setDialog({ kind: 'add' })}>
          <Plus className="size-4" aria-hidden="true" />
          {t('passkeys.add')}
        </Button>
      </div>

      {supported ? null : (
        <p className="rounded-[12px] bg-fill-quaternary px-4 py-3 text-subhead text-label" role="note">
          {t(siteEnabled ? 'passkeys.unsupported' : 'passkeys.notConfigured')}
        </p>
      )}

      {passkeys.length === 0 ? (
        <EmptyState compact icon={<KeyRound className="size-5" />} title={t('passkeys.empty')} />
      ) : (
        <ul className="inset-group bg-fill-quaternary!">
          {passkeys.map((passkey) => (
            <li key={passkey.id} className="flex flex-wrap items-center gap-3 px-4 py-3">
              <div className="min-w-0 flex-1">
                <p className="flex flex-wrap items-center gap-2">
                  <span className="truncate text-subhead font-semibold text-label">{passkey.name}</span>
                  {passkey.backup_eligible ? (
                    <Chip size="sm" variant="soft" color="accent">
                      <Cloud className="size-3" aria-hidden="true" />
                      {t('passkeys.synced')}
                    </Chip>
                  ) : null}
                </p>
                <p className="mt-0.5 text-footnote text-label-secondary">
                  {t('passkeys.created')} <DateTime value={passkey.created_at} format="date" />
                  {' · '}
                  {passkey.last_used_at ? (
                    <>
                      {t('passkeys.lastUsed')} <DateTime value={passkey.last_used_at} format="relative" />
                    </>
                  ) : (
                    t('passkeys.neverUsed')
                  )}
                </p>
              </div>
              <div className="flex items-center gap-1">
                <Button
                  isIconOnly
                  size="sm"
                  variant="ghost"
                  aria-label={t('passkeys.rename', { name: passkey.name })}
                  onPress={() => setDialog({ kind: 'rename', passkey })}
                >
                  <Pencil className="size-4" aria-hidden="true" />
                </Button>
                <Button
                  isIconOnly
                  size="sm"
                  variant="ghost"
                  aria-label={t('passkeys.remove', { name: passkey.name })}
                  onPress={() => setDialog({ kind: 'delete', passkey })}
                >
                  <Trash2 className="size-4" aria-hidden="true" />
                </Button>
              </div>
            </li>
          ))}
        </ul>
      )}

      {dialog?.kind === 'add' ? <AddDialog onClose={close} /> : null}
      {dialog?.kind === 'rename' ? <RenameDialog passkey={dialog.passkey} onClose={close} /> : null}
      {dialog?.kind === 'delete' ? (
        <DeleteDialog passkey={dialog.passkey} email={email} onClose={close} />
      ) : null}
    </section>
  )
}

function FormDialog({
  title,
  onClose,
  onSubmit,
  children,
  submitLabel,
  pending,
  danger,
}: {
  title: string
  onClose: () => void
  onSubmit: (event: FormEvent) => void
  children: ReactNode
  submitLabel: string
  pending: boolean
  danger?: boolean
}) {
  const t = useT('common')
  return (
    <Modal.Backdrop
      isOpen
      onOpenChange={(open) => {
        if (!open && !pending) onClose()
      }}
      isDismissable={!pending}
    >
      <Modal.Container size="sm">
        <Modal.Dialog>
          <ModalClose />
          <Modal.Header>
            <Modal.Heading>{title}</Modal.Heading>
          </Modal.Header>
          <form noValidate onSubmit={onSubmit} className="contents">
            <Modal.Body className="flex flex-col gap-4">{children}</Modal.Body>
            <Modal.Footer>
              <Button variant="tertiary" isDisabled={pending} onPress={onClose}>
                {t('actions.cancel')}
              </Button>
              <Button type="submit" variant={danger ? 'danger' : 'primary'} isPending={pending}>
                {submitLabel}
              </Button>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}

function AddDialog({ onClose }: { onClose: () => void }) {
  const t = useT(['security', 'common'])
  const queryClient = useQueryClient()
  const mapError = useSecurityError()
  const [name, setName] = useState(() => guessDeviceName())
  const [error, setError] = useState<string>()
  const [notice, setNotice] = useState<string>()
  const [pending, setPending] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const trimmed = name.trim()
    if (trimmed.length > NAME_MAX) {
      setError(t('passkeys.nameTooLong', { max: NAME_MAX }))
      return
    }
    setError(undefined)
    setNotice(undefined)
    setPending(true)
    try {
      const { startRegistration } = await loadWebAuthn()
      const begin = await passkeyRegisterBegin(trimmed || undefined)
      const credential = await startRegistration({ optionsJSON: begin.options })
      await passkeyRegisterFinish({
        challenge_id: begin.challenge_id,
        credential,
        ...(trimmed ? { name: trimmed } : {}),
      })
      toast.success(t('passkeys.added'))
      await queryClient.invalidateQueries({ queryKey: securityKey() })
      onClose()
    } catch (err) {
      const kind = webAuthnFailure(err)
      if (isApiError(err)) {
        if (err.status === 409) setError(t('passkeys.duplicate'))
        else if (err.status === 401 || err.fieldError('credential') || err.fieldError('challenge_id'))
          setError(t('passkeys.verifyFailed'))
        // 400 without field errors from `begin`: the site has no WebAuthn RP (app_url is not a domain).
        else if (err.status === 400 && Object.keys(err.errors).length === 0)
          setError(t('passkeys.notConfigured'))
        else setError(mapError(err).form ?? mapError(err).name)
      } else if (kind === 'cancelled') setNotice(t('passkeys.cancelled'))
      else if (kind === 'duplicate') setError(t('passkeys.duplicate'))
      else if (kind === 'unsupported') setError(t('passkeys.unsupported'))
      else if (kind === 'security') setError(t('passkeys.security'))
      else setError(err instanceof Error && err.message ? err.message : t('passkeys.failed'))
    } finally {
      setPending(false)
    }
  }

  return (
    <FormDialog
      title={t('passkeys.addTitle')}
      onClose={onClose}
      onSubmit={(e) => void submit(e)}
      submitLabel={t('passkeys.create')}
      pending={pending}
    >
      <p className="text-subhead text-label-secondary">{t('passkeys.addHint')}</p>
      <FormError message={error} />
      {notice ? (
        <p role="status" className="text-footnote text-label-secondary">
          {notice}
        </p>
      ) : null}
      <PlainField
        name="passkey_name"
        label={t('passkeys.name')}
        value={name}
        onChange={(v) => setName(v.slice(0, NAME_MAX))}
        autoComplete="off"
        maxLength={NAME_MAX}
        autoFocus
        placeholder={t('passkeys.namePlaceholder')}
      />
    </FormDialog>
  )
}

function RenameDialog({ passkey, onClose }: { passkey: Passkey; onClose: () => void }) {
  const t = useT(['security', 'common'])
  const queryClient = useQueryClient()
  const mapError = useSecurityError()
  const [name, setName] = useState(passkey.name)
  const [errors, setErrors] = useState<ReturnType<typeof mapError>>({})
  const [pending, setPending] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    const trimmed = name.trim()
    if (!trimmed) {
      setErrors({ name: t('passkeys.nameRequired') })
      return
    }
    setPending(true)
    try {
      await renamePasskey(passkey.id, trimmed)
      await queryClient.invalidateQueries({ queryKey: securityKey() })
      toast.success(t('passkeys.renamed'))
      onClose()
    } catch (error) {
      setErrors(mapError(error))
    } finally {
      setPending(false)
    }
  }

  return (
    <FormDialog
      title={t('passkeys.renameTitle')}
      onClose={onClose}
      onSubmit={(e) => void submit(e)}
      submitLabel={t('common:actions.save')}
      pending={pending}
    >
      <FormError message={errors.form} />
      <PlainField
        name="passkey_name"
        label={t('passkeys.name')}
        value={name}
        onChange={(v) => {
          setName(v.slice(0, NAME_MAX))
          setErrors({})
        }}
        autoComplete="off"
        maxLength={NAME_MAX}
        autoFocus
        error={errors.name}
      />
    </FormDialog>
  )
}

function DeleteDialog({ passkey, email, onClose }: { passkey: Passkey; email: string; onClose: () => void }) {
  const t = useT(['security', 'common'])
  const queryClient = useQueryClient()
  const mapError = useSecurityError()
  const [password, setPassword] = useState('')
  const [errors, setErrors] = useState<ReturnType<typeof mapError>>({})
  const [pending, setPending] = useState(false)

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    if (!password) {
      setErrors({ password: t('password.oldRequired') })
      return
    }
    setPending(true)
    try {
      await deletePasskey(passkey.id, password)
      await queryClient.invalidateQueries({ queryKey: securityKey() })
      toast.success(t('passkeys.removed'))
      onClose()
    } catch (error) {
      setErrors(mapError(error))
    } finally {
      setPending(false)
    }
  }

  return (
    <FormDialog
      title={t('passkeys.removeTitle', { name: passkey.name })}
      onClose={onClose}
      onSubmit={(e) => void submit(e)}
      submitLabel={t('passkeys.removeConfirm')}
      pending={pending}
      danger
    >
      <p className="text-subhead text-label-secondary">{t('passkeys.removeHint')}</p>
      <FormError message={errors.form} />
      <UsernameHint email={email} />
      <PlainField
        name="password"
        type="password"
        label={t('password.current')}
        value={password}
        onChange={(v) => {
          setPassword(v)
          setErrors({})
        }}
        autoComplete="current-password"
        autoFocus
        error={errors.password}
      />
    </FormDialog>
  )
}
