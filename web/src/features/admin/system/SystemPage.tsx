import { Alert, Button, Chip, Skeleton, Spinner } from '@heroui/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { ClipboardCopy, Cpu, Database, Mail, Radio, Server, Timer } from 'lucide-react'
import { type ReactNode, useMemo, useState } from 'react'
import { createDataTableColumns, DataTable } from '@/components/data-table'
import { DateTime } from '@/components/Formatters'
import { PageShell } from '@/components/PageShell'
import { useLocale, useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type { AdminLogQuery, SchedulerJobStatus } from '@/lib/api/types'
import { copyText } from '@/lib/clipboard'
import { errorMessage } from '@/lib/errors'
import { formatBytes, formatNumber, formatPercent } from '@/lib/format'
import { toast } from '@/lib/toast'
import { buildDiagnostics } from './diagnostics'
import { formatDuration, toSeconds } from './format'
import { LogsViewer } from './LogsViewer'

const STATUS_KEY = [...qk.admin.system(), 'full'] as const
const jobCol = createDataTableColumns<SchedulerJobStatus>()

function Card({ icon, title, children }: { icon: ReactNode; title: string; children: ReactNode }) {
  return (
    <section className="surface-card flex flex-col gap-3 p-5">
      <h2 className="flex items-center gap-2 text-subhead font-semibold text-label-secondary">
        <span
          className="flex size-7 items-center justify-center rounded-[8px] bg-fill-tertiary text-label"
          aria-hidden
        >
          {icon}
        </span>
        {title}
      </h2>
      <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1.5 text-subhead">{children}</dl>
    </section>
  )
}

function Item({ label, children }: { label: string; children: ReactNode }) {
  return (
    <>
      <dt className="text-label-secondary">{label}</dt>
      <dd className="text-end text-label tabular break-all">{children}</dd>
    </>
  )
}

export function SystemPage() {
  const t = useT(['admin-system', 'common'])
  const locale = useLocale()
  const qc = useQueryClient()
  const status = useQuery({
    queryKey: STATUS_KEY,
    queryFn: ({ signal }) => adminApi.systemStatusFull(signal),
    refetchInterval: 30_000,
  })
  const s = status.data
  const [copying, setCopying] = useState(false)
  const num = (n: number | undefined) => (n === undefined ? '—' : formatNumber(n, locale))

  const columns = useMemo(
    () =>
      jobCol.columns([
        jobCol.accessor('name', {
          header: t('scheduler.name'),
          meta: { isPrimary: true },
          cell: (c) => (
            <span className="flex items-center gap-2">
              <span className="font-mono text-label">{c.getValue()}</span>
              {c.row.original.running ? (
                <Chip size="sm" variant="soft" color="accent">
                  <Spinner size="sm" color="current" />
                  {t('scheduler.running')}
                </Chip>
              ) : null}
            </span>
          ),
        }),
        jobCol.accessor('spec', {
          header: t('scheduler.spec'),
          cell: (c) => <code className="font-mono text-footnote text-label">{c.getValue()}</code>,
        }),
        jobCol.accessor('last_run', {
          header: t('scheduler.lastRun'),
          cell: (c) =>
            c.getValue() ? (
              <span className="flex flex-col">
                <DateTime value={c.getValue()} format="relative" />
                {c.row.original.last_duration ? (
                  <span className="text-footnote text-label-secondary">{c.row.original.last_duration}</span>
                ) : null}
              </span>
            ) : (
              <span className="text-label-secondary">{t('scheduler.never')}</span>
            ),
        }),
        jobCol.accessor('next_run', {
          header: t('scheduler.nextRun'),
          meta: { hideOnMobile: true },
          cell: (c) => <DateTime value={c.getValue() ?? null} format="relative" />,
        }),
        jobCol.accessor((j) => j.runs ?? null, {
          id: 'runs',
          header: t('scheduler.runs'),
          meta: { align: 'end', hideOnMobile: true },
          cell: (c) =>
            c.getValue() === null ? (
              '—'
            ) : (
              <span>
                {num(c.getValue() ?? undefined)}
                {c.row.original.failures ? (
                  <span className="block text-footnote text-label-red">
                    {t('scheduler.failures', { n: num(c.row.original.failures) })}
                  </span>
                ) : null}
              </span>
            ),
        }),
        jobCol.accessor('last_error', {
          header: t('scheduler.lastError'),
          cell: (c) =>
            c.getValue() ? (
              <span className="block max-w-80 font-mono text-footnote break-words text-label-red">
                {c.getValue()}
              </span>
            ) : (
              <Chip size="sm" variant="soft" color="success">
                {t('scheduler.ok')}
              </Chip>
            ),
        }),
      ]),
    // eslint-disable-next-line react-hooks/exhaustive-deps -- `num` only depends on locale
    [t, locale],
  )

  const copyDiagnostics = async () => {
    setCopying(true)
    try {
      const q: AdminLogQuery = { level: 'warn', limit: 50 }
      const problems = await qc
        .query({ queryKey: qk.admin.logs(q), queryFn: ({ signal }) => adminApi.systemLogEntries(q, signal) })
        .catch(() => [])
      const ok = await copyText(buildDiagnostics(s, problems))
      if (ok) toast.success(t('diagnostics.copied'))
      else toast.danger(t('common:copy.failed'))
    } finally {
      setCopying(false)
    }
  }

  const mail = s?.mail ?? (s?.mail_queue !== undefined ? { pending: s.mail_queue } : undefined)
  const cache = s?.cache
  const failing = s?.scheduler.filter((j) => j.last_error).length ?? 0

  return (
    <PageShell
      title={t('title')}
      description={t('description')}
      actions={
        <Button variant="secondary" isPending={copying} onPress={() => void copyDiagnostics()}>
          <ClipboardCopy className="size-4" aria-hidden />
          {t('diagnostics.copy')}
        </Button>
      }
    >
      {status.isError ? (
        <Alert status="danger">
          <Alert.Indicator />
          <Alert.Content>
            <Alert.Title>{t('loadFailed')}</Alert.Title>
            <Alert.Description>{errorMessage(status.error)}</Alert.Description>
          </Alert.Content>
        </Alert>
      ) : null}

      {!s ? (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3" aria-busy="true">
          {[0, 1, 2, 3, 4, 5].map((i) => (
            <Skeleton key={i} className="h-40 w-full rounded-card" />
          ))}
        </div>
      ) : (
        <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          <Card icon={<Server className="size-4" />} title={t('cards.panel')}>
            <Item label={t('cards.version')}>{s.version || '—'}</Item>
            <Item label={t('cards.go')}>{s.go_version || '—'}</Item>
            <Item label={t('cards.db')}>{s.db_driver || '—'}</Item>
          </Card>
          <Card icon={<Timer className="size-4" />} title={t('cards.uptime')}>
            <Item label={t('cards.uptime')}>{formatDuration(toSeconds(s.uptime), locale)}</Item>
            <Item label={t('cards.startedAt')}>
              <DateTime value={s.started_at} />
            </Item>
            <Item label={t('cards.jobs')}>
              {failing ? (
                <span className="text-label-red">{t('cards.jobsFailing', { n: failing })}</span>
              ) : (
                t('cards.jobsOk', { n: s.scheduler.length })
              )}
            </Item>
          </Card>
          <Card icon={<Radio className="size-4" />} title={t('cards.online')}>
            <Item label={t('cards.onlineNodes')}>{num(s.online?.nodes)}</Item>
            <Item label={t('cards.onlineUsers')}>{num(s.online?.users)}</Item>
          </Card>
          <Card icon={<Mail className="size-4" />} title={t('cards.mail')}>
            <Item label={t('cards.mailDriver')}>{mail && 'driver' in mail ? (mail.driver ?? '—') : '—'}</Item>
            <Item label={t('cards.mailPending')}>{num(mail?.pending)}</Item>
            <Item label={t('cards.mailSent')}>{num(mail && 'sent' in mail ? mail.sent : undefined)}</Item>
            <Item label={t('cards.mailFailed')}>
              <span className={mail && 'failed' in mail && mail.failed ? 'text-label-red' : undefined}>
                {num(mail && 'failed' in mail ? mail.failed : undefined)}
              </span>
            </Item>
          </Card>
          <Card icon={<Database className="size-4" />} title={t('cards.cache')}>
            <Item label={t('cards.cacheItems')}>
              {num(typeof cache?.items === 'number' ? cache.items : undefined)}
              {typeof cache?.max_items === 'number' && cache.max_items > 0
                ? ` / ${num(cache.max_items)}`
                : ''}
            </Item>
            <Item label={t('cards.cacheHitRatio')}>
              {typeof cache?.hit_ratio === 'number' ? formatPercent(cache.hit_ratio, locale, 1) : '—'}
            </Item>
            <Item label={t('cards.cacheHits')}>
              {num(typeof cache?.hits === 'number' ? cache.hits : undefined)} /{' '}
              {num(typeof cache?.misses === 'number' ? cache.misses : undefined)}
            </Item>
          </Card>
          <Card icon={<Cpu className="size-4" />} title={t('cards.runtime')}>
            <Item label={t('cards.goroutines')}>{num(s.goroutines)}</Item>
            <Item label={t('cards.memAlloc')}>{s.memory ? formatBytes(s.memory.alloc, locale) : '—'}</Item>
            <Item label={t('cards.memSys')}>{s.memory ? formatBytes(s.memory.sys, locale) : '—'}</Item>
          </Card>
        </div>
      )}

      <section aria-labelledby="scheduler-title" className="flex flex-col gap-3">
        <h2 id="scheduler-title" className="text-title3 text-label">
          {t('scheduler.title')}
        </h2>
        <DataTable
          aria-label={t('scheduler.title')}
          columns={columns}
          data={s?.scheduler}
          isLoading={status.isPending}
          isFetching={status.isFetching && !status.isPending}
          getRowId={(j) => j.name}
        />
      </section>

      <LogsViewer />
    </PageShell>
  )
}
