import { Button } from '@heroui/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { RefreshCw } from 'lucide-react'
import { type ReactNode, useEffect, useRef } from 'react'
import { DateTime } from '@/components/Formatters'
import { useLocale, useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type { GeoIPStatus } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { formatBytes, formatNumber } from '@/lib/format'
import { toast } from '@/lib/toast'

/** Under the settings prefix, so saving settings (e.g. `geoip_enable`) refetches it. */
const GEOIP_STATUS_KEY = [...qk.admin.settings(), 'geoip'] as const

/**
 * The IP location database (`GET /admin/geoip`) under the 「IP 归属地」 settings section, with
 * 「立即更新」 (`POST /admin/geoip/update`). A download that outlasts the request (202, `updating`)
 * is polled every 2 s and reported when it ends. Hidden while the feature is off.
 */
export function GeoIPDatabase({ dirty }: { dirty: boolean }) {
  const t = useT('admin-settings')
  const locale = useLocale()
  const qc = useQueryClient()
  const status = useQuery({
    queryKey: GEOIP_STATUS_KEY,
    queryFn: ({ signal }) => adminApi.geoipStatus(signal),
    refetchInterval: (q) => (q.state.data?.updating ? 2000 : false),
  })
  /** A background update started from here: report its end once. */
  const waiting = useRef(false)

  const update = useMutation({
    mutationFn: () => adminApi.updateGeoIP(),
    onSuccess: (data) => {
      qc.setQueryData<GeoIPStatus>(GEOIP_STATUS_KEY, data)
      if (data.updating) {
        waiting.current = true
        toast.info(t('geoip.updating'))
      } else {
        toast.success(t(data.updated ? 'geoip.updated' : 'geoip.current', { version: data.version }))
      }
    },
    onError: (e) => {
      toast.error(errorMessage(e))
      void qc.invalidateQueries({ queryKey: GEOIP_STATUS_KEY })
    },
  })

  const data = status.data
  useEffect(() => {
    if (!data || data.updating || !waiting.current) return
    waiting.current = false
    if (data.last_error) toast.error(t('geoip.lastError', { error: data.last_error }))
    else toast.success(t('geoip.current', { version: data.version }))
  }, [data, t])

  if (status.isError)
    return (
      <p role="alert" className="px-1 text-footnote text-label-red">
        {t('geoip.loadFailed', { error: errorMessage(status.error) })}
      </p>
    )
  if (!data) return <div className="h-28 shimmer rounded-card bg-fill-quaternary" aria-busy="true" />
  if (!data.enabled) return null

  const busy = update.isPending || data.updating
  const rows: Array<{ label: string; value: ReactNode }> = data.version
    ? [
        { label: t('geoip.version'), value: <span className="font-mono">{data.version}</span> },
        { label: t('geoip.records'), value: formatNumber(data.records, locale) },
        { label: t('geoip.size'), value: formatBytes(data.size, locale) },
        { label: t('geoip.updatedAt'), value: <DateTime value={data.updated_at} /> },
        { label: t('geoip.checkedAt'), value: <DateTime value={data.checked_at} /> },
      ]
    : [{ label: t('geoip.version'), value: t('geoip.none') }]

  return (
    <div
      role="group"
      aria-label={t('geoip.status')}
      className="surface-card flex flex-col gap-3 px-5 py-4 sm:px-6"
    >
      <dl className="grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1.5 text-subhead">
        {rows.map((row) => (
          <div key={row.label} className="contents">
            <dt className="text-label-secondary">{row.label}</dt>
            <dd className="min-w-0 truncate text-label">{row.value}</dd>
          </div>
        ))}
      </dl>
      {data.last_error ? (
        <p className="text-footnote break-words text-label-red">
          {t('geoip.lastError', { error: data.last_error })}
        </p>
      ) : null}
      <div className="flex flex-wrap items-center gap-x-3 gap-y-2">
        <Button
          variant="secondary"
          isDisabled={dirty || busy}
          isPending={busy}
          onPress={() => update.mutate()}
        >
          <RefreshCw className="size-4" aria-hidden />
          {t('geoip.update')}
        </Button>
        {dirty ? (
          <p className="text-footnote text-label-secondary">{t('geoip.unsaved')}</p>
        ) : data.updating ? (
          <p role="status" className="text-footnote text-label-secondary">
            {t('geoip.updating')}
          </p>
        ) : null}
      </div>
    </div>
  )
}
