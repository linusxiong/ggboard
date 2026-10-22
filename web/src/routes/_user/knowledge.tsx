import { SearchField } from '@heroui/react'
import { keepPreviousData, useQuery } from '@tanstack/react-query'
import { createFileRoute, Link, useNavigate } from '@tanstack/react-router'
import { BookOpen, ChevronLeft, FileText } from 'lucide-react'
import { useState } from 'react'
import { EmptyState } from '@/components/EmptyState'
import { PageShell } from '@/components/PageShell'
import { SegmentedControl } from '@/components/SegmentedControl'
import { KnowledgeArticleView } from '@/features/content/KnowledgeArticleView'
import { useDebouncedEffect } from '@/features/content/useDebounced'
import { loadNamespaces, useLocale, useT } from '@/i18n'
import { guestConfigQuery, knowledgeIndexQuery } from '@/lib/api/queries'
import type { Locale } from '@/lib/api/types'
import { buttonClass } from '@/lib/button'
import { cx } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'

interface KnowledgeSearch {
  language?: string
  keyword?: string
  id?: number
}

export const Route = createFileRoute('/_user/knowledge')({
  validateSearch: (search: Record<string, unknown>): KnowledgeSearch => {
    const id = Number(search.id)
    const language = typeof search.language === 'string' ? search.language.slice(0, 16) : ''
    const keyword = typeof search.keyword === 'string' ? search.keyword.slice(0, 100) : ''
    return {
      ...(language ? { language } : {}),
      ...(keyword ? { keyword } : {}),
      ...(Number.isInteger(id) && id > 0 ? { id } : {}),
    }
  },
  loader: () => loadNamespaces(['content', 'common']),
  component: KnowledgePage,
})

const FALLBACK_LOCALES: readonly Locale[] = ['zh-CN', 'en']

function KnowledgePage() {
  const t = useT(['content', 'common', 'core'])
  const uiLocale = useLocale()
  const search = Route.useSearch()
  const navigate = useNavigate({ from: Route.fullPath })
  const language = search.language ?? uiLocale
  const config = useQuery(guestConfigQuery())
  const locales = config.data?.locales.length ? config.data.locales : FALLBACK_LOCALES

  const [text, setText] = useState(search.keyword ?? '')
  useDebouncedEffect(text.trim(), 300, (keyword) => {
    void navigate({
      search: (prev) => ({ ...prev, keyword: keyword || undefined }),
      replace: true,
    })
  })

  const index = useQuery({
    ...knowledgeIndexQuery({ language, ...(search.keyword ? { keyword: search.keyword } : {}) }),
    placeholderData: keepPreviousData,
  })
  const categories = Object.entries(index.data ?? {}).filter(([, items]) => items.length > 0)
  const selected = search.id

  return (
    <PageShell
      title={t('knowledge.title')}
      description={t('knowledge.subtitle')}
      actions={
        locales.length > 1 ? (
          <SegmentedControl
            aria-label={t('knowledge.language')}
            size="sm"
            value={language}
            onChange={(next) =>
              void navigate({ search: (prev) => ({ ...prev, language: next, id: undefined }) })
            }
            items={locales.map((locale) => ({ id: locale, label: t(`core:locale.${locale}`) }))}
          />
        ) : null
      }
    >
      <div className="grid items-start gap-5 lg:grid-cols-[minmax(0,320px)_minmax(0,1fr)]">
        <nav
          aria-label={t('knowledge.index')}
          className={cx('flex flex-col gap-4', selected !== undefined && 'hidden lg:flex')}
        >
          <SearchField
            aria-label={t('knowledge.search')}
            value={text}
            onChange={setText}
            onClear={() => setText('')}
            fullWidth
          >
            <SearchField.Group>
              <SearchField.SearchIcon />
              <SearchField.Input placeholder={t('knowledge.searchPlaceholder')} />
              <SearchField.ClearButton />
            </SearchField.Group>
          </SearchField>

          {index.isPending ? (
            <div className="surface-card flex flex-col gap-3 p-4" aria-busy="true">
              {Array.from({ length: 5 }, (_, i) => (
                <div key={i} className="h-5 shimmer rounded bg-fill-tertiary" />
              ))}
            </div>
          ) : index.isError ? (
            <div className="surface-card">
              <EmptyState compact title={t('knowledge.loadError')} description={errorMessage(index.error)} />
            </div>
          ) : categories.length === 0 ? (
            <div className="surface-card">
              <EmptyState
                compact
                icon={<BookOpen className="size-5" strokeWidth={1.75} />}
                title={
                  search.keyword
                    ? t('knowledge.noResults', { keyword: search.keyword })
                    : t('knowledge.empty')
                }
              />
            </div>
          ) : (
            <div
              className={cx(
                'flex flex-col gap-4 transition-opacity',
                index.isPlaceholderData && 'opacity-60',
              )}
            >
              {categories.map(([category, items]) => (
                <section key={category} aria-label={category} className="flex flex-col gap-1.5">
                  <h2 className="px-1 text-footnote font-semibold tracking-wide text-label-secondary uppercase">
                    {category}
                  </h2>
                  <ul className="inset-group shadow-card">
                    {items.map((item) => {
                      const active = item.id === selected
                      return (
                        <li key={item.id}>
                          <Link
                            to="/knowledge"
                            search={(prev) => ({ ...prev, id: item.id })}
                            aria-current={active ? 'page' : undefined}
                            className={cx(
                              'flex items-center gap-3 px-4 py-3 text-subhead text-label transition-colors',
                              active ? 'bg-fill-secondary font-semibold' : 'hover:bg-fill-quaternary',
                            )}
                          >
                            <FileText className="size-4 shrink-0 text-label-secondary" aria-hidden="true" />
                            <span className="min-w-0 flex-1 truncate">{item.title}</span>
                          </Link>
                        </li>
                      )
                    })}
                  </ul>
                </section>
              ))}
            </div>
          )}
        </nav>

        <div className={cx('surface-card min-w-0 p-5 sm:p-7', selected === undefined && 'hidden lg:block')}>
          {selected !== undefined ? (
            <div className="flex flex-col gap-4">
              <Link
                to="/knowledge"
                search={(prev) => ({ ...prev, id: undefined })}
                className={buttonClass({ variant: 'ghost', size: 'sm', className: 'self-start lg:hidden' })}
              >
                <ChevronLeft className="size-4" aria-hidden="true" />
                {t('knowledge.back')}
              </Link>
              <KnowledgeArticleView key={selected} id={selected} />
            </div>
          ) : (
            <EmptyState
              icon={<BookOpen className="size-7" strokeWidth={1.75} />}
              title={t('knowledge.pick')}
              description={t('knowledge.pickHint')}
            />
          )}
        </div>
      </div>
    </PageShell>
  )
}
