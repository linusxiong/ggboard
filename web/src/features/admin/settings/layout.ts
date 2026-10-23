/**
 * Presentation of the settings groups: sidebar icon + tint, titled sections inside each group,
 * dependent keys (shown under their switch/select only while it is on) and input units.
 * Purely visual — keys not listed here still render (in a trailing section), so new backend
 * settings never disappear.
 */
import type { LucideIcon } from 'lucide-react'
import {
  Bell,
  Download,
  Globe,
  Lock,
  Mail,
  Palette,
  Receipt,
  Rss,
  Send,
  Server,
  SlidersHorizontal,
  UserPlus,
  Users,
} from 'lucide-react'
import type { SettingDef, SettingGroupMeta } from '@/lib/api/types'

/** Sidebar symbol + background (raw system color utility: icons only). */
export const GROUP_ICON: Record<string, { icon: LucideIcon; tint: string }> = {
  site: { icon: Globe, tint: 'bg-system-blue' },
  register: { icon: UserPlus, tint: 'bg-system-green' },
  subscribe: { icon: Rss, tint: 'bg-system-orange' },
  order: { icon: Receipt, tint: 'bg-system-teal' },
  invite: { icon: Users, tint: 'bg-system-pink' },
  server: { icon: Server, tint: 'bg-system-gray' },
  email: { icon: Mail, tint: 'bg-system-indigo' },
  frontend: { icon: Palette, tint: 'bg-system-purple' },
  app: { icon: Download, tint: 'bg-system-cyan' },
  security: { icon: Lock, tint: 'bg-system-red' },
  telegram: { icon: Send, tint: 'bg-system-blue' },
  notify: { icon: Bell, tint: 'bg-system-red' },
}
export const FALLBACK_ICON = { icon: SlidersHorizontal, tint: 'bg-system-gray' }

/** Sections per group, in display order (title/caption: `sections.<group>.<id>.*`). */
/**
 * Keys the server still lists (kept for compatibility) but that nothing reads any more, so they must
 * not fall into a group's trailing section. `email_template`: e-mails are customised per template on
 * /admin/mail-templates (docs/API.md "Settings keys").
 */
export const LEGACY_KEYS: ReadonlySet<string> = new Set(['email_template'])

export const GROUP_SECTIONS: Record<string, ReadonlyArray<{ id: string; keys: readonly string[] }>> = {
  site: [
    { id: 'identity', keys: ['app_name', 'app_description', 'logo', 'app_url', 'tos_url'] },
    { id: 'region', keys: ['default_locale', 'currency', 'currency_symbol'] },
    { id: 'privacy', keys: ['show_info_to_user'] },
  ],
  register: [
    {
      id: 'signup',
      keys: [
        'stop_register',
        'email_verify',
        'email_whitelist_enable',
        'email_whitelist_suffix',
        'invite_force',
      ],
    },
    { id: 'trial', keys: ['try_out_plan_id', 'try_out_hour'] },
    {
      id: 'protection',
      keys: [
        'register_limit_by_ip_enable',
        'register_limit_count',
        'register_limit_expire',
        'password_limit_enable',
        'password_limit_count',
        'password_limit_expire',
      ],
    },
    { id: 'captcha', keys: ['captcha_provider', 'captcha_site_key', 'captcha_secret'] },
  ],
  subscribe: [
    { id: 'links', keys: ['subscribe_url', 'subscribe_update_interval'] },
    { id: 'plans', keys: ['plan_change_enable', 'surplus_enable', 'reset_traffic_method'] },
    {
      id: 'content',
      keys: [
        'show_info_to_server_enable',
        'show_protocol_to_server_enable',
        'default_remind_expire',
        'default_remind_traffic',
      ],
    },
    {
      id: 'rules',
      keys: [
        'allow_user_rule_profile',
        'ruleset_default_template',
        'ruleset_default_source',
        'ruleset_mirror',
      ],
    },
    {
      id: 'protection',
      keys: [
        'subscribe_age_optional',
        'subscribe_age_strict',
        'subscribe_leak_max_ips',
        'subscribe_leak_action',
      ],
    },
  ],
  invite: [
    { id: 'codes', keys: ['invite_gen_limit', 'invite_never_expire'] },
    {
      id: 'commission',
      keys: ['invite_commission', 'commission_first_time_enable', 'commission_auto_check_enable'],
    },
    {
      id: 'withdraw',
      keys: ['withdraw_close_enable', 'commission_withdraw_limit', 'commission_withdraw_method'],
    },
    {
      id: 'distribution',
      keys: [
        'commission_distribution_enable',
        'commission_distribution_l1',
        'commission_distribution_l2',
        'commission_distribution_l3',
      ],
    },
  ],
  server: [
    { id: 'auth', keys: ['server_token'] },
    {
      id: 'sync',
      keys: ['server_pull_interval', 'server_push_interval', 'device_limit_mode', 'server_white_list'],
    },
  ],
  email: [
    {
      id: 'delivery',
      keys: [
        'email_driver',
        'email_host',
        'email_port',
        'email_encryption',
        'email_username',
        'email_password',
      ],
    },
    { id: 'sender', keys: ['email_from_address', 'email_from_name'] },
    { id: 'reminders', keys: ['remind_mail_enable'] },
  ],
  telegram: [
    { id: 'bot', keys: ['telegram_bot_token', 'telegram_webhook_secret'] },
    { id: 'admins', keys: ['telegram_admin_chat_ids', 'telegram_events'] },
    { id: 'users', keys: ['telegram_user_notify', 'telegram_discuss_link'] },
  ],
}

/**
 * Dependent key → the key that switches it on and the condition. A dependent renders under its
 * parent (in the same row) and is hidden while the condition is false — when the parent is part
 * of the same group; otherwise it stays visible but disabled.
 */
export const DEPENDS_ON: Record<string, { parent: string; when: (v: Record<string, unknown>) => boolean }> = {
  email_whitelist_suffix: {
    parent: 'email_whitelist_enable',
    when: (v) => v.email_whitelist_enable === true,
  },
  register_limit_count: {
    parent: 'register_limit_by_ip_enable',
    when: (v) => v.register_limit_by_ip_enable === true,
  },
  register_limit_expire: {
    parent: 'register_limit_by_ip_enable',
    when: (v) => v.register_limit_by_ip_enable === true,
  },
  password_limit_count: { parent: 'password_limit_enable', when: (v) => v.password_limit_enable === true },
  password_limit_expire: { parent: 'password_limit_enable', when: (v) => v.password_limit_enable === true },
  try_out_hour: { parent: 'try_out_plan_id', when: (v) => Number(v.try_out_plan_id) > 0 },
  captcha_site_key: { parent: 'captcha_provider', when: (v) => (v.captcha_provider ?? 'none') !== 'none' },
  captcha_secret: { parent: 'captcha_provider', when: (v) => (v.captcha_provider ?? 'none') !== 'none' },
  commission_distribution_l1: {
    parent: 'commission_distribution_enable',
    when: (v) => v.commission_distribution_enable === true,
  },
  commission_distribution_l2: {
    parent: 'commission_distribution_enable',
    when: (v) => v.commission_distribution_enable === true,
  },
  commission_distribution_l3: {
    parent: 'commission_distribution_enable',
    when: (v) => v.commission_distribution_enable === true,
  },
  subscribe_leak_action: {
    parent: 'subscribe_leak_max_ips',
    when: (v) => Number(v.subscribe_leak_max_ips) > 0,
  },
  email_host: { parent: 'email_driver', when: (v) => v.email_driver === 'smtp' },
  email_port: { parent: 'email_driver', when: (v) => v.email_driver === 'smtp' },
  email_encryption: { parent: 'email_driver', when: (v) => v.email_driver === 'smtp' },
  email_username: { parent: 'email_driver', when: (v) => v.email_driver === 'smtp' },
  email_password: { parent: 'email_driver', when: (v) => v.email_driver === 'smtp' },
}

/** Unit adornment (`units.<id>` in admin-settings.json) per integer key. */
export const UNITS: Record<string, string> = {
  register_limit_count: 'times',
  register_limit_expire: 'minutes',
  password_limit_count: 'times',
  password_limit_expire: 'minutes',
  try_out_hour: 'hours',
  subscribe_update_interval: 'hours',
  order_timeout_minutes: 'minutes',
  invite_commission: 'percent',
  invite_gen_limit: 'codes',
  commission_distribution_l1: 'percent',
  commission_distribution_l2: 'percent',
  commission_distribution_l3: 'percent',
  server_pull_interval: 'seconds',
  server_push_interval: 'seconds',
  subscribe_leak_max_ips: 'ips',
}

export interface RowModel {
  def: SettingDef
  /** Keys rendered under this one (its dependents present in the group). */
  children: SettingDef[]
}

export interface SectionModel {
  /** `null` = untitled (groups without a layout, or the trailing "other" section). */
  id: string | null
  rows: RowModel[]
}

/** Splits a group into titled sections of rows; dependents are nested under their parent. */
export function sectionsOf(group: SettingGroupMeta): SectionModel[] {
  const byKey = new Map(group.keys.map((d) => [d.key, d]))
  const nested = new Set(
    group.keys.filter((d) => DEPENDS_ON[d.key] && byKey.has(DEPENDS_ON[d.key]!.parent)).map((d) => d.key),
  )
  const rowsFor = (keys: readonly string[]): RowModel[] =>
    keys
      .filter((k) => byKey.has(k) && !nested.has(k))
      .map((k) => ({
        def: byKey.get(k)!,
        children: group.keys.filter((d) => nested.has(d.key) && DEPENDS_ON[d.key]!.parent === k),
      }))

  const layout = GROUP_SECTIONS[group.name] ?? []
  const listed = new Set(layout.flatMap((s) => s.keys))
  const sections: SectionModel[] = layout
    .map((s) => ({ id: s.id, rows: rowsFor(s.keys) }))
    .filter((s) => s.rows.length > 0)
  const rest = group.keys.map((d) => d.key).filter((k) => !listed.has(k) && !LEGACY_KEYS.has(k))
  const restRows = rowsFor(rest)
  // Dependents whose parent sits in another section of the layout are already nested there.
  if (restRows.length) sections.push({ id: layout.length ? 'other' : null, rows: restRows })
  return sections
}
