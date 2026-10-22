/**
 * `/subscription`: profile switcher (default + user profiles, `?profile=<slug>`), the active profile's link /
 * QR / import buttons and its editors (routing rules, node operations, preview), then the account-wide
 * security (access log, rotate / reset) and age-encryption cards.
 *
 * Servers without `GET /user/rule-profiles` (404) get the single-profile page: the default profile through
 * `/user/rule-profile`, no switcher, and no node operations unless the stored profile carries `nodes`.
 */
import { Tabs } from '@heroui/react'
import { useQuery, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { FileCode2 } from 'lucide-react'
import { lazy, type ReactNode, Suspense, useCallback, useRef, useState } from 'react'
import { PageShell } from '@/components/PageShell'
import { confirm } from '@/components/feedback/confirm'
import { QueryStatus } from '@/features/dashboard/QueryStatus'
import { useT } from '@/i18n'
import { guestConfigQuery, qk, subscriptionQuery, userCatalogQuery, userNodesQuery } from '@/lib/api/queries'
import type { NodeOperations, RuleProfile, RuleProfileSummary } from '@/lib/api/types'
import { saveRuleProfile, saveRuleProfileBySlug } from '@/lib/api/user'
import { AccessCard } from './AccessCard'
import { AgeCard } from './AgeCard'
import { withSubscribeQuery } from './importLinks'
import { NodeOpsEditor } from './NodeOpsEditor'
import { ProfileTabs } from './ProfileTabs'
import { orderProfiles, pickActive, profileUrl, tabKey } from './profiles'
import { profileDetailKey, profileDetailQuery, profilesKey, ruleProfilesQuery } from './queries'
import { RuleBuilder } from './RuleBuilder'
import { normalizeProfile } from './ruleProfile'
import { SAVED_PROFILE, SubscribeCard } from './SubscribeCard'

const PreviewPanel = lazy(() => import('./PreviewPanel'))
const routeApi = getRouteApi('/_user/subscription')

type EditorTab = 'rules' | 'nodes' | 'preview'
const EDITOR_TABS: readonly EditorTab[] = ['rules', 'nodes', 'preview']

export function SubscriptionPage() {
  const t = useT(['subscription', 'common'])
  const { profile: requested } = routeApi.useSearch()
  const navigate = routeApi.useNavigate()
  const sub = useQuery(subscriptionQuery())
  const catalog = useQuery(userCatalogQuery())
  const config = useQuery(guestConfigQuery())
  const list = useQuery(ruleProfilesQuery())
  const [override, setOverride] = useState(SAVED_PROFILE)
  const dirty = useRef({ rules: false, nodes: false })

  const allowCustom = config.data?.allow_user_rule_profile !== false
  const supportsProfiles = list.isSuccess
  const profiles = orderProfiles(list.data)
  const active = pickActive(profiles, requested)
  const appName = config.data?.app_name ?? 'GG Bond'
  const profileName = active.name || t('profiles.default')

  const onDirty = useCallback((part: 'rules' | 'nodes', value: boolean) => {
    dirty.current = { ...dirty.current, [part]: value }
  }, [])

  const select = async (next: RuleProfileSummary) => {
    if (dirty.current.rules || dirty.current.nodes) {
      const ok = await confirm({
        title: t('profiles.unsavedTitle'),
        description: t('profiles.unsavedHint'),
        confirmLabel: t('profiles.unsavedConfirm'),
        tone: 'danger',
      })
      if (!ok) return
    }
    dirty.current = { rules: false, nodes: false }
    setOverride(SAVED_PROFILE)
    void navigate({
      search: next.is_default || !next.slug ? {} : { profile: next.slug },
      replace: true,
      resetScroll: false,
    })
  }

  const content = (
    <>
      <QueryStatus query={sub} minHeight={360}>
        {(data) => {
          const base = profileUrl(data, active)
          return (
            <SubscribeCard
              url={active.is_default ? withSubscribeQuery(base, override ? { profile: override } : {}) : base}
              appName={active.is_default ? appName : `${appName} · ${profileName}`}
              catalog={catalog.data}
              override={override}
              onOverride={setOverride}
              allowOverride={allowCustom && active.is_default}
            />
          )
        }}
      </QueryStatus>
      <ProfileEditors
        key={tabKey(active)}
        active={active}
        profileName={profileName}
        supportsProfiles={supportsProfiles}
        allowCustom={allowCustom}
        override={active.is_default ? override : ''}
        onDirty={onDirty}
      />
    </>
  )

  return (
    <PageShell title={t('title')} description={t('subtitle')}>
      {list.isPending ? (
        <div className="h-10 animate-pulse rounded-[12px] bg-fill-quaternary" aria-busy="true" />
      ) : null}
      {supportsProfiles ? (
        <ProfileTabs
          profiles={profiles}
          active={active}
          reserved={catalog.data?.templates.map((tpl) => tpl.id) ?? []}
          canManage={allowCustom}
          onSelect={(p) => void select(p)}
        >
          {content}
        </ProfileTabs>
      ) : (
        content
      )}

      <div className="grid grid-cols-1 items-start gap-6 lg:grid-cols-2">
        <AccessCard />
        <AgeCard
          subscribeUrl={sub.data?.subscribe_url}
          formats={sub.data?.formats}
          optional={config.data?.subscribe_age_optional}
          strict={config.data?.subscribe_age_strict}
        />
      </div>
    </PageShell>
  )
}

function ProfileEditors({
  active,
  profileName,
  supportsProfiles,
  allowCustom,
  override,
  onDirty,
}: {
  active: RuleProfileSummary
  profileName: string
  supportsProfiles: boolean
  allowCustom: boolean
  /** Template override of the default profile's link (preview follows it). */
  override: string
  onDirty: (part: 'rules' | 'nodes', dirty: boolean) => void
}) {
  const t = useT(['subscription', 'common'])
  const queryClient = useQueryClient()
  const catalog = useQuery(userCatalogQuery())
  const detail = useQuery(profileDetailQuery(active))
  const [tab, setTab] = useState<EditorTab>('rules')
  const [visited, setVisited] = useState<ReadonlySet<EditorTab>>(() => new Set(['rules']))
  const key = profileDetailKey(active)

  const put = useCallback(
    (profile: RuleProfile) =>
      active.is_default || !active.slug
        ? saveRuleProfile(profile)
        : saveRuleProfileBySlug(active.slug, profile),
    [active],
  )
  const stored = () => queryClient.getQueryData<RuleProfile>(key)
  const onSaved = (result: RuleProfile) => {
    queryClient.setQueryData(key, result)
    void queryClient.invalidateQueries({ queryKey: profilesKey() })
    void queryClient.invalidateQueries({ queryKey: [...qk.user.subscription(), 'preview'] })
  }
  const saveRules = (rules: RuleProfile) => {
    const nodes = stored()?.nodes
    return put({ ...rules, ...(nodes ? { nodes } : {}) })
  }
  const saveNodes = async (nodes: NodeOperations) => {
    const current = stored() ?? detail.data
    if (!current) throw new Error(t('common:states.loading'))
    const result = await put({ ...normalizeProfile(current), nodes })
    onSaved(result)
    return result
  }
  const onRulesDirty = useCallback((dirty: boolean) => onDirty('rules', dirty), [onDirty])
  const onNodesDirty = useCallback((dirty: boolean) => onDirty('nodes', dirty), [onDirty])

  return (
    <Tabs
      variant="secondary"
      selectedKey={tab}
      onSelectionChange={(k) => {
        const next = EDITOR_TABS.find((x) => x === String(k)) ?? 'rules'
        setTab(next)
        setVisited((v) => (v.has(next) ? v : new Set([...v, next])))
      }}
      className="flex flex-col gap-4"
    >
      <Tabs.ListContainer className="max-w-full">
        <Tabs.List aria-label={t('editor.tabs', { name: profileName })}>
          {EDITOR_TABS.map((id) => (
            <Tabs.Tab key={id} id={id}>
              {t(`editor.${id}`)}
              <Tabs.Indicator />
            </Tabs.Tab>
          ))}
        </Tabs.List>
      </Tabs.ListContainer>

      <Tabs.Panel id="rules" shouldForceMount className="p-0 data-[inert=true]:hidden">
        <QueryStatus query={catalog} minHeight={640}>
          {(cat) => (
            <QueryStatus query={detail} minHeight={640}>
              {(saved) => (
                <RuleBuilder
                  catalog={cat}
                  saved={saved}
                  readOnly={!allowCustom}
                  save={saveRules}
                  onSaved={onSaved}
                  onDirtyChange={onRulesDirty}
                />
              )}
            </QueryStatus>
          )}
        </QueryStatus>
      </Tabs.Panel>

      <Tabs.Panel id="nodes" shouldForceMount className="p-0 data-[inert=true]:hidden">
        {visited.has('nodes') ? (
          <QueryStatus query={detail} minHeight={640}>
            {(saved) =>
              supportsProfiles || saved.nodes !== undefined ? (
                <NodeOpsSection
                  saved={saved.nodes}
                  readOnly={!allowCustom}
                  save={saveNodes}
                  onDirtyChange={onNodesDirty}
                />
              ) : (
                <Card>
                  <p className="text-subhead text-label-secondary">{t('nodeOps.unsupported')}</p>
                </Card>
              )
            }
          </QueryStatus>
        ) : null}
      </Tabs.Panel>

      <Tabs.Panel id="preview" className="p-0">
        {tab === 'preview' ? (
          <section className="surface-card flex flex-col gap-4 p-6" aria-labelledby="preview-title">
            <div className="flex min-w-0 items-center gap-3">
              <span
                className="flex size-8 items-center justify-center rounded-[10px] bg-fill-tertiary text-label-secondary"
                aria-hidden="true"
              >
                <FileCode2 className="size-4" />
              </span>
              <div className="min-w-0">
                <h2 id="preview-title" className="text-headline text-label">
                  {t('preview.title')}
                </h2>
                <p className="text-footnote text-label-secondary">
                  {override ? t('preview.hintOverride') : t('preview.hint')}
                </p>
              </div>
            </div>
            <Suspense fallback={<div className="h-48 animate-pulse rounded-[12px] bg-fill-quaternary" />}>
              <PreviewPanel
                profile={active.is_default || !active.slug ? override || undefined : active.slug}
              />
            </Suspense>
          </section>
        ) : null}
      </Tabs.Panel>
    </Tabs>
  )
}

function NodeOpsSection(props: Omit<Parameters<typeof NodeOpsEditor>[0], 'nodes'>) {
  const nodes = useQuery(userNodesQuery())
  // A failed node list only disables the preview; the editor still works.
  return <NodeOpsEditor {...props} nodes={nodes.isError ? [] : nodes.data} />
}

function Card({ children }: { children: ReactNode }) {
  return <section className="surface-card p-6">{children}</section>
}
