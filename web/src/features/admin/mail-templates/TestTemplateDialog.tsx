import { Alert, Button, Modal, Spinner } from '@heroui/react'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { ModalClose } from '@/components/feedback/ModalClose'
import { TextInput } from '@/features/admin/settings/inputs/Controlled'
import { useT } from '@/i18n'
import { adminApi, type ApiError, isApiError } from '@/lib/api'
import type { MailTemplateDraft } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { useAuthStore } from '@/stores/auth'

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

interface TestTemplateDialogProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  name: string
  /** Human title of the template (dialog subtitle). */
  title: string
  /** The editor's current content (sent as is, saved or not). */
  draft: MailTemplateDraft
  /** The server rejected the draft itself (`errors.subject` / `errors.body` …): show them on the editor. */
  onDraftErrors: (error: ApiError) => void
}

/**
 * `POST /admin/mail-templates/:name/test {locale, subject, body, email}`: renders the current draft with the
 * sample data and sends it synchronously with the saved mail settings. A delivery failure comes back as
 * `errors.email` (the driver's message) and is shown under the address.
 */
export default function TestTemplateDialog({
  isOpen,
  onOpenChange,
  name,
  title,
  draft,
  onDraftErrors,
}: TestTemplateDialogProps) {
  const t = useT(['admin-settings', 'common'])
  const me = useAuthStore((s) => s.user?.email ?? '')
  const [email, setEmail] = useState(me)
  const [error, setError] = useState<string>()
  const [draftError, setDraftError] = useState<string>()
  const send = useMutation({
    mutationFn: (to: string) => adminApi.testMailTemplate(name, { ...draft, email: to }),
    onSuccess: (_, to) => {
      toast.success(t('mailTemplates.test.sent', { email: to }))
      onOpenChange(false)
    },
    onError: (e) => {
      const emailError = isApiError(e) ? e.fieldError('email') : undefined
      const others = isApiError(e) ? Object.keys(e.errors).filter((k) => k !== 'email') : []
      if (others.length && isApiError(e)) {
        onDraftErrors(e)
        setDraftError(
          t('mailTemplates.test.draftInvalid', { message: e.fieldError(others[0]!) ?? e.message }),
        )
      }
      if (emailError || !others.length) setError(emailError ?? errorMessage(e))
    },
  })
  const submit = () => {
    setDraftError(undefined)
    if (!EMAIL_RE.test(email.trim())) {
      setError(t('mailTemplates.test.invalid'))
      return
    }
    setError(undefined)
    send.mutate(email.trim())
  }
  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="sm">
        <Modal.Dialog>
          <ModalClose />
          <Modal.Header>
            <Modal.Heading>{t('mailTemplates.test.title')}</Modal.Heading>
            <p className="text-subhead text-label-secondary">{title}</p>
          </Modal.Header>
          <form
            noValidate
            onSubmit={(e) => {
              e.preventDefault()
              // React events bubble through portals: never let this submit reach an enclosing form.
              e.stopPropagation()
              submit()
            }}
          >
            <Modal.Body className="flex flex-col gap-4">
              {draftError ? (
                <Alert status="danger">
                  <Alert.Indicator />
                  <Alert.Content>
                    <Alert.Description>{draftError}</Alert.Description>
                  </Alert.Content>
                </Alert>
              ) : null}
              <TextInput
                type="email"
                label={t('mailTemplates.test.to')}
                name="test_email"
                value={email}
                onChange={(v) => {
                  setEmail(v)
                  setError(undefined)
                }}
                autoComplete="email"
                inputMode="email"
                autoFocus
                isRequired
                error={error}
                description={t('mailTemplates.test.hint')}
              />
            </Modal.Body>
            <Modal.Footer>
              <Button slot="close" variant="secondary">
                {t('common:actions.cancel')}
              </Button>
              <Button type="submit" isPending={send.isPending}>
                {({ isPending }) => (
                  <>
                    {isPending ? <Spinner color="current" size="sm" /> : null}
                    {t('mailTemplates.test.send')}
                  </>
                )}
              </Button>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
