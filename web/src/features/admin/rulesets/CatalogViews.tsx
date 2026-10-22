import { Chip } from '@heroui/react'
import { ExternalLink } from 'lucide-react'
import { useMemo } from 'react'
import { createDataTableColumns, DataTable } from '@/components/data-table'
import { useT } from '@/i18n'
import { localized } from './catalog'
import type {
  RulesetCategoryFull,
  RulesetMirror,
  RulesetRuleRefFull,
  RulesetSourceFull,
  RulesetTemplateFull,
} from '@/lib/api/types'

function CustomBadge({ show }: { show: boolean }) {
  const t = useT('admin-settings')
  return show ? (
    <Chip size="sm" variant="soft" color="warning">
      {t('rulesets.custom')}
    </Chip>
  ) : null
}

// ---------------------------------------------------------------- sources

interface SourcesViewProps {
  sources: Record<string, RulesetSourceFull>
  customIds: ReadonlySet<string>
  locale: string
}

export function SourcesView({ sources, customIds, locale }: SourcesViewProps) {
  const t = useT('admin-settings')
  const list = Object.entries(sources).sort(([, a], [, b]) => (a.order ?? 0) - (b.order ?? 0))
  return (
    <ul className="flex flex-col gap-4" aria-label={t('rulesets.tabs.sources')}>
      {list.map(([id, s]) => (
        <li key={id} className="surface-card p-5">
          <div className="flex flex-wrap items-center gap-2">
            <h3 className="text-headline text-label">{s.name}</h3>
            <Chip size="sm" variant="soft" className="font-mono">
              {id}
            </Chip>
            {s.order !== undefined ? (
              <span className="text-footnote text-label-secondary">
                {t('rulesets.order', { n: s.order })}
              </span>
            ) : null}
            <CustomBadge show={customIds.has(id)} />
            {s.homepage ? (
              <a
                href={s.homepage}
                target="_blank"
                rel="noopener noreferrer"
                className="ms-auto inline-flex items-center gap-1 text-subhead text-label-blue hover:underline"
              >
                {t('rulesets.homepage')}
                <ExternalLink className="size-3.5" aria-hidden />
                <span className="sr-only">({s.name})</span>
              </a>
            ) : null}
          </div>
          {s.description ? (
            <p className="mt-1 text-subhead text-label-secondary">{localized(s.description, locale, '')}</p>
          ) : null}
          {s.license ? <p className="mt-1 text-footnote text-label-secondary">{s.license}</p> : null}
          <div className="mt-3 overflow-x-auto">
            <table className="w-full text-footnote">
              <caption className="sr-only">{t('rulesets.urlTemplatesOf', { name: s.name })}</caption>
              <thead>
                <tr className="text-start text-label-secondary">
                  <th scope="col" className="py-1 pe-4 text-start font-medium">
                    {t('rulesets.formatKey')}
                  </th>
                  <th scope="col" className="py-1 pe-4 text-start font-medium">
                    {t('rulesets.behavior')}
                  </th>
                  <th scope="col" className="py-1 text-start font-medium">
                    {t('rulesets.urlTemplate')}
                  </th>
                </tr>
              </thead>
              <tbody>
                {Object.entries(s.urls ?? {}).map(([key, url]) => {
                  const meta = s.url_meta?.[key]
                  return (
                    <tr key={key} className="hairline-t align-top">
                      <th
                        scope="row"
                        className="py-1.5 pe-4 text-start font-mono font-normal whitespace-nowrap text-label"
                      >
                        {key}
                      </th>
                      <td className="py-1.5 pe-4 whitespace-nowrap text-label-secondary">
                        {meta ? `${meta.behavior ?? '—'} · ${meta.format ?? '—'} · {${meta.field}}` : '—'}
                      </td>
                      <td className="py-1.5 font-mono break-all text-label">{url}</td>
                    </tr>
                  )
                })}
              </tbody>
            </table>
          </div>
        </li>
      ))}
    </ul>
  )
}

// ---------------------------------------------------------------- categories

const catCol = createDataTableColumns<RulesetCategoryFull>()

const refText = (ref: RulesetRuleRefFull) =>
  [ref.name, ref.geosite && `geosite:${ref.geosite}`, ref.geoip && `geoip:${ref.geoip}`]
    .filter(Boolean)
    .join(' + ')

interface CategoriesViewProps {
  categories: readonly RulesetCategoryFull[]
  customIds: ReadonlySet<string>
  locale: string
}

export function CategoriesView({ categories, customIds, locale }: CategoriesViewProps) {
  const t = useT('admin-settings')
  const sorted = useMemo(() => [...categories].sort((a, b) => a.order - b.order), [categories])
  const columns = useMemo(
    () =>
      catCol.columns([
        catCol.accessor('id', {
          header: t('rulesets.category'),
          meta: { isPrimary: true },
          cell: (c) => (
            <span className="flex flex-col">
              <span className="flex items-center gap-1.5 text-label">
                {localized(c.row.original.name, locale, c.getValue())}
                <CustomBadge show={customIds.has(c.getValue())} />
              </span>
              <span className="font-mono text-footnote text-label-secondary">{c.getValue()}</span>
            </span>
          ),
        }),
        catCol.accessor('default_target', {
          header: t('rulesets.defaultTarget'),
          cell: (c) => (
            <Chip
              size="sm"
              variant="soft"
              color={c.getValue() === 'REJECT' ? 'danger' : c.getValue() === 'DIRECT' ? 'success' : 'accent'}
            >
              {c.getValue()}
            </Chip>
          ),
        }),
        catCol.accessor('order', {
          header: t('rulesets.orderCol'),
          meta: { align: 'end', hideOnMobile: true, className: 'whitespace-nowrap' },
        }),
        catCol.accessor('rules', {
          header: t('rulesets.sourcesCol'),
          cell: (c) => (
            <ul className="flex flex-col gap-0.5 text-footnote">
              {Object.entries(c.getValue()).map(([src, ref]) => (
                <li key={src} className="flex flex-wrap gap-x-1.5">
                  <span className="font-mono text-label-secondary">{src}</span>
                  <span className="font-mono text-label break-all">
                    {(Array.isArray(ref) ? ref : [ref]).map(refText).join(', ')}
                  </span>
                </li>
              ))}
            </ul>
          ),
        }),
      ]),
    [t, locale, customIds],
  )
  return (
    <DataTable
      aria-label={t('rulesets.tabs.categories')}
      columns={columns}
      data={sorted}
      getRowId={(c) => c.id}
    />
  )
}

// ---------------------------------------------------------------- templates

interface TemplatesViewProps {
  templates: readonly RulesetTemplateFull[]
  customIds: ReadonlySet<string>
  locale: string
}

export function TemplatesView({ templates, customIds, locale }: TemplatesViewProps) {
  const t = useT('admin-settings')
  return (
    <ul className="grid gap-4 lg:grid-cols-2" aria-label={t('rulesets.tabs.templates')}>
      {templates.map((tpl) => {
        const cats = Object.entries(tpl.categories ?? {})
        return (
          <li key={tpl.id} className="surface-card flex flex-col gap-3 p-5">
            <div className="flex flex-wrap items-center gap-2">
              <h3 className="text-headline text-label">{localized(tpl.name, locale, tpl.id)}</h3>
              <Chip size="sm" variant="soft" className="font-mono">
                {tpl.id}
              </Chip>
              <CustomBadge show={customIds.has(tpl.id)} />
            </div>
            {tpl.description ? (
              <p className="text-subhead text-label-secondary">{localized(tpl.description, locale, '')}</p>
            ) : null}
            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-footnote">
              <dt className="text-label-secondary">{t('rulesets.final')}</dt>
              <dd className="font-mono text-label">{tpl.final}</dd>
              <dt className="text-label-secondary">{t('rulesets.geoipCn')}</dt>
              <dd className="text-label">{tpl.geoip_cn_direct ? t('rulesets.yes') : t('rulesets.no')}</dd>
              <dt className="text-label-secondary">{t('rulesets.categoriesCount')}</dt>
              <dd className="text-label">{cats.length}</dd>
            </dl>
            {cats.length ? (
              <div className="flex flex-wrap gap-1">
                {cats.map(([id, target]) => (
                  <Chip key={id} size="sm" variant="soft">
                    <span className="font-mono">
                      {id} → {target}
                    </span>
                  </Chip>
                ))}
              </div>
            ) : null}
            <div>
              <h4 className="mb-1 text-footnote font-semibold text-label-secondary">
                {t('rulesets.groups')}
              </h4>
              <ul className="flex flex-col gap-0.5 text-footnote">
                {tpl.groups.map((g) => (
                  <li key={g.name} className="flex flex-wrap gap-x-2">
                    <span className="font-medium text-label">{g.name}</span>
                    <span className="text-label-secondary">{g.type}</span>
                    <span className="font-mono break-all text-label-secondary">{g.include}</span>
                  </li>
                ))}
              </ul>
            </div>
          </li>
        )
      })}
    </ul>
  )
}

// ---------------------------------------------------------------- mirrors

interface MirrorsViewProps {
  mirrors: readonly RulesetMirror[]
  customIds: ReadonlySet<string>
  locale: string
}

export function MirrorsView({ mirrors, customIds, locale }: MirrorsViewProps) {
  const t = useT('admin-settings')
  return (
    <ul className="inset-group" aria-label={t('rulesets.tabs.mirrors')}>
      {mirrors.map((m) => (
        <li key={m.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 px-4 py-3">
          <span className="text-subhead text-label">{localized(m.name, locale, m.id)}</span>
          <Chip size="sm" variant="soft" className="font-mono">
            {m.id}
          </Chip>
          <CustomBadge show={customIds.has(m.id)} />
          <span className="ms-auto font-mono text-footnote break-all text-label-secondary">
            {m.prefix || t('rulesets.directPrefix')}
          </span>
        </li>
      ))}
    </ul>
  )
}
