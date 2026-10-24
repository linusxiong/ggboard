import { Button, Chip, Dropdown, Label, Skeleton } from '@heroui/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { BookOpen, Ellipsis, Plus } from 'lucide-react'
import { useCallback, useMemo, useState } from 'react'
import { EmptyState } from '@/components/EmptyState'
import { confirm } from '@/components/feedback/confirm'
import { DateTime } from '@/components/Formatters'
import { PageShell } from '@/components/PageShell'
import { DebouncedSearch } from '@/features/admin/orders/DebouncedSearch'
import { SelectInput } from '@/features/admin/settings/inputs/Controlled'
import { useT } from '@/i18n'
import { adminApi } from '@/lib/api'
import { qk } from '@/lib/api/queries'
import type { AdminKnowledge, ID } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { KnowledgeDialog } from './KnowledgeDialog'
import { filterKnowledge, type KnowledgeFilters, LANGUAGE_SUGGESTIONS } from './knowledge'
import { moveId } from './reorder'
import { ShowSwitch } from './ShowSwitch'
import { SortButtons } from './SortButtons'
import { useContentList } from './useContentList'

const LIST_KEY = [...qk.admin.knowledge(), 'all'] as const
const ALL = '__all'

interface KnowledgePageProps {
  filters: KnowledgeFilters
  onFiltersChange: (patch: KnowledgeFilters) => void
}

export function KnowledgePage({ filters, onFiltersChange }: KnowledgePageProps) {
  const t = useT(['admin-support', 'common'])
  const qc = useQueryClient()
  const q = useQuery({ queryKey: LIST_KEY, queryFn: ({ signal }) => adminApi.knowledgeAll({}, signal) })
  const all = useMemo(() => q.data?.items ?? [], [q.data])
  const [dialog, setDialog] = useState<{ open: boolean; article: AdminKnowledge | null }>({
    open: false,
    article: null,
  })
  const { toggle, reorder } = useContentList<AdminKnowledge>({
    queryKey: LIST_KEY,
    invalidate: ['admin', 'knowledge'],
    setShow: (id, show) => adminApi.updateKnowledge(id, { show }),
    sort: (ids) => adminApi.sortKnowledge(ids),
  })

  const languages = useMemo(
    () => [...new Set([...LANGUAGE_SUGGESTIONS, ...all.map((k) => k.language)])].filter(Boolean),
    [all],
  )
  const categoriesByLanguage = useMemo(() => {
    const map = new Map<string, string[]>()
    for (const k of all) {
      const list = map.get(k.language) ?? []
      if (!list.includes(k.category)) list.push(k.category)
      map.set(k.language, list)
    }
    return map
  }, [all])
  const categories = useMemo(
    () =>
      filters.language
        ? (categoriesByLanguage.get(filters.language) ?? [])
        : [...new Set(all.map((k) => k.category))],
    [all, categoriesByLanguage, filters.language],
  )

  const visible = useMemo(() => filterKnowledge(all, filters), [all, filters])
  const visibleIds = useMemo(() => new Set<ID>(visible.map((k) => k.id)), [visible])
  const allIds = all.map((k) => k.id)
  const filtered = visible.length !== all.length

  const onSearch = useCallback((v: string | undefined) => onFiltersChange({ q: v }), [onFiltersChange])

  const remove = async (k: AdminKnowledge) => {
    const ok = await confirm({
      title: t('knowledge.deleteTitle', { title: k.title }),
      description: t('common:confirm.irreversible'),
      confirmLabel: t('common:actions.delete'),
      tone: 'danger',
    })
    if (!ok) return
    try {
      await adminApi.deleteKnowledge(k.id)
      toast.success(t('knowledge.deleted'))
      await qc.invalidateQueries({ queryKey: ['admin', 'knowledge'] })
    } catch (e) {
      toast.danger(errorMessage(e))
    }
  }

  const open = (article: AdminKnowledge | null) => setDialog({ open: true, article })

  return (
    <PageShell
      title={t('knowledge.title')}
      help={t('knowledge.help')}
      actions={
        <Button onPress={() => open(null)}>
          <Plus className="size-4" aria-hidden />
          {t('knowledge.create')}
        </Button>
      }
    >
      <section aria-label={t('knowledge.filters')} className="surface-card grid gap-4 p-4 sm:grid-cols-3">
        <SelectInput
          label={t('knowledge.fields.language')}
          name="language"
          value={filters.language ?? ALL}
          onChange={(v) =>
            onFiltersChange({ language: v === null || v === ALL ? undefined : v, category: undefined })
          }
          options={[
            { value: ALL, label: t('knowledge.allLanguages') },
            ...languages.map((l) => ({
              value: l,
              label: (LANGUAGE_SUGGESTIONS as readonly string[]).includes(l)
                ? t(`knowledge.languages.${l}`)
                : l,
            })),
          ]}
        />
        <SelectInput
          label={t('knowledge.fields.category')}
          name="category"
          value={filters.category ?? ALL}
          onChange={(v) => onFiltersChange({ category: v === null || v === ALL ? undefined : v })}
          options={[
            { value: ALL, label: t('knowledge.allCategories') },
            ...categories.map((c) => ({ value: c, label: c })),
          ]}
        />
        <DebouncedSearch
          label={t('knowledge.search')}
          value={filters.q}
          onCommit={onSearch}
          placeholder={t('knowledge.searchPlaceholder')}
          delay={200}
        />
      </section>

      {q.isPending ? (
        <div className="surface-card flex flex-col gap-3 p-4" aria-busy="true">
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className="h-14 w-full rounded-[12px]" />
          ))}
        </div>
      ) : visible.length === 0 ? (
        <div className="surface-card">
          <EmptyState
            icon={<BookOpen className="size-7" strokeWidth={1.75} />}
            title={all.length ? t('knowledge.noMatch') : t('knowledge.emptyTitle')}
            description={all.length ? undefined : t('knowledge.emptyHint')}
            action={
              all.length ? undefined : <Button onPress={() => open(null)}>{t('knowledge.create')}</Button>
            }
          />
        </div>
      ) : (
        <ul aria-label={t('knowledge.title')} className="inset-group shadow-card">
          {visible.map((k, i) => (
            <li key={k.id} className="flex flex-wrap items-center gap-x-4 gap-y-2 px-4 py-3">
              <SortButtons
                itemLabel={k.title}
                canUp={i > 0}
                canDown={i < visible.length - 1}
                isDisabled={reorder.isPending}
                upLabel={t('content.moveUp')}
                downLabel={t('content.moveDown')}
                onMove={(dir) => {
                  const next = moveId(allIds, k.id, dir, filtered ? visibleIds : undefined)
                  if (next) reorder.mutate(next)
                }}
              />
              <div className="min-w-0 flex-1 basis-48">
                <p className="truncate text-headline text-label">{k.title}</p>
                <p className="mt-0.5 flex flex-wrap items-center gap-1.5 text-footnote text-label-secondary">
                  <Chip size="sm" variant="soft" color="accent">
                    {k.category}
                  </Chip>
                  <Chip size="sm" variant="soft">
                    {k.language}
                  </Chip>
                  <span>
                    {t('knowledge.updated')} <DateTime value={k.updated_at} format="relative" />
                  </span>
                </p>
              </div>
              <div className="flex items-center gap-2">
                <ShowSwitch
                  isSelected={k.show}
                  aria-label={t('content.showFor', { title: k.title })}
                  onChange={(show) => toggle.mutate({ id: k.id, show })}
                />
                <Button
                  size="sm"
                  variant="tertiary"
                  aria-label={`${t('common:actions.edit')}: ${k.title}`}
                  onPress={() => open(k)}
                >
                  {t('common:actions.edit')}
                </Button>
                <Dropdown>
                  <Button
                    size="sm"
                    variant="ghost"
                    isIconOnly
                    aria-label={t('content.moreFor', { title: k.title })}
                  >
                    <Ellipsis className="size-4" aria-hidden />
                  </Button>
                  <Dropdown.Popover placement="bottom end">
                    <Dropdown.Menu onAction={(key) => key === 'delete' && void remove(k)}>
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

      <KnowledgeDialog
        isOpen={dialog.open}
        onOpenChange={(o) => setDialog((d) => ({ ...d, open: o }))}
        article={dialog.article}
        categories={categoriesByLanguage}
        defaultLanguage={filters.language}
      />
    </PageShell>
  )
}
