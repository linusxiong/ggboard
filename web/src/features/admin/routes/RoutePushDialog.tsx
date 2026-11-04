/**
 * Pushes one route to every node or to a chosen set. The chips mirror the nodes' `route_ids`, so a node
 * that is unticked here loses the route.
 */
import { Button, Modal, SearchField } from '@heroui/react'
import { useMemo, useState } from 'react'
import { ModalClose } from '@/components/feedback/ModalClose'
import { ChipSelect } from '@/components/form/ChipSelect'
import { HelpTip } from '@/components/HelpTip'
import { SegmentedControl } from '@/components/SegmentedControl'
import { useT } from '@/i18n'
import * as adminApi from '@/lib/api/admin'
import { qk } from '@/lib/api/queries'
import type { Node, NodeRoute, RouteApplyInput } from '@/lib/api/types'
import { useAdminMutation } from '../shared/useAdminMutation'
import { nodeMatches, pushTargets } from './routePush'

type Scope = 'all' | 'some'

interface RoutePushDialogProps {
  route: NodeRoute
  /** All nodes, child nodes included. */
  nodes: readonly Node[]
  isOpen: boolean
  onOpenChange: (open: boolean) => void
}

export function RoutePushDialog({ route, nodes, isOpen, onOpenChange }: RoutePushDialogProps) {
  const t = useT(['admin-nodes', 'common'])
  const targets = useMemo(() => pushTargets(nodes), [nodes])
  const applied = useMemo(() => targets.filter((n) => n.route_ids.includes(route.id)), [targets, route.id])
  const [scope, setScope] = useState<Scope>(
    applied.length > 0 && applied.length < targets.length ? 'some' : 'all',
  )
  const [selected, setSelected] = useState(() => applied.map((n) => String(n.id)))
  const [query, setQuery] = useState('')

  const apply = useAdminMutation({
    mutationFn: (input: RouteApplyInput) => adminApi.applyRoute(route.id, input),
    success: (result) =>
      result.node_count ? t('routes.push.done', { count: result.node_count }) : t('routes.push.withdrawn'),
    invalidate: [qk.admin.nodes()],
    onSuccess: () => onOpenChange(false),
  })

  const options = targets
    .filter((n) => nodeMatches(n, query))
    .map((n) => ({ value: String(n.id), label: n.name }))
  const shownIds = options.map((o) => o.value)
  const chosen = targets.filter((n) => selected.includes(String(n.id)))
  const count = scope === 'all' ? targets.length : chosen.length
  const withdrawn = scope === 'all' ? 0 : applied.filter((n) => !selected.includes(String(n.id))).length
  const name = route.remarks || `#${route.id}`

  return (
    <Modal.Backdrop isOpen={isOpen} onOpenChange={onOpenChange}>
      <Modal.Container size="md" scroll="inside">
        <Modal.Dialog>
          <ModalClose />
          <Modal.Header>
            <span className="flex items-center gap-1">
              <Modal.Heading>{t('routes.push.title', { name })}</Modal.Heading>
              <HelpTip subject={t('routes.push.helpSubject')}>{t('routes.push.help')}</HelpTip>
            </span>
          </Modal.Header>
          <Modal.Body className="flex flex-col gap-4">
            <SegmentedControl
              aria-label={t('routes.push.scope')}
              items={[
                { id: 'all', label: t('routes.push.all') },
                { id: 'some', label: t('routes.push.some') },
              ]}
              value={scope}
              onChange={setScope}
            />
            {targets.length === 0 ? (
              <p className="text-subhead text-label-secondary">{t('routes.push.noNodes')}</p>
            ) : scope === 'all' ? (
              <p className="tabular text-subhead text-label-secondary">
                {t('routes.push.applied', { count: applied.length, total: targets.length })}
              </p>
            ) : (
              <>
                <div className="flex items-center gap-2">
                  <SearchField
                    aria-label={t('routes.push.search')}
                    value={query}
                    onChange={setQuery}
                    className="min-w-0 flex-1"
                  >
                    <SearchField.Group>
                      <SearchField.SearchIcon />
                      <SearchField.Input placeholder={t('routes.push.searchPlaceholder')} />
                      <SearchField.ClearButton />
                    </SearchField.Group>
                  </SearchField>
                  <Button
                    variant="tertiary"
                    isDisabled={shownIds.every((id) => selected.includes(id))}
                    onPress={() => setSelected((prev) => [...new Set([...prev, ...shownIds])])}
                  >
                    {t('routes.push.selectAll')}
                  </Button>
                  <Button
                    variant="tertiary"
                    isDisabled={!shownIds.some((id) => selected.includes(id))}
                    onPress={() => setSelected((prev) => prev.filter((id) => !shownIds.includes(id)))}
                  >
                    {t('routes.push.clear')}
                  </Button>
                </div>
                {options.length ? (
                  <ChipSelect
                    label={t('routes.push.nodes', { count: chosen.length, total: targets.length })}
                    options={options}
                    value={selected}
                    onChange={setSelected}
                  />
                ) : (
                  <p className="text-subhead text-label-secondary">{t('routes.push.noMatch')}</p>
                )}
                {count > 0 && withdrawn > 0 ? (
                  <p
                    role="status"
                    className="rounded-[10px] bg-warning-soft px-3 py-2 text-footnote text-label-orange-soft"
                  >
                    {t('routes.push.withdrawNote', { count: withdrawn })}
                  </p>
                ) : null}
              </>
            )}
          </Modal.Body>
          <Modal.Footer>
            <Button slot="close" variant="tertiary">
              {t('common:actions.cancel')}
            </Button>
            <Button
              variant={count === 0 && withdrawn > 0 ? 'danger' : 'primary'}
              isPending={apply.isPending}
              isDisabled={count === 0 && withdrawn === 0}
              onPress={() =>
                apply.mutate(scope === 'all' ? { all: true } : { node_ids: chosen.map((n) => n.id) })
              }
            >
              {count === 0 && withdrawn > 0
                ? t('routes.push.withdraw', { count: withdrawn })
                : t('routes.push.submit', { count })}
            </Button>
          </Modal.Footer>
        </Modal.Dialog>
      </Modal.Container>
    </Modal.Backdrop>
  )
}
