/**
 * Commission withdrawal (`POST /user/commission/withdraw {method, account}` → 201 `{ticket_id}`):
 * the request is a support ticket that staff settle by hand. Handles "withdrawals closed" (403),
 * "an open ticket exists" (409, links to it) and the minimum-amount / method errors (400).
 */
import { Alert, Button, Modal } from '@heroui/react'
import { useQueryClient } from '@tanstack/react-query'
import { Link } from '@tanstack/react-router'
import { CircleCheck } from 'lucide-react'
import { useState } from 'react'
import { FormError, useAppForm } from '@/components/form'
import { submitError } from '@/components/form/submitError'
import { useT } from '@/i18n'
import { isApiError, userApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type { ID } from '@/lib/api/types'
import { buttonClass } from '@/lib/button'
import { useFormatMoney } from '@/features/shop/site'
import { WITHDRAW_METHOD_SUGGESTIONS } from './inviteLink'
import { findOpenTicket } from '@/features/tickets/openTicket'

interface WithdrawDialogProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  available: number
}

type Outcome = { kind: 'created'; ticketId: ID } | { kind: 'closed' } | { kind: 'open'; ticketId: ID | null }

export default function WithdrawDialog({ isOpen, onOpenChange, available }: WithdrawDialogProps) {
  const t = useT(['invite', 'common'])
  const money = useFormatMoney()
  const queryClient = useQueryClient()
  const [formError, setFormError] = useState<string>()
  const [outcome, setOutcome] = useState<Outcome | null>(null)

  const form = useAppForm({
    defaultValues: { method: '', account: '' },
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      try {
        const { ticket_id } = await userApi.withdrawCommission({
          method: value.method.trim(),
          account: value.account.trim(),
        })
        setOutcome({ kind: 'created', ticketId: ticket_id })
        void queryClient.invalidateQueries({ queryKey: ['user', 'tickets'] })
        void queryClient.invalidateQueries({ queryKey: qk.user.stats() })
      } catch (error) {
        if (isApiError(error) && error.status === 403) {
          setOutcome({ kind: 'closed' })
          return
        }
        if (isApiError(error) && error.status === 409) {
          setOutcome({ kind: 'open', ticketId: await findOpenTicket() })
          return
        }
        setFormError(submitError(formApi, error))
      }
    },
  })

  const close = () => onOpenChange(false)

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container placement="auto" size="sm">
        <Modal.Dialog>
          <Modal.CloseTrigger aria-label={t('common:actions.close')} />
          <Modal.Header>
            <Modal.Heading>{t('withdraw.title')}</Modal.Heading>
          </Modal.Header>
          {outcome?.kind === 'created' ? (
            <>
              <Modal.Body className="flex flex-col items-center gap-3 py-2 text-center" role="status">
                <CircleCheck className="size-12 text-system-green" aria-hidden="true" />
                <p className="text-headline text-label">{t('withdraw.created')}</p>
                <p className="text-subhead text-label-secondary">{t('withdraw.createdBody')}</p>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="tertiary" onPress={close}>
                  {t('common:actions.close')}
                </Button>
                <Link
                  to="/tickets/$id"
                  params={{ id: String(outcome.ticketId) }}
                  className={buttonClass()}
                  onClick={close}
                >
                  {t('withdraw.viewTicket')}
                </Link>
              </Modal.Footer>
            </>
          ) : outcome ? (
            <>
              <Modal.Body>
                <Alert status={outcome.kind === 'closed' ? 'danger' : 'warning'}>
                  <Alert.Indicator />
                  <Alert.Content>
                    <Alert.Title>
                      {outcome.kind === 'closed' ? t('withdraw.closedTitle') : t('withdraw.openTitle')}
                    </Alert.Title>
                    <Alert.Description>
                      {outcome.kind === 'closed' ? t('withdraw.closedBody') : t('withdraw.openBody')}
                    </Alert.Description>
                  </Alert.Content>
                </Alert>
              </Modal.Body>
              <Modal.Footer>
                <Button variant="tertiary" onPress={close}>
                  {t('common:actions.close')}
                </Button>
                {outcome.kind === 'open' ? (
                  outcome.ticketId !== null ? (
                    <Link
                      to="/tickets/$id"
                      params={{ id: String(outcome.ticketId) }}
                      className={buttonClass()}
                      onClick={close}
                    >
                      {t('withdraw.viewOpenTicket')}
                    </Link>
                  ) : (
                    <Link to="/tickets" className={buttonClass()} onClick={close}>
                      {t('withdraw.viewTickets')}
                    </Link>
                  )
                ) : null}
              </Modal.Footer>
            </>
          ) : (
            <form
              noValidate
              onSubmit={(e) => {
                e.preventDefault()
                void form.handleSubmit()
              }}
            >
              <Modal.Body className="flex flex-col gap-4">
                <p className="text-subhead text-label-secondary">
                  {t('withdraw.body', { available: money(available) })}
                </p>
                <form.AppField
                  name="method"
                  validators={{
                    onSubmit: ({ value }) => (value.trim() ? undefined : t('withdraw.methodRequired')),
                  }}
                >
                  {(f) => (
                    <div className="flex flex-col gap-2">
                      <f.TextField
                        label={t('withdraw.method')}
                        placeholder={t('withdraw.methodPlaceholder')}
                        isRequired
                        maxLength={64}
                      />
                      <div
                        className="flex flex-wrap gap-2"
                        role="group"
                        aria-label={t('withdraw.suggestions')}
                      >
                        {WITHDRAW_METHOD_SUGGESTIONS.map((method) => (
                          <Button
                            key={method}
                            size="sm"
                            variant={f.state.value === method ? 'primary' : 'tertiary'}
                            aria-pressed={f.state.value === method}
                            onPress={() => f.handleChange(method)}
                          >
                            {method}
                          </Button>
                        ))}
                      </div>
                    </div>
                  )}
                </form.AppField>
                <form.AppField
                  name="account"
                  validators={{
                    onSubmit: ({ value }) => (value.trim() ? undefined : t('withdraw.accountRequired')),
                  }}
                >
                  {(f) => (
                    <f.TextField
                      label={t('withdraw.account')}
                      placeholder={t('withdraw.accountPlaceholder')}
                      autoComplete="off"
                      isRequired
                      maxLength={255}
                    />
                  )}
                </form.AppField>
                <FormError message={formError} />
              </Modal.Body>
              <Modal.Footer className="mt-5">
                <Button slot="close" variant="tertiary">
                  {t('common:actions.cancel')}
                </Button>
                <form.AppForm>
                  <form.SubmitButton size="md">{t('withdraw.submit')}</form.SubmitButton>
                </form.AppForm>
              </Modal.Footer>
            </form>
          )}
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
