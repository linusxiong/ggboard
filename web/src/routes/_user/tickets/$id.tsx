import { Button } from '@heroui/react'
import { useMutation, useQueryClient, useSuspenseQuery } from '@tanstack/react-query'
import { createFileRoute, Link } from '@tanstack/react-router'
import { ChevronLeft, Lock } from 'lucide-react'
import { confirm } from '@/components/feedback/confirm'
import { DateTime } from '@/components/Formatters'
import { PageShell } from '@/components/PageShell'
import { TicketLevelChip, TicketStatusChip } from '@/features/tickets/labels'
import { parseTicketId } from '@/features/tickets/meta'
import { ReplyBox } from '@/features/tickets/ReplyBox'
import { TicketThread } from '@/features/tickets/TicketThread'
import { loadNamespaces, useT } from '@/i18n'
import * as userApi from '@/lib/api/user'
import { qk, ticketQuery } from '@/lib/api/queries'
import { buttonClass } from '@/lib/button'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'

/** Staff replies show up without a reload while the ticket is open. */
const REFRESH_MS = 20_000

export const Route = createFileRoute('/_user/tickets/$id')({
  // Loaders stay in the entry chunk (only components are split): import the query factories lazily.
  loader: async ({ context, params }) => {
    const id = parseTicketId(params.id)
    const q = await import('@/lib/api/queries')
    return Promise.all([loadNamespaces(['tickets', 'common']), context.queryClient.query(q.ticketQuery(id))])
  },
  component: TicketDetailPage,
})

function TicketDetailPage() {
  const t = useT(['tickets', 'common'])
  const id = parseTicketId(Route.useParams().id)
  const queryClient = useQueryClient()
  const { data: ticket } = useSuspenseQuery({
    ...ticketQuery(id),
    refetchInterval: (q) => (q.state.data?.status === 0 ? REFRESH_MS : false),
  })
  const open = ticket.status === 0

  const close = useMutation({
    mutationFn: () => userApi.closeTicket(id),
    onSuccess: async () => {
      toast.success(t('detail.closed'))
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: qk.user.ticket(id) }),
        queryClient.invalidateQueries({ queryKey: ['user', 'tickets'] }),
        queryClient.invalidateQueries({ queryKey: qk.user.stats() }),
      ])
    },
    onError: (e) => toast.danger(errorMessage(e)),
  })

  return (
    <PageShell
      title={ticket.subject}
      description={
        <span className="flex flex-wrap items-center gap-2">
          <TicketStatusChip ticket={ticket} />
          <TicketLevelChip level={ticket.level} />
          <span className="text-footnote">
            {t('detail.createdAt')} <DateTime value={ticket.created_at} />
          </span>
        </span>
      }
      actions={
        <>
          <Link to="/tickets" className={buttonClass({ variant: 'ghost', size: 'sm' })}>
            <ChevronLeft className="size-4" aria-hidden="true" />
            {t('detail.back')}
          </Link>
          {open ? (
            <Button
              size="sm"
              variant="danger-soft"
              isPending={close.isPending}
              onPress={async () => {
                const ok = await confirm({
                  title: t('detail.closeTitle'),
                  description: t('detail.closeBody'),
                  confirmLabel: t('detail.close'),
                  tone: 'danger',
                })
                if (ok) close.mutate()
              }}
            >
              {t('detail.close')}
            </Button>
          ) : null}
        </>
      }
    >
      <section className="surface-card overflow-hidden" aria-label={t('detail.conversation')}>
        <TicketThread messages={ticket.messages ?? []} />
        {open ? (
          <ReplyBox ticketId={id} />
        ) : (
          <div className="flex flex-wrap items-center justify-between gap-3 border-t-[0.5px] border-hairline px-4 py-4 sm:px-6">
            <p className="flex items-center gap-2 text-subhead text-label-secondary">
              <Lock className="size-4" aria-hidden="true" />
              {t('detail.closedHint')}
            </p>
            <Link
              to="/tickets"
              search={{ new: 1 }}
              className={buttonClass({ size: 'sm', variant: 'secondary' })}
            >
              {t('list.new')}
            </Link>
          </div>
        )}
      </section>
    </PageShell>
  )
}
