import { Chip } from '@heroui/react'
import { useT } from '@/i18n'
import type { Ticket, TicketLevel } from '@/lib/api/types'

import { LEVEL_KEY } from './meta'

const LEVEL_COLOR = { 0: 'default', 1: 'warning', 2: 'danger' } as const

export function TicketLevelChip({ level }: { level: TicketLevel }) {
  const t = useT('tickets')
  return (
    <Chip size="sm" variant="soft" color={LEVEL_COLOR[level]}>
      {t(`level.${LEVEL_KEY[level]}`)}
    </Chip>
  )
}

/** Open + awaiting staff / open + replied / closed. */
export function TicketStatusChip({ ticket }: { ticket: Pick<Ticket, 'status' | 'reply_status'> }) {
  const t = useT('tickets')
  if (ticket.status === 1)
    return (
      <Chip size="sm" variant="soft" color="default">
        {t('status.closed')}
      </Chip>
    )
  return ticket.reply_status === 1 ? (
    <Chip size="sm" variant="soft" color="success">
      {t('status.replied')}
    </Chip>
  ) : (
    <Chip size="sm" variant="soft" color="accent">
      {t('status.waiting')}
    </Chip>
  )
}
