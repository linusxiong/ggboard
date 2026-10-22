/** A notice: optional cover image, title, date, tags and Markdown content collapsed to a preview. */
import { Button, Chip } from '@heroui/react'
import { ChevronDown } from 'lucide-react'
import { useId, useState } from 'react'
import { DateTime } from '@/components/Formatters'
import { Markdown } from '@/components/Markdown'
import { useT } from '@/i18n'
import type { Notice } from '@/lib/api/types'
import { cx } from '@/lib/cn'

/** Long enough to need "show more" (roughly > 6 rendered lines). */
function isLongNotice(content: string): boolean {
  return content.length > 280 || content.split('\n').length > 6
}

export function NoticeCard({ notice }: { notice: Notice }) {
  const t = useT('content')
  const [expanded, setExpanded] = useState(false)
  const bodyId = useId()
  const long = isLongNotice(notice.content)
  const cover = /^https?:\/\//i.test(notice.img_url) ? notice.img_url : ''

  return (
    <article className="surface-card overflow-hidden" aria-labelledby={`${bodyId}-title`}>
      {cover ? (
        <img
          src={cover}
          alt=""
          width={1200}
          height={500}
          loading="lazy"
          decoding="async"
          className="aspect-[12/5] w-full bg-fill-quaternary object-cover"
        />
      ) : null}
      <div className="flex flex-col gap-3 p-5 sm:p-6">
        <header className="flex flex-col gap-1.5">
          <h2 id={`${bodyId}-title`} className="text-title3 text-label">
            {notice.title}
          </h2>
          <div className="flex flex-wrap items-center gap-2 text-footnote text-label-secondary">
            <DateTime value={notice.created_at} format="date" />
            {notice.tags.map((tag) => (
              <Chip key={tag} size="sm" variant="soft">
                {tag}
              </Chip>
            ))}
          </div>
        </header>
        <div id={bodyId} className={cx('relative', long && !expanded && 'max-h-40 overflow-hidden')}>
          <Markdown source={notice.content} className="text-subhead" />
          {long && !expanded ? (
            <div
              className="pointer-events-none absolute inset-x-0 bottom-0 h-16 bg-gradient-to-t from-elevated to-transparent"
              aria-hidden="true"
            />
          ) : null}
        </div>
        {long ? (
          <Button
            size="sm"
            variant="ghost"
            className="self-start"
            aria-expanded={expanded}
            aria-controls={bodyId}
            onPress={() => setExpanded((v) => !v)}
          >
            {expanded ? t('notices.collapse') : t('notices.expand')}
            <ChevronDown
              className={cx(
                'size-4 transition-transform motion-reduce:transition-none',
                expanded && 'rotate-180',
              )}
              aria-hidden="true"
            />
          </Button>
        ) : null}
      </div>
    </article>
  )
}
