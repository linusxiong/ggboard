import { useQuery } from '@tanstack/react-query'
import { createFileRoute } from '@tanstack/react-router'
import { PageShell } from '@/components/PageShell'
import { SegmentedTabs } from '@/components/SegmentedControl'
import { QueryStatus } from '@/features/dashboard/QueryStatus'
import { AccountPanel } from '@/features/security/AccountPanel'
import { ChangePasswordCard } from '@/features/security/ChangePasswordCard'
import { PasskeysCard } from '@/features/security/PasskeysCard'
import { securityQuery } from '@/features/security/api'
import { TelegramCard } from '@/features/security/TelegramCard'
import { TwoFactorCard } from '@/features/security/TwoFactorCard'
import { SubscriptionInfoCard } from '@/features/subscription/SubscriptionInfoCard'
import { loadNamespaces, useT } from '@/i18n'
import { guestConfigQuery, meQuery, subscriptionQuery } from '@/lib/api/queries'
import { useAuthStore } from '@/stores/auth'

type ProfileTab = 'account' | 'security' | 'subscription'
const TABS: readonly ProfileTab[] = ['account', 'security', 'subscription']
const isTab = (value: unknown): value is ProfileTab => TABS.includes(value as ProfileTab)

export const Route = createFileRoute('/_user/profile')({
  // `/.well-known/change-password` and `/.well-known/passkey-endpoints` point at `/profile?tab=security`.
  validateSearch: (search: Record<string, unknown>): { tab?: ProfileTab } =>
    isTab(search.tab) ? { tab: search.tab } : {},
  loaderDeps: ({ search: { tab } }) => ({ tab: tab ?? 'account' }),
  // Loaders stay in the entry bundle (only components are auto-split): keep the query factories lazy.
  loader: async ({ context: { queryClient }, deps: { tab } }) => {
    const [q, sec] = await Promise.all([
      import('@/lib/api/queries'),
      import('@/features/security/api'),
      loadNamespaces(['security', 'subscription']),
    ])
    const prime = (p: Promise<unknown>) => p.catch(() => undefined)
    await Promise.all([
      prime(queryClient.query({ ...q.meQuery(), staleTime: 'static' })),
      tab === 'security' ? prime(queryClient.query({ ...sec.securityQuery(), staleTime: 'static' })) : null,
      tab === 'subscription'
        ? prime(queryClient.query({ ...q.subscriptionQuery(), staleTime: 'static' }))
        : null,
    ])
  },
  component: ProfilePage,
})

function ProfilePage() {
  const t = useT(['security', 'common'])
  const { tab = 'account' } = Route.useSearch()
  const navigate = Route.useNavigate()
  const stored = useAuthStore((s) => s.user)
  const me = useQuery(meQuery())
  const user = me.data ?? stored

  return (
    <PageShell title={t('profile.title')} description={t('profile.subtitle')}>
      <SegmentedTabs
        value={tab}
        onChange={(next) => {
          if (isTab(next) && next !== tab)
            void navigate({
              search: next === 'account' ? {} : { tab: next },
              replace: true,
              resetScroll: false,
            })
        }}
        className="flex flex-col gap-6"
      >
        <SegmentedTabs.List
          aria-label={t('profile.tabs')}
          items={TABS.map((id) => ({ id, label: t(`profile.tab.${id}`) }))}
        />
        <SegmentedTabs.Panel id="account" className="flex flex-col gap-6">
          {user ? <AccountPanel user={user} /> : null}
          <TelegramCard />
        </SegmentedTabs.Panel>
        <SegmentedTabs.Panel id="security">
          {user ? <SecurityPanel email={user.email} /> : null}
        </SegmentedTabs.Panel>
        <SegmentedTabs.Panel id="subscription">
          {user ? <SubscriptionPanel uuid={user.uuid} /> : null}
        </SegmentedTabs.Panel>
      </SegmentedTabs>
    </PageShell>
  )
}

function SecurityPanel({ email }: { email: string }) {
  const overview = useQuery(securityQuery())
  const config = useQuery(guestConfigQuery())
  const appName = config.data?.app_name || 'GG Bond'
  return (
    <div className="flex flex-col gap-6">
      <ChangePasswordCard email={email} />
      <QueryStatus query={overview} minHeight={360}>
        {(data) => (
          <>
            <TwoFactorCard overview={data} email={email} appName={appName} />
            <PasskeysCard
              passkeys={data.passkeys}
              email={email}
              siteEnabled={config.data?.passkey_enabled !== false}
            />
          </>
        )}
      </QueryStatus>
    </div>
  )
}

function SubscriptionPanel({ uuid }: { uuid: string }) {
  const sub = useQuery(subscriptionQuery())
  return <SubscriptionInfoCard url={sub.data?.subscribe_url} uuid={uuid} />
}
