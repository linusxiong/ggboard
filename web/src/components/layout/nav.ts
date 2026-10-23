/**
 * Navigation config for the user and admin shells. Feature agents: add/rename entries here
 * (append-only) — labels live in `src/i18n/<locale>/nav.json`.
 */
import type { LinkProps } from '@tanstack/react-router'
import type { LucideIcon } from 'lucide-react'
import {
  Activity,
  BookOpen,
  CircleUserRound,
  CreditCard,
  Gauge,
  Gift,
  Layers,
  LayoutDashboard,
  LifeBuoy,
  ListTree,
  Mail,
  Megaphone,
  Package,
  Receipt,
  Route as RouteIcon,
  Rss,
  Server,
  Settings,
  ShoppingBag,
  TicketPercent,
  Users,
} from 'lucide-react'

export interface NavItem {
  /** Route path (also the item id). Type-checked against the generated route tree. */
  to: NonNullable<LinkProps['to']>
  /** Key in `nav.json`. */
  label: string
  icon: LucideIcon
  /** Shown in the mobile tab bar (max 4; the rest go under "More"). */
  primary?: boolean
  /** Admin area: visible to staff (`is_staff`) as well as admins. */
  staff?: boolean
  /** Sidebar section (label `nav.sections.<id>`); items keep their array order inside a section. */
  section?: NavSection
}

export type NavSection = 'ops' | 'content' | 'system'
/** Display order of the sidebar sections. */
export const NAV_SECTIONS: readonly NavSection[] = ['ops', 'content', 'system']

export const USER_NAV: readonly NavItem[] = [
  { to: '/dashboard', label: 'user.dashboard', icon: LayoutDashboard, primary: true },
  { to: '/shop', label: 'user.shop', icon: ShoppingBag, primary: true },
  { to: '/subscription', label: 'user.subscription', icon: Rss, primary: true },
  { to: '/orders', label: 'user.orders', icon: Receipt, primary: true },
  { to: '/invite', label: 'user.invite', icon: Gift },
  { to: '/tickets', label: 'user.tickets', icon: LifeBuoy },
  { to: '/knowledge', label: 'user.knowledge', icon: BookOpen },
  { to: '/notices', label: 'user.notices', icon: Megaphone },
  { to: '/profile', label: 'user.profile', icon: CircleUserRound },
]

export const ADMIN_NAV: readonly NavItem[] = [
  { to: '/admin/dashboard', label: 'admin.dashboard', icon: Gauge, primary: true, section: 'ops' },
  { to: '/admin/users', label: 'admin.users', icon: Users, primary: true, staff: true, section: 'ops' },
  { to: '/admin/plans', label: 'admin.plans', icon: Package, section: 'ops' },
  { to: '/admin/groups', label: 'admin.groups', icon: Layers, section: 'system' },
  { to: '/admin/nodes', label: 'admin.nodes', icon: Server, primary: true, section: 'system' },
  { to: '/admin/routes', label: 'admin.routes', icon: RouteIcon, section: 'system' },
  { to: '/admin/orders', label: 'admin.orders', icon: Receipt, primary: true, section: 'ops' },
  { to: '/admin/coupons', label: 'admin.coupons', icon: TicketPercent, section: 'ops' },
  { to: '/admin/payments', label: 'admin.payments', icon: CreditCard, section: 'system' },
  { to: '/admin/tickets', label: 'admin.tickets', icon: LifeBuoy, staff: true, section: 'ops' },
  { to: '/admin/notices', label: 'admin.notices', icon: Megaphone, staff: true, section: 'content' },
  { to: '/admin/knowledge', label: 'admin.knowledge', icon: BookOpen, staff: true, section: 'content' },
  { to: '/admin/settings', label: 'admin.settings', icon: Settings, section: 'system' },
  { to: '/admin/mail-templates', label: 'admin.mailTemplates', icon: Mail, section: 'system' },
  { to: '/admin/rulesets', label: 'admin.rulesets', icon: ListTree, section: 'system' },
  { to: '/admin/system', label: 'admin.system', icon: Activity, section: 'system' },
]
