/**
 * Backend snippets for a node (docs/NODE_PROTOCOLS.md §1.1). Only placeholders for secrets:
 * the shared token lives in Settings → server (`server_token`); a node with its own token (or a
 * child whose parent has one) shows `<node_token>` instead, since the shared one no longer works there.
 */
import type { NodeType } from '@/lib/api/types'

export interface IntegrationInput {
  type: NodeType
  /** `null` before the node is saved. */
  nodeId: number | null
  /** Hysteria version (1 | 2). */
  version?: unknown
  appUrl: string
  /** The node (for a child: its parent) has its own token instead of the shared `server_token`. */
  nodeToken?: boolean
}

export const SERVER_TOKEN_PLACEHOLDER = '<server_token>'
export const NODE_TOKEN_PLACEHOLDER = '<node_token>'

/** XrayR `NodeType` (NewV2board panel). XrayR serves vmess, vless, trojan and shadowsocks only. */
export function xrayrNodeType(type: NodeType): string | null {
  switch (type) {
    case 'vmess':
      return 'V2ray'
    case 'vless':
      return 'Vless'
    case 'trojan':
      return 'Trojan'
    case 'shadowsocks':
      return 'Shadowsocks'
    default:
      return null
  }
}

/** V2bX `NodeType`; hysteria 2 is `hysteria2`. No shadowsocksr / mieru. */
export function v2bxNodeType(type: NodeType, version?: unknown): string | null {
  switch (type) {
    case 'vmess':
    case 'vless':
    case 'trojan':
    case 'shadowsocks':
    case 'tuic':
    case 'anytls':
      return type
    case 'hysteria':
      return version === 1 ? 'hysteria' : 'hysteria2'
    default:
      return null
  }
}

/** soga `server_type`; soga runs hysteria 2 only and has no tuic. */
export function sogaServerType(type: NodeType, version?: unknown): string | null {
  switch (type) {
    case 'vmess':
      return 'v2ray'
    case 'shadowsocks':
      return 'ss'
    case 'shadowsocksr':
      return 'ssr'
    case 'hysteria':
      return version === 1 ? null : 'hysteria'
    case 'tuic':
      return null
    default:
      return type
  }
}

export function integrationSnippets({ type, nodeId, version, appUrl, nodeToken }: IntegrationInput) {
  const token = nodeToken ? NODE_TOKEN_PLACEHOLDER : SERVER_TOKEN_PLACEHOLDER
  const id = nodeId === null ? '<node_id>' : String(nodeId)
  const xrayr = xrayrNodeType(type)
  const v2bx = v2bxNodeType(type, version)
  const soga = sogaServerType(type, version)
  const core = type === 'hysteria' || type === 'tuic' || type === 'anytls' ? 'sing' : 'xray'
  return {
    xrayr: xrayr
      ? [
          'PanelType: "NewV2board"',
          'ApiConfig:',
          `  ApiHost: "${appUrl}"`,
          `  ApiKey: "${token}"`,
          `  NodeID: ${id}`,
          `  NodeType: ${xrayr}`,
          ...(type === 'vless' ? ['  EnableVless: true'] : []),
        ].join('\n')
      : null,
    v2bx: v2bx
      ? JSON.stringify(
          {
            Core: core,
            ApiHost: appUrl,
            ApiKey: token,
            NodeID: nodeId ?? '<node_id>',
            NodeType: v2bx,
            Timeout: 30,
          },
          null,
          2,
        )
      : null,
    soga: soga
      ? [
          'type=soga-v1',
          `server_type=${soga}`,
          `node_id=${id}`,
          'soga_key=<soga license>',
          'api=webapi',
          `webapi_url=${appUrl}/soga/`,
          `webapi_key=${token}`,
        ].join('\n')
      : null,
  }
}
