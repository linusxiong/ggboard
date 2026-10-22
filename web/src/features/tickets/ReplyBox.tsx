/** Reply composer: plain-text area + send (⌘/Ctrl + Enter also sends). */
import { Button, Label, TextArea, TextField } from '@heroui/react'
import { useMutation, useQueryClient } from '@tanstack/react-query'
import { SendHorizontal } from 'lucide-react'
import { useState } from 'react'
import { FormError } from '@/components/FormError'
import { useT } from '@/i18n'
import { userApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type { ID } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'

export function ReplyBox({ ticketId }: { ticketId: ID }) {
  const t = useT('tickets')
  const queryClient = useQueryClient()
  const [text, setText] = useState('')
  const [error, setError] = useState<string>()

  const reply = useMutation({
    mutationFn: (message: string) => userApi.replyTicket(ticketId, message),
    onSuccess: async (ticket) => {
      setText('')
      setError(undefined)
      if (ticket.messages) queryClient.setQueryData(qk.user.ticket(ticketId), ticket)
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: qk.user.ticket(ticketId) }),
        queryClient.invalidateQueries({ queryKey: ['user', 'tickets'] }),
      ])
    },
    onError: (e) => setError(errorMessage(e)),
  })

  const send = () => {
    const message = text.trim()
    if (!message) {
      setError(t('detail.replyRequired'))
      return
    }
    reply.mutate(message)
  }

  return (
    <form
      className="flex flex-col gap-2 border-t-[0.5px] border-hairline p-4 sm:px-6"
      noValidate
      onSubmit={(e) => {
        e.preventDefault()
        send()
      }}
    >
      <TextField
        name="message"
        value={text}
        onChange={(value) => {
          setText(value)
          if (error) setError(undefined)
        }}
        isInvalid={Boolean(error)}
        validationBehavior="aria"
        fullWidth
      >
        <Label className="sr-only">{t('detail.reply')}</Label>
        <TextArea
          rows={3}
          maxLength={10000}
          placeholder={t('detail.replyPlaceholder')}
          className="w-full resize-y"
          onKeyDown={(e) => {
            if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
              e.preventDefault()
              send()
            }
          }}
        />
      </TextField>
      <FormError message={error} />
      <div className="flex items-center justify-between gap-3">
        <span className="hidden text-caption text-label-secondary sm:inline">{t('detail.shortcut')}</span>
        <Button type="submit" isPending={reply.isPending} className="ms-auto">
          <SendHorizontal className="size-4" aria-hidden="true" />
          {t('detail.send')}
        </Button>
      </div>
    </form>
  )
}
