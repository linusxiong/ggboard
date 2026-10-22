import { Chip, ToggleButton, ToggleButtonGroup } from '@heroui/react'
import { type KeyboardEvent, useRef } from 'react'
import { TablePagination } from '@/components/data-table/DataTable'
import { DateTime } from '@/components/Formatters'
import { DebouncedSearch } from '@/features/admin/orders/DebouncedSearch'
import { useT } from '@/i18n'
import type { AdminTicket, ID, Page } from '@/lib/api/types'
import { cx } from '@/lib/cn'
import { LEVEL_COLOR } from './query'
import { TICKET_VIEWS, type TicketSearch, type TicketView } from './search'

interface TicketListProps {
  search: TicketSearch
  data: Page<AdminTicket> | undefined
  isLoading: boolean
  isFetching: boolean
  awaitingCount?: number
  onSearchChange: (patch: Partial<TicketSearch>) => void
  onCommitQuery: (q: string | undefined) => void
  className?: string
}

/** Master list: view switch, user search, keyboard-navigable ticket rows (↑/↓, Home/End). */
export function TicketList({
  search,
  data,
  isLoading,
  isFetching,
  awaitingCount,
  onSearchChange,
  onCommitQuery,
  className,
}: TicketListProps) {
  const t = useT(['admin-support', 'common'])
  const listRef = useRef<HTMLUListElement>(null)

  const onKeyDown = (e: KeyboardEvent<HTMLUListElement>) => {
    const items = Array.from(
      listRef.current?.querySelectorAll<HTMLButtonElement>('button[data-ticket]') ?? [],
    )
    const index = items.indexOf(document.activeElement as HTMLButtonElement)
    let next = -1
    if (e.key === 'ArrowDown') next = Math.min(items.length - 1, index + 1)
    else if (e.key === 'ArrowUp') next = Math.max(0, index - 1)
    else if (e.key === 'Home') next = 0
    else if (e.key === 'End') next = items.length - 1
    if (next >= 0) {
      e.preventDefault()
      items[next]?.focus()
    }
  }

  const select = (id: ID) => onSearchChange({ id })

  return (
    <section
      aria-label={t('tickets.listLabel')}
      className={cx('surface-card flex min-h-0 flex-col', className)}
    >
      <div className="flex flex-col gap-3 p-3 hairline-b">
        <ToggleButtonGroup
          aria-label={t('tickets.viewLabel')}
          size="sm"
          fullWidth
          selectionMode="single"
          disallowEmptySelection
          selectedKeys={[search.view]}
          onSelectionChange={(keys) => {
            const view = [...keys][0] as TicketView | undefined
            if (view && view !== search.view) onSearchChange({ view, page: 1 })
          }}
        >
          {TICKET_VIEWS.map((v, i) => (
            <ToggleButton key={v} id={v}>
              {i > 0 ? <ToggleButtonGroup.Separator /> : null}
              {t(`tickets.views.${v}`)}
              {v === 'awaiting' && awaitingCount ? (
                <span className="ms-1 rounded-full bg-accent px-1.5 text-caption2 text-accent-foreground tabular">
                  {awaitingCount}
                </span>
              ) : null}
            </ToggleButton>
          ))}
        </ToggleButtonGroup>
        <DebouncedSearch
          label={t('tickets.search')}
          hideLabel
          value={search.q}
          onCommit={onCommitQuery}
          placeholder={t('tickets.searchPlaceholder')}
        />
      </div>

      {isLoading && !data ? (
        <div className="flex flex-col gap-2 p-3" aria-busy="true">
          {[0, 1, 2, 3].map((i) => (
            <div key={i} className="h-16 animate-pulse rounded-[12px] bg-fill-tertiary" />
          ))}
        </div>
      ) : data && data.items.length === 0 ? (
        <p className="p-6 text-center text-subhead text-label-secondary">{t('tickets.empty')}</p>
      ) : (
        <ul
          ref={listRef}
          onKeyDown={onKeyDown}
          aria-label={t('tickets.listLabel')}
          className={cx('min-h-0 flex-1 overflow-auto p-2', isFetching && 'opacity-70 transition-opacity')}
        >
          {data?.items.map((ticket) => {
            const awaiting = ticket.status === 0 && ticket.reply_status === 0
            const active = ticket.id === search.id
            return (
              <li key={ticket.id}>
                <button
                  type="button"
                  data-ticket={ticket.id}
                  aria-current={active ? 'true' : undefined}
                  onClick={() => select(ticket.id)}
                  className={cx(
                    'flex w-full flex-col gap-1 rounded-[12px] px-3 py-2.5 text-start outline-none transition-colors',
                    'focus-visible:ring-2 focus-visible:ring-focus',
                    active ? 'bg-fill-secondary' : 'hover:bg-fill-quaternary',
                  )}
                >
                  <span className="flex items-center gap-2">
                    {awaiting ? (
                      <span className="size-2 shrink-0 rounded-full bg-system-blue" aria-hidden />
                    ) : null}
                    <span
                      className={cx(
                        'min-w-0 flex-1 truncate text-subhead text-label',
                        awaiting && 'font-semibold',
                      )}
                    >
                      {ticket.subject}
                    </span>
                    <DateTime
                      value={ticket.updated_at}
                      format="relative"
                      className="shrink-0 text-caption text-label-secondary"
                    />
                  </span>
                  <span className="flex flex-wrap items-center gap-1.5 text-footnote text-label-secondary">
                    <span className="min-w-0 truncate">{ticket.user?.email ?? `#${ticket.user_id}`}</span>
                    <Chip size="sm" variant="soft" color={LEVEL_COLOR[ticket.level] ?? 'default'}>
                      {t(`tickets.level.${ticket.level}`)}
                    </Chip>
                    {ticket.status === 1 ? (
                      <Chip size="sm" variant="soft">
                        {t('tickets.status.1')}
                      </Chip>
                    ) : awaiting ? (
                      <span className="sr-only">{t('tickets.awaiting')}</span>
                    ) : (
                      <Chip size="sm" variant="soft" color="success">
                        {t('tickets.replied')}
                      </Chip>
                    )}
                  </span>
                </button>
              </li>
            )
          })}
        </ul>
      )}

      {data && data.total > search.page_size ? (
        <div className="hairline-t">
          <TablePagination
            page={search.page}
            pageCount={Math.max(1, Math.ceil(data.total / search.page_size))}
            pageSize={search.page_size}
            total={data.total}
            onPageChange={(p) => onSearchChange({ page: p })}
          />
        </div>
      ) : null}
    </section>
  )
}
