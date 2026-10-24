import { Button, Chip, Dropdown, Label, Skeleton } from '@heroui/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { Ellipsis, Megaphone, Plus } from 'lucide-react'
import { useState } from 'react'
import { EmptyState } from '@/components/EmptyState'
import { confirm } from '@/components/feedback/confirm'
import { DateTime } from '@/components/Formatters'
import { PageShell } from '@/components/PageShell'
import { useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type { AdminNotice } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { NoticeDialog } from './NoticeDialog'
import { moveId } from './reorder'
import { ShowSwitch } from './ShowSwitch'
import { SortButtons } from './SortButtons'
import { useContentList } from './useContentList'

const LIST_KEY = [...qk.admin.notices(), 'all'] as const

export function NoticesPage() {
  const t = useT(['admin-support', 'common'])
  const qc = useQueryClient()
  const q = useQuery({ queryKey: LIST_KEY, queryFn: ({ signal }) => adminApi.noticeList(signal) })
  const items = q.data?.items ?? []
  const [dialog, setDialog] = useState<{ open: boolean; notice: AdminNotice | null }>({
    open: false,
    notice: null,
  })
  const { toggle, reorder } = useContentList<AdminNotice>({
    queryKey: LIST_KEY,
    invalidate: qk.admin.notices(),
    setShow: (id, show) => adminApi.updateNotice(id, { show }),
    sort: (ids) => adminApi.sortNotices(ids),
  })

  const remove = async (n: AdminNotice) => {
    const ok = await confirm({
      title: t('notices.deleteTitle', { title: n.title }),
      description: t('common:confirm.irreversible'),
      confirmLabel: t('common:actions.delete'),
      tone: 'danger',
    })
    if (!ok) return
    try {
      await adminApi.deleteNotice(n.id)
      toast.success(t('notices.deleted'))
      await qc.invalidateQueries({ queryKey: qk.admin.notices() })
    } catch (e) {
      toast.danger(errorMessage(e))
    }
  }

  const ids = items.map((n) => n.id)
  const open = (notice: AdminNotice | null) => setDialog({ open: true, notice })

  return (
    <PageShell
      title={t('notices.title')}
      help={t('notices.help')}
      actions={
        <Button onPress={() => open(null)}>
          <Plus className="size-4" aria-hidden />
          {t('notices.create')}
        </Button>
      }
    >
      {q.isPending ? (
        <div className="surface-card flex flex-col gap-3 p-4" aria-busy="true">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-16 w-full rounded-[12px]" />
          ))}
        </div>
      ) : items.length === 0 ? (
        <div className="surface-card">
          <EmptyState
            icon={<Megaphone className="size-7" strokeWidth={1.75} />}
            title={t('notices.emptyTitle')}
            description={t('notices.emptyHint')}
            action={<Button onPress={() => open(null)}>{t('notices.create')}</Button>}
          />
        </div>
      ) : (
        <ul aria-label={t('notices.title')} className="inset-group shadow-card">
          {items.map((n, i) => (
            <li key={n.id} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
              <SortButtons
                itemLabel={n.title}
                canUp={i > 0}
                canDown={i < items.length - 1}
                isDisabled={reorder.isPending}
                upLabel={t('content.moveUp')}
                downLabel={t('content.moveDown')}
                onMove={(dir) => {
                  const next = moveId(ids, n.id, dir)
                  if (next) reorder.mutate(next)
                }}
              />
              {n.img_url ? (
                <img
                  src={n.img_url}
                  alt=""
                  width={72}
                  height={48}
                  loading="lazy"
                  className="h-12 w-18 shrink-0 rounded-[8px] bg-fill-tertiary object-cover"
                />
              ) : null}
              <div className="min-w-0 flex-1 basis-48">
                <p className="truncate text-headline text-label">{n.title}</p>
                <p className="mt-0.5 flex flex-wrap items-center gap-1.5 text-footnote text-label-secondary">
                  <DateTime value={n.created_at} format="date" />
                  {n.tags?.map((tag) => (
                    <Chip key={tag} size="sm" variant="soft">
                      {tag}
                    </Chip>
                  ))}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <ShowSwitch
                  isSelected={n.show}
                  aria-label={t('content.showFor', { title: n.title })}
                  onChange={(show) => toggle.mutate({ id: n.id, show })}
                />
                <Button
                  size="sm"
                  variant="tertiary"
                  aria-label={`${t('common:actions.edit')}: ${n.title}`}
                  onPress={() => open(n)}
                >
                  {t('common:actions.edit')}
                </Button>
                <Dropdown>
                  <Button
                    size="sm"
                    variant="ghost"
                    isIconOnly
                    aria-label={t('content.moreFor', { title: n.title })}
                  >
                    <Ellipsis className="size-4" aria-hidden />
                  </Button>
                  <Dropdown.Popover placement="bottom end">
                    <Dropdown.Menu onAction={(key) => key === 'delete' && void remove(n)}>
                      <Dropdown.Item id="delete" textValue={t('common:actions.delete')} variant="danger">
                        <Label>{t('common:actions.delete')}</Label>
                      </Dropdown.Item>
                    </Dropdown.Menu>
                  </Dropdown.Popover>
                </Dropdown>
              </div>
            </li>
          ))}
        </ul>
      )}
      <NoticeDialog
        isOpen={dialog.open}
        onOpenChange={(o) => setDialog((d) => ({ ...d, open: o }))}
        notice={dialog.notice}
      />
    </PageShell>
  )
}
