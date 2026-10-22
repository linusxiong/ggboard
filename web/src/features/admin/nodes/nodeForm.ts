/**
 * Node form model ↔ API `Node` / `NodeInput`. Common fields are TanStack Form fields; the
 * `protocol_settings` object is edited by `ProtocolForm` (see protocol.ts).
 */
import type { Node, NodeInput, NodeType } from '@/lib/api/types'
import { joinList, parseList } from '../shared/units'
import { defaultSettings, finalizeSettings, type Settings } from './protocol'

export const NO_PARENT = '0'

export interface NodeFormValues {
  type: NodeType
  name: string
  group_ids: string[]
  route_ids: string[]
  /** `'0'` = no parent (collection keys cannot be empty). */
  parent_id: string
  host: string
  /** Client port: `443` or a hopping range `20000-30000`. */
  port: string
  server_port: number | null
  /** Comma separated. */
  tags: string
  rate: number | null
  show: boolean
  sort: number | null
  protocol_settings: Settings
}

export function nodeToValues(node: Node | null | undefined, type: NodeType = 'vless'): NodeFormValues {
  return {
    type: node?.type ?? type,
    name: node?.name ?? '',
    group_ids: (node?.group_ids ?? []).map(String),
    route_ids: (node?.route_ids ?? []).map(String),
    parent_id: node?.parent_id ? String(node.parent_id) : NO_PARENT,
    host: node?.host ?? '',
    port: node?.port ?? '',
    server_port: node?.server_port ?? null,
    tags: joinList(node?.tags),
    rate: node?.rate ?? 1,
    show: node?.show ?? true,
    sort: node?.sort ?? null,
    protocol_settings: node ? { ...(node.protocol_settings as Settings) } : defaultSettings(type),
  }
}

export function valuesToNodeInput(values: NodeFormValues): NodeInput {
  const input: NodeInput = {
    type: values.type,
    name: values.name.trim(),
    group_ids: values.group_ids.map(Number),
    route_ids: values.route_ids.map(Number),
    parent_id: values.parent_id && values.parent_id !== NO_PARENT ? Number(values.parent_id) : null,
    host: values.host.trim(),
    port: values.port.trim(),
    server_port: values.server_port ?? 0,
    tags: parseList(values.tags),
    rate: values.rate ?? 1,
    show: values.show,
    protocol_settings: finalizeSettings(values.type, values.protocol_settings),
  }
  if (values.sort !== null) input.sort = Math.trunc(values.sort)
  return input
}

/** `443`, `20000-30000` (1–65535, ascending range). */
export function portError(port: string): 'required' | 'port' | undefined {
  const value = port.trim()
  if (!value) return 'required'
  const match = /^(\d{1,5})(?:-(\d{1,5}))?$/.exec(value)
  if (!match) return 'port'
  const from = Number(match[1])
  const to = match[2] === undefined ? from : Number(match[2])
  if (from < 1 || to > 65535 || to < from) return 'port'
  return undefined
}

export const PROTOCOL_PREFIX = 'protocol_settings.'

/** Splits a server `errors` map into protocol-settings paths (prefix removed) and the rest. */
export function splitNodeErrors(errors: Record<string, string[]>) {
  const protocol: Record<string, string> = {}
  const rest: Record<string, string[]> = {}
  for (const [key, messages] of Object.entries(errors)) {
    const message = messages[0]
    if (!message) continue
    if (key === 'protocol_settings') protocol[''] = message
    else if (key.startsWith(PROTOCOL_PREFIX)) protocol[key.slice(PROTOCOL_PREFIX.length)] = message
    else rest[key.split('.')[0] ?? key] = messages
  }
  return { protocol, rest }
}
