/**
 * `/admin/nodes` — node list with status, filters (URL), drag-to-reorder, show/hide toggle, create/edit
 * sheet and per-node drawers.
 */
import { Button, Chip, Dropdown, Label, ListBox, SearchField, Select } from '@heroui/react'
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query'
import { getRouteApi } from '@tanstack/react-router'
import { BarChart3, Copy, Ellipsis, Pencil, Plus, Server, Trash2, Users } from 'lucide-react'
import { type Key, useMemo, useState } from 'react'
import { EmptyState } from '@/components/EmptyState'
import { createDataTableColumns, DataTable, DragHandle, useRowReorder } from '@/components/data-table'
import { confirm } from '@/components/feedback/confirm'
import { PageShell } from '@/components/PageShell'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import { adminGroupsQuery, adminNodesQuery, qk } from '@/lib/api/queries'
import { type Node, NODE_TYPES } from '@/lib/api/types'
import { cx } from '@/lib/cn'
import { errorMessage } from '@/lib/errors'
import { toast } from '@/lib/toast'
import { useAdminMutation } from '../shared/useAdminMutation'
import { useFormatters } from '../shared/useFormatters'
import { useNow } from '../shared/useNow'
import { LoadBars, StatusBadge, VisibilityToggle } from './NodeBits'
import { NodeOnlineDrawer, NodeStatsDrawer } from './NodeDrawers'
import { NodeSheet } from './NodeSheet'
import type { NodeSearch } from './search'
import { nodeState } from './status'

const route = getRouteApi('/_admin/admin/nodes')
const col = createDataTableColumns<Node>()
const ALL = '__all__'

type Panel =
  { kind: 'sheet'; node: Node | null } | { kind: 'stats'; node: Node } | { kind: 'online'; node: Node }

function FilterSelect({
  label,
  value,
  options,
  onChange,
}: {
  label: string
  value: string | undefined
  options: ReadonlyArray<{ value: string; label: string }>
  onChange: (value: string | undefined) => void
}) {
  return (
    <Select
      value={value ?? ALL}
      onChange={(k: Key | null) => onChange(k === null || k === ALL ? undefined : String(k))}
      className="w-full sm:w-40"
    >
      <Label>{label}</Label>
      <Select.Trigger>
        <Select.Value />
        <Select.Indicator />
      </Select.Trigger>
      <Select.Popover>
        <ListBox>
          {options.map((o) => (
            <ListBox.Item key={o.value} id={o.value} textValue={o.label}>
              {o.label}
              <ListBox.ItemIndicator />
            </ListBox.Item>
          ))}
        </ListBox>
      </Select.Popover>
    </Select>
  )
}

export function NodesPage() {
  const t = useT(['admin-nodes', 'common'])
  const fmt = useFormatters()
  const search = route.useSearch()
  const navigate = route.useNavigate()
  const queryClient = useQueryClient()
  const nodes = useQuery(adminNodesQuery())
  const groups = useQuery(adminGroupsQuery())
  const [panel, setPanel] = useState<Panel | null>(null)
  const [query, setQuery] = useState(search.q ?? '')
  const now = useNow()

  const setSearch = (patch: Partial<NodeSearch>) =>
    void navigate({ search: (prev: NodeSearch) => ({ ...prev, ...patch }), replace: true })

  const groupName = useMemo(() => new Map((groups.data ?? []).map((g) => [g.id, g.name])), [groups.data])
  const all = useMemo(() => nodes.data ?? [], [nodes.data])
  const rowIds = useMemo(() => all.map((n) => String(n.id)), [all])
  const nameById = useMemo(() => new Map(all.map((n) => [String(n.id), n.name])), [all])
  const childCount = useMemo(() => {
    const m = new Map<number, number>()
    for (const n of all) if (n.parent_id) m.set(n.parent_id, (m.get(n.parent_id) ?? 0) + 1)
    return m
  }, [all])
  const filtered = Boolean(search.type || search.group || search.status || search.q)
  const rows = useMemo(() => {
    const q = search.q?.toLowerCase()
    return all.filter(
      (n) =>
        (!search.type || n.type === search.type) &&
        (!search.group || n.group_ids.includes(search.group)) &&
        (!search.status || nodeState(n.status, now) === search.status) &&
        (!q || n.name.toLowerCase().includes(q) || n.host.toLowerCase().includes(q) || String(n.id) === q),
    )
  }, [all, search, now])

  const patchNodes = (update: (rows: Node[]) => Node[]) => {
    const previous = queryClient.getQueryData<Node[]>(qk.admin.nodes())
    if (previous) queryClient.setQueryData(qk.admin.nodes(), update(previous))
    return { previous }
  }
  const rollback = (error: unknown, ctx: { previous?: Node[] } | undefined) => {
    if (ctx?.previous) queryClient.setQueryData(qk.admin.nodes(), ctx.previous)
    toast.danger(errorMessage(error))
  }

  const { mutate: toggleShowMutate } = useMutation({
    mutationFn: (v: { id: number; show: boolean }) => adminApi.updateNode(v.id, { show: v.show }),
    onMutate: async (v) => {
      await queryClient.cancelQueries({ queryKey: qk.admin.nodes() })
      return patchNodes((list) => list.map((n) => (n.id === v.id ? { ...n, show: v.show } : n)))
    },
    onError: (e, _v, ctx) => rollback(e, ctx),
    onSettled: () => queryClient.invalidateQueries({ queryKey: qk.admin.nodes() }),
  })
  const { mutate: reorderMutate, isPending: reorderPending } = useMutation({
    mutationFn: (ids: number[]) => adminApi.sortNodes(ids),
    onMutate: async (ids) => {
      await queryClient.cancelQueries({ queryKey: qk.admin.nodes() })
      return patchNodes((list) => {
        const byId = new Map(list.map((n) => [n.id, n]))
        return ids.flatMap((id, i) => {
          const n = byId.get(id)
          return n ? [{ ...n, sort: i + 1 }] : []
        })
      })
    },
    onError: (e, _ids, ctx) => rollback(e, ctx),
    onSettled: () => queryClient.invalidateQueries({ queryKey: qk.admin.nodes() }),
  })
  // Sort = position in the full list, so reordering is off while filtered (help text says so).
  const dragAndDropHooks = useRowReorder({
    ids: rowIds,
    getLabel: (id) => nameById.get(id) ?? id,
    onReorder: (ids) => reorderMutate(ids.map(Number)),
    isDisabled: filtered || reorderPending,
  })
  const copy = useAdminMutation({
    mutationFn: (id: number) => adminApi.copyNode(id),
    // A copy of a node with its own token gets a fresh one (its backend needs the new token).
    success: (n) => t(n.token ? 'nodes.copiedNewToken' : 'nodes.copied', { name: n.name }),
    invalidate: [qk.admin.nodes()],
  })
  const remove = useAdminMutation({
    mutationFn: (id: number) => adminApi.deleteNode(id),
    success: t('nodes.deleted'),
    invalidate: [qk.admin.nodes(), qk.admin.groups()],
  })

  const askDelete = (n: Node) => {
    const children = childCount.get(n.id) ?? 0
    void confirm({
      title: t('nodes.deleteTitle', { name: n.name }),
      description: children ? t('nodes.deleteChildren', { count: children }) : t('nodes.deleteHint'),
      confirmLabel: t('common:actions.delete'),
      tone: 'danger',
    }).then((yes) => yes && remove.mutate(n.id))
  }

  const columns = useMemo(
    () =>
      col.columns([
        col.display({
          id: 'order',
          header: () => <span className="sr-only">{t('nodes.columns.order')}</span>,
          // Handle only: 16 px gutter + 32 px button, the name column supplies the gap.
          meta: { hideOnMobile: true, className: 'w-px pe-0' },
          cell: (c) => <DragHandle label={t('nodes.reorder', { name: c.row.original.name })} />,
        }),
        col.accessor('name', {
          header: t('nodes.columns.name'),
          meta: { isPrimary: true, isRowHeader: true },
          cell: (c) => {
            const n = c.row.original
            return (
              <span className="flex min-w-36 flex-col gap-1">
                <span className="flex items-center gap-1.5">
                  <span className="tabular text-footnote text-label-secondary">#{n.id}</span>
                  <button
                    type="button"
                    onClick={() => setPanel({ kind: 'sheet', node: n })}
                    aria-label={t('nodes.editNamed', { name: n.name })}
                    className={cx(
                      'text-start font-medium underline-offset-2 hover:underline focus-visible:rounded-[6px] focus-visible:outline-2 focus-visible:outline-focus',
                      // Hidden from users → dimmed (the eye-off toggle in the row says why).
                      n.show ? 'text-label-blue' : 'text-label-secondary',
                    )}
                  >
                    {n.name}
                  </button>
                </span>
                {n.parent_id ? (
                  <span className="text-footnote text-label-secondary">
                    {t('nodes.childOf', { name: n.parent ?? `#${n.parent_id}` })}
                  </span>
                ) : null}
                {n.tags.length ? (
                  <span className="flex flex-wrap gap-1">
                    {n.tags.map((tag) => (
                      <Chip key={tag} size="sm" variant="soft">
                        {tag}
                      </Chip>
                    ))}
                  </span>
                ) : null}
              </span>
            )
          },
        }),
        col.accessor('type', {
          header: t('nodes.columns.type'),
          cell: (c) => (
            <span className="flex flex-col items-start gap-1 whitespace-nowrap">
              <Chip size="sm" variant="soft" color="accent">
                {t(`nodes.types.${c.getValue()}`)}
              </Chip>
              <span className="tabular text-caption text-label-secondary">
                {t('nodes.rateShort', { rate: c.row.original.rate })}
              </span>
            </span>
          ),
        }),
        col.display({
          id: 'address',
          header: t('nodes.columns.address'),
          cell: (c) => {
            const address = `${c.row.original.host}:${c.row.original.port}`
            return (
              <span className="flex flex-col whitespace-nowrap">
                <span
                  className="block max-w-48 truncate font-mono text-footnote text-label md:max-w-36 xl:max-w-60"
                  title={address}
                >
                  {address}
                </span>
                <span className="text-caption text-label-secondary">
                  {t('nodes.serverPortShort', { port: c.row.original.server_port })}
                </span>
              </span>
            )
          },
        }),
        col.accessor('group_ids', {
          header: t('nodes.columns.groups'),
          cell: (c) => {
            const ids = c.getValue()
            if (!ids.length)
              return (
                <span className="whitespace-nowrap text-footnote text-label-orange">
                  {t('nodes.noGroups')}
                </span>
              )
            return (
              <span className="flex flex-wrap gap-1 whitespace-nowrap">
                {ids.slice(0, 2).map((id) => (
                  <Chip key={id} size="sm" variant="secondary">
                    {groupName.get(id) ?? `#${id}`}
                  </Chip>
                ))}
                {ids.length > 2 ? (
                  <span className="text-caption text-label-secondary">+{ids.length - 2}</span>
                ) : null}
              </span>
            )
          },
        }),
        col.accessor('status', {
          header: t('nodes.columns.status'),
          cell: (c) => {
            const count = c.getValue()?.online ?? 0
            return (
              <span className="flex flex-col items-start gap-0.5 whitespace-nowrap">
                <StatusBadge status={c.getValue()} fmt={fmt} />
                {count > 0 ? (
                  <button
                    type="button"
                    onClick={() => setPanel({ kind: 'online', node: c.row.original })}
                    aria-label={t('nodes.online.open', { name: c.row.original.name, count })}
                    className="tabular text-footnote text-label-blue underline-offset-2 hover:underline focus-visible:rounded-[6px] focus-visible:outline-2 focus-visible:outline-focus"
                  >
                    {t('nodes.onlineCount', { count: fmt.count(count) })}
                  </button>
                ) : (
                  <span className="text-footnote text-label-secondary">
                    {t('nodes.onlineCount', { count: 0 })}
                  </span>
                )}
              </span>
            )
          },
        }),
        col.display({
          id: 'load',
          header: t('nodes.columns.load'),
          meta: { hideOnMobile: true },
          cell: (c) => (
            <LoadBars
              load={c.row.original.status?.load as Record<string, unknown> | null | undefined}
              fmt={fmt}
            />
          ),
        }),
      ]),
    [t, fmt, groupName],
  )

  const onAction = (n: Node, key: Key) => {
    switch (key) {
      case 'edit':
        setPanel({ kind: 'sheet', node: n })
        break
      case 'copy':
        copy.mutate(n.id)
        break
      case 'stats':
        setPanel({ kind: 'stats', node: n })
        break
      case 'online':
        setPanel({ kind: 'online', node: n })
        break
      case 'delete':
        askDelete(n)
        break
    }
  }

  const typeOptions = [
    { value: ALL, label: t('nodes.filters.allTypes') },
    ...NODE_TYPES.map((nt) => ({ value: nt, label: t(`nodes.types.${nt}`) })),
  ]
  const groupOptions = [
    { value: ALL, label: t('nodes.filters.allGroups') },
    ...(groups.data ?? []).map((g) => ({ value: String(g.id), label: g.name })),
  ]
  const statusOptions = [
    { value: ALL, label: t('nodes.filters.allStatus') },
    ...(['online', 'idle', 'offline'] as const).map((s) => ({ value: s, label: t(`nodes.status.${s}`) })),
  ]

  return (
    <PageShell
      title={t('nodes.title')}
      help={t('nodes.help')}
      actions={
        <Button onPress={() => setPanel({ kind: 'sheet', node: null })}>
          <Plus className="size-4" aria-hidden="true" />
          {t('nodes.create')}
        </Button>
      }
    >
      <div className="surface-card flex flex-col gap-3 p-4 sm:flex-row sm:flex-wrap sm:items-end">
        <SearchField
          value={query}
          onChange={setQuery}
          onSubmit={(v) => setSearch({ q: v.trim() || undefined })}
          onClear={() => setSearch({ q: undefined })}
          className="w-full sm:w-64"
        >
          <Label>{t('nodes.filters.search')}</Label>
          <SearchField.Group>
            <SearchField.SearchIcon />
            <SearchField.Input
              placeholder={t('nodes.filters.searchPlaceholder')}
              onBlur={() => setSearch({ q: query.trim() || undefined })}
            />
            <SearchField.ClearButton />
          </SearchField.Group>
        </SearchField>
        <FilterSelect
          label={t('nodes.filters.type')}
          value={search.type}
          options={typeOptions}
          onChange={(v) => setSearch({ type: v as NodeSearch['type'] })}
        />
        <FilterSelect
          label={t('nodes.filters.group')}
          value={search.group ? String(search.group) : undefined}
          options={groupOptions}
          onChange={(v) => setSearch({ group: v ? Number(v) : undefined })}
        />
        <FilterSelect
          label={t('nodes.filters.status')}
          value={search.status}
          options={statusOptions}
          onChange={(v) => setSearch({ status: v as NodeSearch['status'] })}
        />
        {filtered ? (
          <Button
            variant="tertiary"
            onPress={() => {
              setQuery('')
              void navigate({ search: {}, replace: true })
            }}
          >
            {t('nodes.filters.reset')}
          </Button>
        ) : null}
      </div>

      <DataTable
        aria-label={t('nodes.title')}
        columns={columns}
        data={nodes.data ? rows : undefined}
        isLoading={nodes.isPending}
        isFetching={nodes.isFetching && !nodes.isPending}
        getRowId={(n) => String(n.id)}
        emptyState={
          <EmptyState compact icon={<Server />} title={filtered ? t('nodes.noMatch') : t('nodes.empty')} />
        }
        dragAndDropHooks={dragAndDropHooks}
        rowActions={(n) => (
          <>
            <VisibilityToggle
              name={n.name}
              shown={n.show}
              onChange={(show) => toggleShowMutate({ id: n.id, show })}
            />
            <Dropdown>
              <Button size="sm" variant="ghost" isIconOnly aria-label={t('nodes.moreFor', { name: n.name })}>
                <Ellipsis className="size-4" aria-hidden="true" />
              </Button>
              <Dropdown.Popover placement="bottom end">
                <Dropdown.Menu onAction={(k) => onAction(n, k)}>
                  <Dropdown.Item id="edit" textValue={t('common:actions.edit')}>
                    <Pencil className="size-4" aria-hidden="true" />
                    <Label>{t('common:actions.edit')}</Label>
                  </Dropdown.Item>
                  <Dropdown.Item id="stats" textValue={t('nodes.actions.stats')}>
                    <BarChart3 className="size-4" aria-hidden="true" />
                    <Label>{t('nodes.actions.stats')}</Label>
                  </Dropdown.Item>
                  <Dropdown.Item id="online" textValue={t('nodes.actions.online')}>
                    <Users className="size-4" aria-hidden="true" />
                    <Label>{t('nodes.actions.online')}</Label>
                  </Dropdown.Item>
                  <Dropdown.Item id="copy" textValue={t('nodes.actions.copy')}>
                    <Copy className="size-4" aria-hidden="true" />
                    <Label>{t('nodes.actions.copy')}</Label>
                  </Dropdown.Item>
                  <Dropdown.Item id="delete" textValue={t('common:actions.delete')} variant="danger">
                    <Trash2 className="size-4" aria-hidden="true" />
                    <Label>{t('common:actions.delete')}</Label>
                  </Dropdown.Item>
                </Dropdown.Menu>
              </Dropdown.Popover>
            </Dropdown>
          </>
        )}
      />

      {panel?.kind === 'sheet' ? (
        <NodeSheet
          key={panel.node?.id ?? 'new'}
          node={panel.node}
          nodes={all}
          isOpen
          onOpenChange={(open) => !open && setPanel(null)}
        />
      ) : null}
      {panel?.kind === 'stats' ? <NodeStatsDrawer node={panel.node} onClose={() => setPanel(null)} /> : null}
      {panel?.kind === 'online' ? (
        <NodeOnlineDrawer node={panel.node} onClose={() => setPanel(null)} />
      ) : null}
    </PageShell>
  )
}
