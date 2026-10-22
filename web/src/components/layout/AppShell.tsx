/**
 * Authenticated shell shared by the user and admin areas:
 * - desktop: collapsible sidebar (`md`+) + glass top bar,
 * - mobile: glass top bar + bottom tab bar (4 primary items + "More").
 * Renders `<main id="main">` with the route `<Outlet/>` inside a Suspense boundary.
 */
import { Avatar, Button, Dropdown, Header, Label, Separator } from '@heroui/react'
import { type QueryClient, QueryClientProvider, useQuery, useQueryClient } from '@tanstack/react-query'
import { Link, Outlet, useNavigate } from '@tanstack/react-router'
import { Ellipsis, LogOut, PanelLeftClose, PanelLeftOpen, Shield, UserRound } from 'lucide-react'
import { Suspense, useEffect } from 'react'
import { AriaProviders } from '@/components/AriaProviders'
import { ConfirmHost } from '@/components/feedback/ConfirmHost'
import { LoadingPage } from '@/components/LoadingPage'
import { LocaleToggle } from '@/components/LocaleToggle'
import { Logo } from '@/components/Logo'
import { ThemeToggle } from '@/components/ThemeToggle'
import { useT } from '@/i18n'
import { meQuery } from '@/lib/api/queries'
import * as userApi from '@/lib/api/user'
import { cx } from '@/lib/cn'
import { toast } from '@/lib/toast'
import { isAdmin, isStaff, useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import type { NavItem } from './nav'

interface AppShellProps {
  area: 'user' | 'admin'
  items: readonly NavItem[]
  /** From the layout's router context (`withQueryClient` loads React Query lazily). */
  queryClient: QueryClient
}

export function AppShell({ queryClient, ...props }: AppShellProps) {
  return (
    <QueryClientProvider client={queryClient}>
      <AriaProviders>
        <Shell {...props} />
      </AriaProviders>
    </QueryClientProvider>
  )
}

function Shell({ area, items }: Omit<AppShellProps, 'queryClient'>) {
  const t = useT(['nav', 'common', 'core'])
  const user = useAuthStore((s) => s.user)
  const token = useAuthStore((s) => s.token)
  const setUser = useAuthStore((s) => s.setUser)
  const collapsed = useUiStore((s) => s.sidebarCollapsed)
  const toggleSidebar = useUiStore((s) => s.toggleSidebar)

  // Keep the persisted user fresh (plan, balance, flags).
  const me = useQuery({ ...meQuery(), enabled: Boolean(token) })
  useEffect(() => {
    if (me.data) setUser(me.data)
  }, [me.data, setUser])

  const visible = area === 'admin' && !isAdmin(user) ? items.filter((i) => i.staff) : items
  const primary = visible.filter((i) => i.primary).slice(0, 4)
  const more = visible.filter((i) => !primary.includes(i))

  return (
    <div className="min-h-dvh md:flex">
      <a
        href="#main"
        className="sr-only focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50 focus:rounded-[10px] focus:bg-elevated focus:px-3 focus:py-2"
      >
        {t('core:a11y.skipToContent')}
      </a>

      {/* ---------------------------------------------------------------- sidebar */}
      <aside
        className={cx(
          'sticky top-0 hidden h-dvh shrink-0 flex-col border-e-[0.5px] border-hairline bg-elevated md:flex',
          'transition-[width] duration-200 ease-out',
          collapsed ? 'w-[76px]' : 'w-64',
        )}
      >
        <div className={cx('flex h-14 items-center', collapsed ? 'justify-center' : 'px-5')}>
          <Link to={area === 'admin' ? '/admin/dashboard' : '/dashboard'} className="rounded-[10px]">
            <Logo withText={!collapsed} text={t('core:appName')} />
          </Link>
        </div>
        <nav aria-label={t('mainNavigation')} className="flex-1 overflow-y-auto px-3 py-2">
          <ul className="flex flex-col gap-0.5">
            {visible.map((item) => (
              <li key={item.to}>
                <SidebarLink item={item} label={t(item.label)} collapsed={collapsed} />
              </li>
            ))}
          </ul>
        </nav>
        <div className={cx('flex items-center gap-1 p-3', collapsed ? 'flex-col' : 'justify-between')}>
          {isStaff(user) ? (
            <Link
              to={area === 'admin' ? '/dashboard' : '/admin/dashboard'}
              className={cx(
                'flex items-center gap-2 rounded-[10px] px-3 py-2 text-footnote text-label-secondary hover:bg-fill-quaternary',
                collapsed && 'justify-center px-2',
              )}
              title={collapsed ? (area === 'admin' ? t('userPanel') : t('adminPanel')) : undefined}
            >
              {area === 'admin' ? (
                <UserRound className="size-4" aria-hidden="true" />
              ) : (
                <Shield className="size-4" aria-hidden="true" />
              )}
              <span className={collapsed ? 'sr-only' : undefined}>
                {area === 'admin' ? t('userPanel') : t('adminPanel')}
              </span>
            </Link>
          ) : (
            <span />
          )}
          <Button
            isIconOnly
            size="sm"
            variant="ghost"
            aria-label={collapsed ? t('expandSidebar') : t('collapseSidebar')}
            onPress={toggleSidebar}
          >
            {collapsed ? (
              <PanelLeftOpen className="size-[18px]" aria-hidden="true" />
            ) : (
              <PanelLeftClose className="size-[18px]" aria-hidden="true" />
            )}
          </Button>
        </div>
      </aside>

      {/* ---------------------------------------------------------------- content */}
      <div className="flex min-w-0 flex-1 flex-col">
        <header className="glass glass-border-b sticky top-0 z-30 flex h-14 items-center justify-between gap-3 px-4 sm:px-6">
          <div className="flex min-w-0 items-center gap-2">
            <Link
              to={area === 'admin' ? '/admin/dashboard' : '/dashboard'}
              className="rounded-[10px] md:hidden"
            >
              <Logo text={t('core:appName')} />
            </Link>
            <span className="truncate text-headline text-label">
              {area === 'admin' ? t('adminPanel') : t('userPanel')}
            </span>
          </div>
          <div className="flex items-center gap-1">
            <LocaleToggle compact />
            <ThemeToggle />
            <UserMenu area={area} />
          </div>
        </header>

        <main
          id="main"
          tabIndex={-1}
          className="flex-1 px-4 pt-6 pb-28 outline-none sm:px-6 md:pb-12 lg:px-10"
        >
          <Suspense fallback={<LoadingPage />}>
            <Outlet />
          </Suspense>
        </main>
      </div>

      {/* ---------------------------------------------------------------- mobile tab bar */}
      <nav
        aria-label={t('mainNavigation')}
        className="glass glass-border-t pb-safe fixed inset-x-0 bottom-0 z-30 md:hidden"
      >
        <ul className="mx-auto flex max-w-lg items-stretch justify-around px-2">
          {primary.map((item) => (
            <li key={item.to} className="flex-1">
              <TabLink item={item} label={t(item.label)} />
            </li>
          ))}
          {more.length > 0 ? (
            <li className="flex-1">
              <Dropdown>
                <Button
                  variant="ghost"
                  className="h-auto w-full flex-col gap-0.5 rounded-none py-2 text-caption2 font-normal text-label-secondary"
                >
                  <Ellipsis className="size-6" aria-hidden="true" />
                  <span>{t('more')}</span>
                </Button>
                <Dropdown.Popover placement="top end">
                  <Dropdown.Menu aria-label={t('more')}>
                    {more.map((item) => (
                      <Dropdown.Item key={item.to} id={item.to} href={item.to} textValue={t(item.label)}>
                        <item.icon className="size-4 text-label-secondary" aria-hidden="true" />
                        <Label>{t(item.label)}</Label>
                      </Dropdown.Item>
                    ))}
                  </Dropdown.Menu>
                </Dropdown.Popover>
              </Dropdown>
            </li>
          ) : null}
        </ul>
      </nav>

      <ConfirmHost />
    </div>
  )
}

function SidebarLink({ item, label, collapsed }: { item: NavItem; label: string; collapsed: boolean }) {
  const Icon = item.icon
  return (
    <Link
      to={item.to}
      title={collapsed ? label : undefined}
      className={cx(
        'group flex items-center gap-3 rounded-[10px] px-3 py-2 text-subhead text-label transition-colors',
        'hover:bg-fill-quaternary data-[status=active]:bg-fill-tertiary data-[status=active]:font-semibold',
        collapsed && 'justify-center px-2',
      )}
    >
      <Icon
        className="size-[18px] shrink-0 text-label-secondary group-data-[status=active]:text-system-blue"
        aria-hidden="true"
      />
      <span className={collapsed ? 'sr-only' : 'truncate'}>{label}</span>
    </Link>
  )
}

function TabLink({ item, label }: { item: NavItem; label: string }) {
  const Icon = item.icon
  return (
    <Link
      to={item.to}
      className="flex flex-col items-center gap-0.5 py-2 text-caption2 text-label-secondary data-[status=active]:text-label-blue"
    >
      <Icon className="size-6" aria-hidden="true" />
      <span>{label}</span>
    </Link>
  )
}

function UserMenu({ area }: { area: 'user' | 'admin' }) {
  const t = useT(['nav', 'common'])
  const user = useAuthStore((s) => s.user)
  const navigate = useNavigate()
  const queryClient = useQueryClient()
  const initial = user?.email.charAt(0).toUpperCase() ?? '?'

  const logout = async () => {
    await userApi.logout().catch(() => undefined)
    useAuthStore.getState().logout()
    toast(t('loggedOut'))
    // Leave the shell before clearing the cache: clearing first makes every still-mounted query (e.g. the
    // profile page's `meQuery`) refetch without a token → a pointless 401 + console error.
    await navigate({ to: '/login', replace: true })
    queryClient.clear()
  }

  return (
    <Dropdown>
      <Button variant="ghost" size="sm" isIconOnly aria-label={t('userMenu')} className="rounded-full">
        <Avatar size="sm" className="size-8">
          {user?.avatar_url ? <Avatar.Image src={user.avatar_url} alt="" /> : null}
          <Avatar.Fallback>{initial}</Avatar.Fallback>
        </Avatar>
      </Button>
      <Dropdown.Popover placement="bottom end" className="min-w-56">
        <Dropdown.Menu
          aria-label={t('userMenu')}
          onAction={(key) => {
            if (key === 'logout') void logout()
          }}
        >
          <Dropdown.Section>
            <Header>
              <span className="block text-caption text-label-secondary">{t('signedInAs')}</span>
              <span className="block truncate text-subhead text-label">{user?.email}</span>
            </Header>
          </Dropdown.Section>
          <Separator />
          <Dropdown.Item id="profile" href="/profile" textValue={t('user.profile')}>
            <UserRound className="size-4 text-label-secondary" aria-hidden="true" />
            <Label>{t('user.profile')}</Label>
          </Dropdown.Item>
          {isStaff(user) ? (
            <Dropdown.Item
              id="switch"
              href={area === 'admin' ? '/dashboard' : '/admin/dashboard'}
              textValue={area === 'admin' ? t('userPanel') : t('adminPanel')}
            >
              {area === 'admin' ? (
                <UserRound className="size-4 text-label-secondary" aria-hidden="true" />
              ) : (
                <Shield className="size-4 text-label-secondary" aria-hidden="true" />
              )}
              <Label>{area === 'admin' ? t('userPanel') : t('adminPanel')}</Label>
            </Dropdown.Item>
          ) : null}
          <Dropdown.Item id="logout" textValue={t('logout')} variant="danger">
            <LogOut className="size-4" aria-hidden="true" />
            <Label>{t('logout')}</Label>
          </Dropdown.Item>
        </Dropdown.Menu>
      </Dropdown.Popover>
    </Dropdown>
  )
}
