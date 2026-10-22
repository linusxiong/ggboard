import { Alert, Button, Chip, Kbd, Label, Skeleton, Spinner, TextArea, TextField } from '@heroui/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { ArrowLeft, Lock, SendHorizontal } from 'lucide-react'
import { useEffect, useRef, useState } from 'react'
import { DateTime } from '@/components/Formatters'
import { confirm } from '@/components/feedback/confirm'
import { useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { adminTicketQuery, qk } from '@/lib/api/queries'
import type { AdminTicket, ID } from '@/lib/api/types'
import { cx } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { LEVEL_COLOR } from './query'

interface TicketThreadProps {
  id: ID
  onBack: () => void
  className?: string
}

const isMac =
  typeof navigator !== 'undefined' && /Mac|iP(hone|ad|od)/.test(navigator.platform || navigator.userAgent)

/** Chat-style thread with reply box (⌘/Ctrl+Enter sends) and close; a staff reply reopens a closed ticket. */
export function TicketThread({ id, onBack, className }: TicketThreadProps) {
  const t = useT(['admin-support', 'common'])
  const qc = useQueryClient()
  const q = useQuery({ ...adminTicketQuery(id), refetchInterval: 30_000 })
  const ticket = q.data
  const [draft, setDraft] = useState('')
  const scroller = useRef<HTMLDivElement>(null)
  const replyRef = useRef<HTMLTextAreaElement>(null)
  const count = ticket?.messages?.length ?? 0

  // Keep the newest message in view when the thread loads or grows.
  useEffect(() => {
    const el = scroller.current
    if (el) el.scrollTop = el.scrollHeight
  }, [count, id])

  const refresh = async (next?: AdminTicket) => {
    if (next) qc.setQueryData(qk.admin.ticket(id), (prev: AdminTicket | undefined) => ({ ...prev, ...next }))
    await Promise.all([
      qc.invalidateQueries({ queryKey: qk.admin.ticket(id) }),
      qc.invalidateQueries({ queryKey: qk.admin.tickets() }),
    ])
  }

  const reply = useMutation({
    mutationFn: (message: string) => adminApi.replyTicket(id, message),
    onSuccess: async (next) => {
      setDraft('')
      toast.success(t('tickets.replySent'))
      await refresh(next as AdminTicket)
    },
    onError: (e) => toast.danger(errorMessage(e)),
  })

  const close = async () => {
    const ok = await confirm({
      title: t('tickets.closeTitle'),
      description: t('tickets.closeHint'),
      confirmLabel: t('tickets.close'),
    })
    if (!ok) return
    try {
      await adminApi.closeTicket(id)
      toast.success(t('tickets.closed'))
      await refresh()
    } catch (e) {
      toast.danger(errorMessage(e))
    }
  }

  const send = () => {
    const message = draft.trim()
    if (message && !reply.isPending) reply.mutate(message)
  }

  return (
    <section
      aria-label={ticket ? t('tickets.threadLabel', { subject: ticket.subject }) : t('tickets.thread')}
      className={cx('surface-card flex min-h-0 flex-col', className)}
      onKeyDown={(e) => {
        if (e.key === 'Escape' && !e.defaultPrevented) onBack()
      }}
    >
      <header className="flex flex-wrap items-start gap-3 p-4 hairline-b">
        <Button
          size="sm"
          variant="ghost"
          isIconOnly
          aria-label={t('tickets.back')}
          onPress={onBack}
          className="md:hidden"
        >
          <ArrowLeft className="size-4" aria-hidden />
        </Button>
        {ticket ? (
          <div className="min-w-0 flex-1">
            <h2 className="text-headline text-label break-words">{ticket.subject}</h2>
            <p className="mt-1 flex flex-wrap items-center gap-1.5 text-footnote text-label-secondary">
              <span>#{ticket.id}</span>
              <span aria-hidden>·</span>
              <span className="break-all">{ticket.user?.email ?? `#${ticket.user_id}`}</span>
              <span aria-hidden>·</span>
              <DateTime value={ticket.created_at} />
              <Chip size="sm" variant="soft" color={LEVEL_COLOR[ticket.level] ?? 'default'}>
                {t(`tickets.level.${ticket.level}`)}
              </Chip>
              <Chip size="sm" variant="soft" color={ticket.status === 0 ? 'success' : 'default'}>
                {t(`tickets.status.${ticket.status}`)}
              </Chip>
            </p>
          </div>
        ) : (
          <div className="flex-1">
            <Skeleton className="h-5 w-2/3 rounded" />
            <Skeleton className="mt-2 h-4 w-1/2 rounded" />
          </div>
        )}
        {ticket?.status === 0 ? (
          <Button size="sm" variant="secondary" onPress={() => void close()}>
            <Lock className="size-4" aria-hidden />
            {t('tickets.close')}
          </Button>
        ) : null}
      </header>

      <div
        ref={scroller}
        className="min-h-48 flex-1 overflow-auto px-4 py-4"
        aria-live="polite"
        aria-busy={q.isPending}
      >
        {q.isError ? (
          <p className="text-subhead text-label-red">{errorMessage(q.error)}</p>
        ) : !ticket ? (
          <div className="flex flex-col gap-3">
            <Skeleton className="h-14 w-2/3 rounded-[18px]" />
            <Skeleton className="ms-auto h-14 w-1/2 rounded-[18px]" />
          </div>
        ) : (
          <ol className="flex flex-col gap-3">
            {(ticket.messages ?? []).map((m) => (
              <li key={m.id} className={cx('flex flex-col gap-1', m.is_me ? 'items-end' : 'items-start')}>
                <span className="sr-only">{m.is_me ? t('tickets.staff') : t('tickets.user')}</span>
                <div
                  className={cx(
                    'max-w-[85%] rounded-[18px] px-3.5 py-2 text-body whitespace-pre-wrap break-words',
                    m.is_me
                      ? 'rounded-br-[6px] bg-accent text-accent-foreground'
                      : 'rounded-bl-[6px] bg-fill-secondary text-label',
                  )}
                >
                  {m.message}
                </div>
                <DateTime value={m.created_at} className="px-1 text-caption text-label-secondary" />
              </li>
            ))}
          </ol>
        )}
      </div>

      <form
        className="flex flex-col gap-2 p-3 hairline-t"
        onSubmit={(e) => {
          e.preventDefault()
          send()
        }}
      >
        {ticket?.status === 1 ? (
          <Alert status="default">
            <Alert.Indicator />
            <Alert.Content>
              <Alert.Description>{t('tickets.closedHint')}</Alert.Description>
            </Alert.Content>
          </Alert>
        ) : null}
        <TextField value={draft} onChange={setDraft} fullWidth isDisabled={!ticket}>
          <Label className="sr-only">{t('tickets.reply')}</Label>
          <TextArea
            ref={replyRef}
            rows={3}
            placeholder={t('tickets.replyPlaceholder')}
            className="w-full"
            onKeyDown={(e) => {
              if (e.key === 'Enter' && (e.metaKey || e.ctrlKey)) {
                e.preventDefault()
                send()
              } else if (e.key === 'Escape') {
                e.preventDefault()
                replyRef.current?.blur()
              }
            }}
          />
        </TextField>
        <div className="flex items-center justify-between gap-2">
          <span className="text-caption text-label-secondary">
            <Kbd>{isMac ? '⌘' : 'Ctrl'}</Kbd> + <Kbd>Enter</Kbd> {t('tickets.toSend')}
          </span>
          <Button type="submit" size="sm" isDisabled={!draft.trim() || !ticket} isPending={reply.isPending}>
            {({ isPending }) => (
              <>
                {isPending ? (
                  <Spinner color="current" size="sm" />
                ) : (
                  <SendHorizontal className="size-4" aria-hidden />
                )}
                {ticket?.status === 1 ? t('tickets.replyReopen') : t('tickets.send')}
              </>
            )}
          </Button>
        </div>
      </form>
    </section>
  )
}
