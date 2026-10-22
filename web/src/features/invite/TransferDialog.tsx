/** Commission → account balance (`POST /user/commission/transfer {amount}` in cents). */
import { Button, Modal } from '@heroui/react'
import { useQueryClient } from '@tanstack/react-query'
import { useState } from 'react'
import { FormError, useAppForm } from '@/components/form'
import { submitError } from '@/components/form/submitError'
import { useT } from '@/i18n'
import { userApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import { toast } from '@/lib/toast'
import { useAuthStore } from '@/stores/auth'
import { useFormatMoney } from '@/features/shop/site'
import { parseAmount } from './inviteLink'

interface TransferDialogProps {
  isOpen: boolean
  onOpenChange: (open: boolean) => void
  /** Available commission (cents). */
  available: number
}

export default function TransferDialog({ isOpen, onOpenChange, available }: TransferDialogProps) {
  const t = useT(['invite', 'common'])
  const money = useFormatMoney()
  const queryClient = useQueryClient()
  const [formError, setFormError] = useState<string>()

  const form = useAppForm({
    defaultValues: { amount: available > 0 ? available / 100 : null },
    onSubmit: async ({ value, formApi }) => {
      setFormError(undefined)
      const cents = parseAmount(value.amount)
      if (cents === null) return
      try {
        const user = await userApi.transferCommission(cents)
        useAuthStore.getState().setUser(user)
        queryClient.setQueryData(qk.user.me(), user)
        await queryClient.invalidateQueries({ queryKey: qk.user.invite() })
        toast.success(t('transfer.done', { amount: money(cents) }))
        onOpenChange(false)
      } catch (error) {
        setFormError(submitError(formApi, error))
      }
    },
  })

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container placement="auto" size="sm">
        <Modal.Dialog>
          <Modal.CloseTrigger aria-label={t('common:actions.close')} />
          <Modal.Header>
            <Modal.Heading>{t('transfer.title')}</Modal.Heading>
          </Modal.Header>
          <form
            noValidate
            onSubmit={(e) => {
              e.preventDefault()
              void form.handleSubmit()
            }}
          >
            <Modal.Body className="mt-2 flex flex-col gap-4">
              <p className="text-subhead text-label-secondary">
                {t('transfer.body', { available: money(available) })}
              </p>
              <form.AppField
                name="amount"
                validators={{
                  onSubmit: ({ value }) => {
                    const cents = parseAmount(value)
                    if (cents === null) return t('transfer.invalid')
                    if (cents > available) return t('transfer.tooMuch', { available: money(available) })
                    return undefined
                  },
                }}
              >
                {(f) => (
                  <f.NumberField
                    label={t('transfer.amount')}
                    minValue={0.01}
                    maxValue={available / 100}
                    step={0.01}
                    formatOptions={{ minimumFractionDigits: 0, maximumFractionDigits: 2 }}
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
                <form.SubmitButton size="md">{t('transfer.submit')}</form.SubmitButton>
              </form.AppForm>
            </Modal.Footer>
          </form>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
