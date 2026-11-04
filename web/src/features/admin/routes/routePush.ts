import type { Node } from '@/lib/api/types'

/** Nodes a route can be pushed to: child nodes take their parent's routes. */
export const pushTargets = (nodes: readonly Node[]) => nodes.filter((n) => !n.parent_id)

/** Case-insensitive search over the name, host and tags; an empty query matches every node. */
export function nodeMatches(node: Node, query: string): boolean {
  const q = query.trim().toLowerCase()
  if (!q) return true
  return [node.name, node.host, ...node.tags].some((text) => text.toLowerCase().includes(q))
}
