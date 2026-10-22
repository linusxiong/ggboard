import { Alert, Button, Modal, Spinner } from '@heroui/react'
import { useMutation } from '@tanstack/react-query'
import { useState } from 'react'
import { useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { useAuthStore } from '@/stores/auth'
import { TextInput } from './inputs/Controlled'
import { ModalClose } from '@/components/feedback/ModalClose'

interface TestMailDialogProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  /** The email group has unsaved edits (the test uses saved settings). */
  unsaved: boolean
}

const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/

/** `POST /admin/settings/mail/test {email}` — sends a test message with the saved mail settings. */
export function TestMailDialog({ isOpen, onOpenChange, unsaved }: TestMailDialogProps) {
  const t = useT(['admin-settings', 'common'])
  const me = useAuthStore((s) => s.user?.email ?? '')
  const [email, setEmail] = useState(me)
  const [error, setError] = useState<string>()
  const send = useMutation({
    mutationFn: (to: string) => adminApi.testMail(to),
    onSuccess: () => {
      toast.success(t('email.testSent', { email }))
      onOpenChange(false)
    },
    onError: (e) => setError(errorMessage(e)),
  })
  const submit = () => {
    if (!EMAIL_RE.test(email.trim())) {
      setError(t('email.testInvalid'))
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
            <Modal.Heading>{t('email.testTitle')}</Modal.Heading>
          </Modal.Header>
          <form
            noValidate
            onSubmit={(e) => {
              e.preventDefault()
              submit()
            }}
          >
            <Modal.Body className="flex flex-col gap-4">
              {unsaved ? (
                <Alert status="warning">
                  <Alert.Indicator />
                  <Alert.Content>
                    <Alert.Description>{t('email.testUnsaved')}</Alert.Description>
                  </Alert.Content>
                </Alert>
              ) : null}
              <TextInput
                type="email"
                label={t('email.testTo')}
                name="test_email"
                value={email}
                onChange={(v) => {
                  setEmail(v)
                  setError(undefined)
                }}
                autoComplete="email"
                autoFocus
                isRequired
                error={error}
                description={t('email.testHint')}
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
                    {t('email.testSend')}
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
