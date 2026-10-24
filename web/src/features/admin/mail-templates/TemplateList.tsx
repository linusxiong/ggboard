import type { LucideIcon } from 'lucide-react'
import { Bell, CalendarClock, Gauge, KeyRound, Mail, MessageSquareReply, Receipt } from 'lucide-react'
import { useT } from '@/i18n'
import type { AdminMailTemplateSummary } from '@/lib/api/types'
import { cx } from '@/lib/cn'
import { MAIL_LOCALES } from './search'
import { templateTitle } from './variables'

/** Sidebar symbol + background per template (raw system colour utilities: icons only). */
const TEMPLATE_ICON: Record<string, { icon: LucideIcon; tint: string }> = {
  verify_code: { icon: KeyRound, tint: 'bg-system-blue' },
  remind_expire: { icon: CalendarClock, tint: 'bg-system-orange' },
  remind_traffic: { icon: Gauge, tint: 'bg-system-teal' },
  order_paid: { icon: Receipt, tint: 'bg-system-green' },
  ticket_reply: { icon: MessageSquareReply, tint: 'bg-system-indigo' },
  notify: { icon: Bell, tint: 'bg-system-red' },
}
const FALLBACK_ICON = { icon: Mail, tint: 'bg-system-gray' }

/** Phones: the list is a horizontal chip strip — keep the selected chip in view (no page scroll). */
function revealOnPhone(li: HTMLLIElement | null) {
  const strip = li?.parentElement
  if (!li || !strip || strip.scrollWidth <= strip.clientWidth) return
  const offset = li.getBoundingClientRect().left - strip.getBoundingClientRect().left + strip.scrollLeft
  strip.scrollTo({
    left: Math.max(0, offset - (strip.clientWidth - li.offsetWidth) / 2),
    behavior: 'instant',
  })
}

interface TemplateListProps {
  items: readonly AdminMailTemplateSummary[]
  active: string
  /** The selected template has unsaved edits (orange dot). */
  dirty: boolean
  onSelect: (name: string) => void
}

/**
 * System-Settings-style list of the templates: symbol, title and the state of each locale (customised or
 * built-in); what a template is for sits behind the ⓘ of the selected one's title. A chip strip with
 * titles only on phones.
 */
export function TemplateList({ items, active, dirty, onSelect }: TemplateListProps) {
  const t = useT('admin-settings')
  return (
    <nav aria-label={t('mailTemplates.listLabel')} className="-mx-4 md:sticky md:top-[5.5rem] md:mx-0">
      <ul className="no-scrollbar flex gap-1.5 overflow-x-auto px-4 md:flex-col md:gap-0.5 md:overflow-visible md:px-0">
        {items.map((item) => {
          const { icon: Icon, tint } = TEMPLATE_ICON[item.name] ?? FALLBACK_ICON
          const selected = item.name === active
          const anyCustom = MAIL_LOCALES.some((l) => item.locales[l]?.custom)
          return (
            <li key={item.name} className="shrink-0" ref={selected ? revealOnPhone : undefined}>
              <button
                type="button"
                aria-current={selected ? 'page' : undefined}
                onClick={() => onSelect(item.name)}
                className={cx(
                  'flex w-full items-start gap-2.5 rounded-[12px] py-2 ps-2 pe-3 text-start text-label transition-colors duration-150',
                  'max-md:items-center max-md:rounded-full max-md:bg-fill-quaternary max-md:py-1 max-md:ps-1',
                  selected ? 'bg-selection max-md:bg-selection' : 'hover:bg-fill-quaternary',
                )}
              >
                <span
                  className={cx(
                    'mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-[8px] text-white max-md:mt-0 max-md:size-6 max-md:rounded-full',
                    tint,
                  )}
                  aria-hidden="true"
                >
                  <Icon className="size-4 max-md:size-3.5" />
                </span>
                <span className="flex min-w-0 flex-1 flex-col gap-0.5">
                  <span className="flex items-center gap-2">
                    <span
                      className={cx(
                        'whitespace-nowrap text-subhead md:truncate',
                        selected && 'font-semibold text-label-blue-soft',
                      )}
                    >
                      {templateTitle(t, item.name)}
                    </span>
                    {selected && dirty ? (
                      <span
                        className="size-2 shrink-0 rounded-full bg-system-orange"
                        role="img"
                        aria-label={t('mailTemplates.unsaved')}
                      />
                    ) : null}
                    {anyCustom ? (
                      <span
                        className="size-1.5 shrink-0 rounded-full bg-system-blue md:hidden"
                        role="img"
                        aria-label={t('mailTemplates.status.custom')}
                      />
                    ) : null}
                  </span>
                  <span className="flex flex-wrap gap-x-3 gap-y-0.5 text-caption text-label-secondary max-md:hidden">
                    {MAIL_LOCALES.map((l) => {
                      const custom = item.locales[l]?.custom === true
                      return (
                        <span key={l} className="inline-flex items-center gap-1">
                          <span
                            className={cx(
                              'size-1.5 shrink-0 rounded-full',
                              custom ? 'bg-system-blue' : 'bg-system-gray-3',
                            )}
                            aria-hidden="true"
                          />
                          {t(`mailTemplates.localeShort.${l}`)}{' '}
                          {custom ? t('mailTemplates.status.custom') : t('mailTemplates.status.default')}
                        </span>
                      )
                    })}
                  </span>
                </span>
              </button>
            </li>
          )
        })}
      </ul>
    </nav>
  )
}
