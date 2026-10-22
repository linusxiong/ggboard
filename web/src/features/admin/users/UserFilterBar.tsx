/** Filter bar for `/admin/users`: quick selects + e-mail search, advanced identifiers in a disclosure. */
import { Button, Input, Label, ListBox, SearchField, Select, TextField } from '@heroui/react'
import { useQuery } from '@tanstack/react-query'
import { type FormEvent, type Key, useState } from 'react'
import { SegmentedControl } from '@/components/SegmentedControl'
import { useT } from '@/i18n'
import { adminGroupsQuery, adminPlansQuery } from '@/lib/api/queries'
import { activeFilterCount, type UserSearch } from './filters'

const ALL = '__all__'

function QuickSelect({
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
      className="w-full sm:w-36"
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

type Advanced = Pick<UserSearch, 'id' | 'uuid' | 'token' | 'invite_user_id' | 'remarks'>

export function UserFilterBar({
  search,
  onChange,
  onReset,
}: {
  search: UserSearch
  onChange: (patch: Partial<UserSearch>) => void
  onReset: () => void
}) {
  const t = useT(['admin-users', 'common'])
  const plans = useQuery(adminPlansQuery())
  const groups = useQuery(adminGroupsQuery())
  const [email, setEmail] = useState(search.email ?? '')
  const advancedCount = [search.id, search.uuid, search.token, search.invite_user_id, search.remarks].filter(
    (v) => v !== undefined,
  ).length
  const [showAdvanced, setShowAdvanced] = useState(advancedCount > 0)
  const [draft, setDraft] = useState({
    id: search.id ? String(search.id) : '',
    uuid: search.uuid ?? '',
    token: search.token ?? '',
    invite_user_id: search.invite_user_id ? String(search.invite_user_id) : '',
    remarks: search.remarks ?? '',
  })

  const applyAdvanced = (event: FormEvent) => {
    event.preventDefault()
    const int = (v: string) => (/^\d+$/.test(v.trim()) && Number(v) > 0 ? Number(v) : undefined)
    const patch: Advanced = {
      id: int(draft.id),
      uuid: draft.uuid.trim() || undefined,
      token: draft.token.trim() || undefined,
      invite_user_id: int(draft.invite_user_id),
      remarks: draft.remarks.trim() || undefined,
    }
    onChange(patch)
  }

  const all = (label: string) => ({ value: ALL, label })
  const total = activeFilterCount(search)

  return (
    <div className="surface-card flex flex-col gap-4 p-4">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <SegmentedControl
          aria-label={t('filters.status')}
          value={search.banned === undefined ? ALL : search.banned ? 'banned' : 'normal'}
          onChange={(v) => onChange({ banned: v === ALL ? undefined : v === 'banned' })}
          items={[
            { id: ALL, label: t('filters.anyStatus') },
            { id: 'normal', label: t('filters.normal') },
            { id: 'banned', label: t('filters.banned') },
          ]}
        />
        <div className="flex flex-wrap items-center gap-2">
          <Button
            size="sm"
            variant="tertiary"
            aria-expanded={showAdvanced}
            onPress={() => setShowAdvanced((v) => !v)}
          >
            {advancedCount ? t('filters.advancedCount', { count: advancedCount }) : t('filters.advanced')}
          </Button>
          {total ? (
            <Button
              size="sm"
              variant="ghost"
              onPress={() => {
                setEmail('')
                setDraft({ id: '', uuid: '', token: '', invite_user_id: '', remarks: '' })
                onReset()
              }}
            >
              {t('filters.reset', { count: total })}
            </Button>
          ) : null}
        </div>
      </div>
      <div className="flex flex-col gap-3 sm:flex-row sm:flex-wrap sm:items-end">
        <SearchField
          value={email}
          onChange={setEmail}
          onSubmit={(v) => onChange({ email: v.trim() || undefined })}
          onClear={() => onChange({ email: undefined })}
          className="w-full sm:w-64"
        >
          <Label>{t('filters.email')}</Label>
          <SearchField.Group>
            <SearchField.SearchIcon />
            <SearchField.Input
              placeholder={t('filters.emailPlaceholder')}
              onBlur={() => onChange({ email: email.trim() || undefined })}
            />
            <SearchField.ClearButton />
          </SearchField.Group>
        </SearchField>
        <QuickSelect
          label={t('filters.plan')}
          value={search.plan_id ? String(search.plan_id) : undefined}
          options={[
            all(t('filters.allPlans')),
            ...(plans.data ?? []).map((p) => ({ value: String(p.id), label: p.name })),
          ]}
          onChange={(v) => onChange({ plan_id: v ? Number(v) : undefined })}
        />
        <QuickSelect
          label={t('filters.group')}
          value={search.group_id ? String(search.group_id) : undefined}
          options={[
            all(t('filters.allGroups')),
            ...(groups.data ?? []).map((g) => ({ value: String(g.id), label: g.name })),
          ]}
          onChange={(v) => onChange({ group_id: v ? Number(v) : undefined })}
        />
        <QuickSelect
          label={t('filters.role')}
          value={search.role}
          options={[
            all(t('filters.anyRole')),
            ...(['admin', 'staff', 'user'] as const).map((r) => ({ value: r, label: t(`roles.${r}`) })),
          ]}
          onChange={(v) => onChange({ role: v as UserSearch['role'] })}
        />
        <QuickSelect
          label={t('filters.expiry')}
          value={search.expired}
          options={[
            all(t('filters.anyExpiry')),
            ...(['active', 'expired', 'never'] as const).map((e) => ({ value: e, label: t(`expiry.${e}`) })),
          ]}
          onChange={(v) => onChange({ expired: v as UserSearch['expired'] })}
        />
      </div>
      {showAdvanced ? (
        <form onSubmit={applyAdvanced} className="grid gap-3 sm:grid-cols-2 lg:grid-cols-6 lg:items-end">
          {(
            [
              ['id', t('filters.id'), 'numeric'],
              ['invite_user_id', t('filters.inviter'), 'numeric'],
              ['uuid', 'UUID', 'text'],
              ['token', t('filters.token'), 'text'],
              ['remarks', t('filters.remarks'), 'text'],
            ] as const
          ).map(([key, label, mode]) => (
            <TextField
              key={key}
              value={draft[key]}
              onChange={(v) => setDraft((d) => ({ ...d, [key]: v }))}
              className={key === 'uuid' || key === 'token' ? 'lg:col-span-1' : undefined}
            >
              <Label>{label}</Label>
              <Input
                inputMode={mode}
                spellCheck={false}
                autoComplete="off"
                className={mode === 'text' && key !== 'remarks' ? 'font-mono text-footnote' : undefined}
              />
            </TextField>
          ))}
          <Button type="submit" variant="secondary">
            {t('filters.apply')}
          </Button>
        </form>
      ) : null}
    </div>
  )
}
