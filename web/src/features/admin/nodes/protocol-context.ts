import { createContext, useContext } from 'react'
import type { TFunction } from '@/i18n'
import type { NodeType } from '@/lib/api/types'
import { getPath, type Settings } from './protocol'

export interface ProtocolContextValue {
  type: NodeType
  value: Settings
  set: (path: string, value: unknown) => void
  /** Messages keyed by path inside `protocol_settings`. */
  errors: Record<string, string>
  t: TFunction
}

export const ProtocolContext = createContext<ProtocolContextValue | null>(null)

/** Binds a protocol-settings path: current value, setter, error and a stable DOM id. */
export function useSetting(path: string) {
  const ctx = useContext(ProtocolContext)
  if (!ctx) throw new Error('useSetting must be used inside <ProtocolContext>')
  return {
    value: getPath(ctx.value, path),
    set: (value: unknown) => ctx.set(path, value),
    error: ctx.errors[path],
    id: `ps-${path.replace(/\./g, '-')}`,
    t: ctx.t,
    settings: ctx.value,
    type: ctx.type,
  }
}
