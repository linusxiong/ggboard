/**
 * Subscription access log rows (`[{ip, location, client, format, at}]`), shared by the user's security card
 * (masked IPs) and the admin user drawer (full IPs). The location (resolved from the full IP) is secondary text
 * after the IP; it wraps under it on narrow screens and is truncated with the full text in `title`. Labels come
 * from the caller so either i18n namespace works.
 */
import { Chip } from '@heroui/react'
import { DateTime } from '@/components/Formatters'
import type { SubscriptionAccess } from '@/lib/api/types'
import { clientLabel } from './accessLog'

export function AccessLogList({
  entries,
  labels,
  className,
}: {
  entries: readonly SubscriptionAccess[]
  labels: {
    ip: string
    client: string
    format: string
    time: string
    unknownClient: string
    list: string
    /** Admin view: label for `token_tag` (shown when present). */
    tokenTag?: string
    /** Screen-reader label of the IP location (shown when the server sends one). */
    location?: string
  }
  className?: string
}) {
  return (
    <ol aria-label={labels.list} className={className ?? 'inset-group bg-fill-quaternary!'}>
      {entries.map((entry, i) => (
        <li
          key={`${entry.at}-${entry.ip}-${i}`}
          className="flex flex-wrap items-center justify-between gap-x-3 gap-y-1 px-4 py-2.5"
        >
          <div className="flex min-w-0 flex-col">
            <span className="flex min-w-0 flex-wrap items-baseline gap-x-2">
              <span className="font-mono text-subhead text-label" title={labels.ip}>
                {entry.ip}
              </span>
              {entry.location ? (
                <span
                  className="min-w-0 max-w-full truncate text-footnote text-label-secondary"
                  title={entry.location}
                >
                  {labels.location ? <span className="sr-only">{labels.location}: </span> : null}
                  {entry.location}
                </span>
              ) : null}
            </span>
            <span className="truncate text-caption text-label-secondary" title={entry.client}>
              <span className="sr-only">{labels.client}: </span>
              {clientLabel(entry.client) || labels.unknownClient}
            </span>
          </div>
          <div className="flex shrink-0 items-center gap-2">
            {entry.token_tag && labels.tokenTag ? (
              <span className="font-mono text-caption text-label-secondary" title={labels.tokenTag}>
                <span className="sr-only">{labels.tokenTag}: </span>#{entry.token_tag}
              </span>
            ) : null}
            {entry.format ? (
              <Chip size="sm" variant="soft">
                <span className="sr-only">{labels.format}: </span>
                {entry.format}
              </Chip>
            ) : null}
            <span className="text-caption text-label-secondary">
              <span className="sr-only">{labels.time}: </span>
              <DateTime value={entry.at} format="datetime" />
            </span>
          </div>
        </li>
      ))}
    </ol>
  )
}
