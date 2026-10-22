/**
 * Ticket conversation as a chat thread: my messages on the right (tint), staff on the left (fill).
 * Messages are plain text (never HTML/Markdown) with line breaks kept.
 */
import { Headset } from 'lucide-react'
import { useEffect, useRef } from 'react'
import { DateTime } from '@/components/Formatters'
import { useT } from '@/i18n'
import type { TicketMessage } from '@/lib/api/types'
import { cx } from '@/lib/cn'

export function TicketThread({ messages }: { messages: readonly TicketMessage[] }) {
  const t = useT('tickets')
  const scroller = useRef<HTMLDivElement>(null)
  const last = messages.at(-1)?.id

  // Keep the newest message in view (inside the thread only — the page does not jump).
  useEffect(() => {
    const el = scroller.current
    if (el) el.scrollTop = el.scrollHeight
  }, [last])

  return (
    <div
      ref={scroller}
      className="max-h-[min(60vh,640px)] overflow-y-auto px-4 py-5 sm:px-6"
      role="log"
      aria-label={t('detail.thread')}
      aria-live="polite"
      tabIndex={0}
    >
      <ol className="flex flex-col gap-4">
        {messages.map((message) => (
          <li
            key={message.id}
            className={cx('flex items-end gap-2', message.is_me ? 'flex-row-reverse' : 'flex-row')}
          >
            {message.is_me ? null : (
              <span
                className="mb-5 flex size-8 shrink-0 items-center justify-center rounded-full bg-system-indigo text-white"
                aria-hidden="true"
              >
                <Headset className="size-4" />
              </span>
            )}
            <div
              className={cx(
                'flex max-w-[85%] flex-col gap-1 sm:max-w-[70%]',
                message.is_me ? 'items-end' : 'items-start',
              )}
            >
              <span className="sr-only">{message.is_me ? t('detail.me') : t('detail.staff')}</span>
              <p
                className={cx(
                  'rounded-[20px] px-4 py-2.5 text-body break-words whitespace-pre-wrap',
                  message.is_me
                    ? 'rounded-ee-[6px] bg-tint text-white'
                    : 'rounded-es-[6px] bg-fill-tertiary text-label',
                )}
              >
                {message.message}
              </p>
              <span className="px-1 text-caption text-label-secondary">
                {message.is_me ? null : `${t('detail.staff')} · `}
                <DateTime value={message.created_at} />
              </span>
            </div>
          </li>
        ))}
      </ol>
    </div>
  )
}
