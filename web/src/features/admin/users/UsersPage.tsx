/**
 * `/admin/users` — server-paginated user list with URL filters, detail drawer and admin tools
 * (create, batch generate, CSV export, bulk mail). Staff can list and view users only.
 */
import { Button, Chip } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { Download, Eye, Mail, Plus, UsersRound } from 'lucide-react'
import { useMemo, useState } from 'react'
import { createDataTableColumns, DataTable } from '@/components/data-table'
import { PageShell } from '@/components/PageShell'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import { adminGroupsQuery, adminPlansQuery, adminUsersQuery } from '@/lib/api/queries'
import type { AdminUser } from '@/lib/api/types'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { downloadBlob, stampedName } from '../shared/download'
import { useIsAdmin } from '../shared/permissions'
import { useFormatters } from '../shared/useFormatters'
import { useNow } from '../shared/useNow'
import { clearFilters, toFilterParams, toUserFilters, type UserSearch } from './filters'
import { LoginAsDialog, UserActionsMenu } from './UserActionsMenu'
import { CreateUserDialog, GenerateUsersDialog, MailUsersDialog } from './UserDialogs'
import { UserDrawer } from './UserDrawer'
import { UserFilterBar } from './UserFilterBar'
import { useUserActions } from './useUserActions'

const route = getRouteApi('/_admin/admin/users')
const col = createDataTableColumns<AdminUser>()
type Dialog = 'create' | 'generate' | 'mail' | null

export function UsersPage() {
  const t = useT(['admin-users', 'common'])
  const fmt = useFormatters()
  const canEdit = useIsAdmin()
  const now = useNow()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const users = useQuery(adminUsersQuery(toUserFilters(search)))
  const plans = useQuery(adminPlansQuery())
  const groups = useQuery(adminGroupsQuery())
  const [openUser, setOpenUser] = useState<number | null>(null)
  const [dialog, setDialog] = useState<Dialog>(null)
  const [exporting, setExporting] = useState(false)
  const actions = useUserActions((id) => setOpenUser((current) => (current === id ? null : current)))

  const setSearch = (patch: Partial<UserSearch>) =>
    void navigate({ search: (prev: UserSearch) => ({ ...prev, ...patch, page: 1 }), replace: true })

  const planName = useMemo(() => new Map((plans.data ?? []).map((p) => [p.id, p.name])), [plans.data])
  const groupName = useMemo(() => new Map((groups.data ?? []).map((g) => [g.id, g.name])), [groups.data])

  /** Human-readable active filters (bulk mail confirmation). */
  const filterSummary = useMemo(() => {
    const out: string[] = []
    if (search.email) out.push(`${t('filters.email')}: ${search.email}`)
    if (search.plan_id)
      out.push(`${t('filters.plan')}: ${planName.get(search.plan_id) ?? `#${search.plan_id}`}`)
    if (search.group_id)
      out.push(`${t('filters.group')}: ${groupName.get(search.group_id) ?? `#${search.group_id}`}`)
    if (search.banned !== undefined) out.push(search.banned ? t('filters.banned') : t('filters.normal'))
    if (search.role) out.push(t(`roles.${search.role}`))
    if (search.expired) out.push(t(`expiry.${search.expired}`))
    if (search.invite_user_id) out.push(`${t('filters.inviter')}: #${search.invite_user_id}`)
    if (search.id) out.push(`ID: ${search.id}`)
    if (search.uuid) out.push(`UUID: ${search.uuid}`)
    if (search.token) out.push(`${t('filters.token')}: ${search.token}`)
    if (search.remarks) out.push(`${t('filters.remarks')}: ${search.remarks}`)
    return out
  }, [search, t, planName, groupName])

  const exportCsv = async () => {
    setExporting(true)
    try {
      const blob = await adminApi.exportUsers(toFilterParams(search))
      downloadBlob(blob, stampedName('users', 'csv'))
      toast.success(t('export.done'))
    } catch (error) {
      toast.danger(errorMessage(error))
    } finally {
      setExporting(false)
    }
  }

  const columns = useMemo(
    () =>
      col.columns([
        col.accessor('id', {
          header: 'ID',
          meta: { hideOnMobile: true, className: 'w-16' },
          cell: (c) => <span className="tabular text-label-secondary">{c.getValue()}</span>,
        }),
        col.accessor('email', {
          header: t('columns.email'),
          enableSorting: false,
          meta: { isPrimary: true, isRowHeader: true },
          cell: (c) => {
            const u = c.row.original
            return (
              <span className="flex min-w-44 flex-col items-start gap-1">
                <button
                  type="button"
                  onClick={() => setOpenUser(u.id)}
                  className="break-all text-start font-medium text-label-blue underline-offset-2 hover:underline focus-visible:rounded-[6px] focus-visible:outline-2 focus-visible:outline-focus"
                >
                  {u.email}
                </button>
                {u.is_admin || u.is_staff || u.banned ? (
                  <span className="flex flex-wrap gap-1">
                    {u.is_admin ? (
                      <Chip size="sm" variant="soft" color="accent">
                        {t('roles.admin')}
                      </Chip>
                    ) : null}
                    {u.is_staff ? (
                      <Chip size="sm" variant="soft" color="accent">
                        {t('roles.staff')}
                      </Chip>
                    ) : null}
                    {u.banned ? (
                      <Chip size="sm" variant="soft" color="danger">
                        {t('filters.banned')}
                      </Chip>
                    ) : null}
                  </span>
                ) : null}
                {u.remarks ? (
                  <span className="line-clamp-1 text-footnote text-label-secondary">{u.remarks}</span>
                ) : null}
              </span>
            )
          },
        }),
        col.accessor('plan', {
          header: t('columns.plan'),
          enableSorting: false,
          cell: (c) =>
            c.getValue()?.name ?? <span className="text-label-secondary">{t('detail.noPlan')}</span>,
        }),
        col.display({
          id: 'traffic',
          header: t('columns.traffic'),
          cell: (c) => {
            const u = c.row.original
            const used = u.u + u.d
            const ratio = u.transfer_enable > 0 ? Math.min(1, used / u.transfer_enable) : 0
            const tone = ratio >= 0.9 ? 'bg-system-red' : ratio >= 0.7 ? 'bg-system-orange' : 'bg-system-blue'
            return (
              <span className="flex min-w-32 flex-col gap-1">
                <span className="tabular text-footnote text-label">
                  {fmt.bytes(used)} /{' '}
                  {u.transfer_enable > 0 ? fmt.bytes(u.transfer_enable) : t('common:values.unlimited')}
                </span>
                <span
                  className="block h-1 w-full overflow-hidden rounded-full bg-fill-tertiary"
                  aria-hidden="true"
                >
                  <span
                    className={`block h-full rounded-full ${tone}`}
                    style={{ width: `${Math.round(ratio * 100)}%` }}
                  />
                </span>
              </span>
            )
          },
        }),
        col.accessor('expired_at', {
          header: t('columns.expiredAt'),
          cell: (c) => {
            const v = c.getValue()
            if (!v) return <span className="text-label-secondary">{t('common:values.never')}</span>
            const past = new Date(v).getTime() < now
            return (
              <span className={past ? 'text-label-red' : 'text-label'}>
                {fmt.date(v)}
                {past ? <span className="block text-footnote">{t('expiry.expired')}</span> : null}
              </span>
            )
          },
        }),
        col.accessor('balance', {
          header: t('columns.balance'),
          meta: { align: 'end' },
          cell: (c) => <span className="tabular">{fmt.money(c.getValue())}</span>,
        }),
        col.accessor('created_at', {
          header: t('columns.createdAt'),
          meta: { hideOnMobile: true },
          cell: (c) => <span className="tabular text-label-secondary">{fmt.date(c.getValue())}</span>,
        }),
      ]),
    [t, fmt, now],
  )

  return (
    <PageShell
      title={t('title')}
      description={users.data ? t('description', { count: users.data.total }) : t('descriptionLoading')}
      actions={
        canEdit ? (
          <>
            <Button variant="secondary" onPress={() => setDialog('mail')}>
              <Mail className="size-4" aria-hidden="true" />
              {t('toolbar.mail')}
            </Button>
            <Button variant="secondary" isPending={exporting} onPress={() => void exportCsv()}>
              <Download className="size-4" aria-hidden="true" />
              {t('toolbar.export')}
            </Button>
            <Button variant="secondary" onPress={() => setDialog('generate')}>
              <UsersRound className="size-4" aria-hidden="true" />
              {t('toolbar.generate')}
            </Button>
            <Button onPress={() => setDialog('create')}>
              <Plus className="size-4" aria-hidden="true" />
              {t('toolbar.create')}
            </Button>
          </>
        ) : null
      }
    >
      <UserFilterBar
        search={search}
        onChange={setSearch}
        onReset={() => void navigate({ search: clearFilters(search), replace: true })}
      />
      {users.isError && !users.data ? (
        <p role="alert" className="text-subhead text-label-red">
          {errorMessage(users.error)}
        </p>
      ) : null}
      <DataTable
        aria-label={t('title')}
        columns={columns}
        data={users.data?.items}
        total={users.data?.total}
        page={search.page}
        pageSize={search.page_size}
        onPageChange={(page) => void navigate({ search: (prev: UserSearch) => ({ ...prev, page }) })}
        sort={search.sort}
        onSortChange={(sort) => void navigate({ search: (prev: UserSearch) => ({ ...prev, sort, page: 1 }) })}
        isLoading={users.isPending}
        isFetching={users.isFetching && !users.isPending}
        getRowId={(u) => String(u.id)}
        emptyState={<p className="py-8 text-center text-subhead text-label-secondary">{t('empty')}</p>}
        rowActions={(u) => (
          <>
            <Button
              size="sm"
              variant="ghost"
              isIconOnly
              aria-label={t('actions.viewNamed', { name: u.email })}
              onPress={() => setOpenUser(u.id)}
            >
              <Eye className="size-4" aria-hidden="true" />
            </Button>
            {canEdit ? <UserActionsMenu user={u} onAction={(action) => void actions.run(action, u)} /> : null}
          </>
        )}
      />

      {openUser !== null ? (
        <UserDrawer key={openUser} userId={openUser} canEdit={canEdit} onClose={() => setOpenUser(null)} />
      ) : null}
      <LoginAsDialog value={actions.loginUrl} onClose={actions.closeLoginUrl} />
      {dialog === 'create' ? (
        <CreateUserDialog
          isOpen
          onOpenChange={(open) => !open && setDialog(null)}
          onCreated={(id) => setOpenUser(id)}
        />
      ) : null}
      {dialog === 'generate' ? (
        <GenerateUsersDialog isOpen onOpenChange={(open) => !open && setDialog(null)} />
      ) : null}
      {dialog === 'mail' ? (
        <MailUsersDialog
          isOpen
          onOpenChange={(open) => !open && setDialog(null)}
          filters={toFilterParams(search)}
          matching={users.data?.total}
          filterSummary={filterSummary}
        />
      ) : null}
    </PageShell>
  )
}
