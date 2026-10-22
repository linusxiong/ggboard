/**
 * New ticket (`POST /user/tickets` → 201 Ticket → `/tickets/$id`). Only one ticket may be open at
 * a time: a 409 explains that and links to the open one.
 */
import { Alert, Button, Modal } from '@heroui/react'
import { useQueryClient } from '@tanstack/react-query'
import { Link, useNavigate } from '@tanstack/react-router'
import { useState } from 'react'
import { FormError, useAppForm } from '@/components/form'
import { submitError } from '@/components/form/submitError'
import { useT } from '@/i18n'
import { isApiError, userApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type { ID, TicketLevel } from '@/lib/api/types'
import { buttonClass } from '@/lib/button'
import { LEVEL_KEY, TICKET_LEVELS } from './meta'
import { findOpenTicket } from './openTicket'

interface CreateTicketDialogProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
}

export default function CreateTicketDialog({ isOpen, onOpenChange }: CreateTicketDialogProps) {
  const t = useT(['tickets', 'common'])
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const [formError, setFormError] = useState<string>()
  /** `undefined` = no conflict; `null` = an open ticket exists but its id is unknown. */
  const [openTicket, setOpenTicket] = useState<ID | null | undefined>(undefined)

  const form = useAppForm({
    defaultValues: { subject: '', level: '1', message: '' },
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      try {
        const ticket = await userApi.createTicket({
          subject: value.subject.trim(),
          level: Number(value.level ?? 1) as TicketLevel,
          message: value.message.trim(),
        })
        await queryClient.invalidateQueries({ queryKey: ['user', 'tickets'] })
        void queryClient.invalidateQueries({ queryKey: qk.user.stats() })
        queryClient.setQueryData(qk.user.ticket(ticket.id), ticket)
        onOpenChange(false)
        await navigate({ to: '/tickets/$id', params: { id: String(ticket.id) } })
      } catch (error) {
        if (isApiError(error) && error.status === 409) {
          setOpenTicket(await findOpenTicket())
          return
        }
        setFormError(submitError(formApi, error))
      }
    },
  })

  const required = (message: string) => ({
    onSubmit: ({ value }: { value: string }) => (value.trim() ? undefined : message),
  })

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container placement="auto" size="md" scroll="inside">
        <Modal.Dialog>
          <Modal.CloseTrigger aria-label={t('common:actions.close')} />
          <Modal.Header>
            <Modal.Heading>{t('create.title')}</Modal.Heading>
          </Modal.Header>
          <form
            noValidate
            onSubmit={(e) => {
              e.preventDefault()
              void form.handleSubmit()
            }}
          >
            <Modal.Body className="mt-2 flex flex-col gap-4">
              {openTicket !== undefined ? (
                <Alert status="warning">
                  <Alert.Indicator />
                  <Alert.Content>
                    <Alert.Title>{t('create.conflictTitle')}</Alert.Title>
                    <Alert.Description>{t('create.conflictBody')}</Alert.Description>
                    <div className="mt-3">
                      {openTicket !== null ? (
                        <Link
                          to="/tickets/$id"
                          params={{ id: String(openTicket) }}
                          className={buttonClass({ size: 'sm' })}
                          onClick={() => onOpenChange(false)}
                        >
                          {t('create.goToOpen')}
                        </Link>
                      ) : null}
                    </div>
                  </Alert.Content>
                </Alert>
              ) : null}
              <form.AppField name="subject" validators={required(t('create.subjectRequired'))}>
                {(f) => <f.TextField label={t('create.subject')} isRequired maxLength={255} autoFocus />}
              </form.AppField>
              <form.AppField name="level">
                {(f) => (
                  <f.SelectField
                    label={t('create.level')}
                    options={TICKET_LEVELS.map((level) => ({
                      value: String(level),
                      label: t(`level.${LEVEL_KEY[level]}`),
                    }))}
                  />
                )}
              </form.AppField>
              <form.AppField name="message" validators={required(t('create.messageRequired'))}>
                {(f) => (
                  <f.TextAreaField
                    label={t('create.message')}
                    placeholder={t('create.messagePlaceholder')}
                    rows={6}
                    maxLength={10000}
                    isRequired
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
                <form.SubmitButton size="md">{t('create.submit')}</form.SubmitButton>
              </form.AppForm>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
