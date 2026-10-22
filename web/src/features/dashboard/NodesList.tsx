import { Button, Chip } from '@heroui/react'
import { Server } from 'lucide-react'
import { useState } from 'react'
import { EmptyState } from '@/components/EmptyState'
import { useT } from '@/i18n'
import type { UserNode } from '@/lib/api/types'
import { cx } from '@/lib/cn'
import { NODE_TYPE_LABEL } from './format'

const COLLAPSED = 8

export function NodesList({ nodes }: { nodes: readonly UserNode[] }) {
  const t = useT(['dashboard', 'common'])
  const [expanded, setExpanded] = useState(false)
  const sorted = [...nodes].sort((a, b) => a.sort - b.sort || a.id - b.id)
  const visible = expanded ? sorted : sorted.slice(0, COLLAPSED)
  const online = nodes.filter((n) => n.is_online).length

  return (
    <section className="surface-card flex flex-col gap-4 p-6" aria-labelledby="nodes-title">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 id="nodes-title" className="text-headline text-label">
          {t('nodes.title')}
        </h2>
        {nodes.length > 0 ? (
          <p className="text-footnote text-label-secondary">
            {t('nodes.summary', { online, total: nodes.length })}
          </p>
        ) : null}
      </div>
      {nodes.length === 0 ? (
        <EmptyState compact icon={<Server className="size-5" />} title={t('nodes.empty')} />
      ) : (
        <ul className="grid gap-x-6 sm:grid-cols-2">
          {visible.map((node) => (
            <li
              key={node.id}
              className="flex min-w-0 items-center gap-3 border-b-[0.5px] border-hairline py-3"
            >
              <span
                className={cx(
                  'size-2.5 shrink-0 rounded-full',
                  node.is_online ? 'bg-system-green' : 'bg-system-gray-3',
                )}
                aria-hidden="true"
              />
              <div className="min-w-0 flex-1">
                <p className="flex items-center gap-2">
                  <span className="truncate text-subhead font-semibold text-label">{node.name}</span>
                  {node.is_online ? (
                    <span className="sr-only">{t('nodes.online')}</span>
                  ) : (
                    <span className="shrink-0 text-caption text-label-secondary">{t('nodes.offline')}</span>
                  )}
                </p>
                <div className="mt-1 flex flex-wrap items-center gap-1.5">
                  <Chip size="sm" variant="soft" color="accent">
                    {NODE_TYPE_LABEL[node.type] ?? node.type}
                  </Chip>
                  {node.tags.map((tag) => (
                    <Chip key={tag} size="sm" variant="secondary">
                      {tag}
                    </Chip>
                  ))}
                  {node.host ? (
                    <span className="truncate font-mono text-caption text-label-secondary">
                      {node.host}
                      {node.port ? `:${node.port}` : ''}
                    </span>
                  ) : null}
                </div>
              </div>
              <span className="shrink-0 text-footnote tabular text-label-secondary" title={t('nodes.rate')}>
                {t('nodes.rateValue', { rate: formatRate(node.rate) })}
              </span>
            </li>
          ))}
        </ul>
      )}
      {sorted.length > COLLAPSED ? (
        <Button variant="ghost" size="sm" className="self-center" onPress={() => setExpanded((v) => !v)}>
          {expanded ? t('nodes.showLess') : t('nodes.showAll', { count: sorted.length })}
        </Button>
      ) : null}
    </section>
  )
}

const formatRate = (rate: number) => (Number.isInteger(rate) ? rate.toFixed(1) : String(rate))
