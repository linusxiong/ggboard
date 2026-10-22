/** Joins truthy class names (tiny clsx). Use HeroUI's `cn` (tailwind-merge) only when you need conflict resolution. */
export function cx(...parts: Array<string | false | null | undefined | 0>): string {
  let out = ''
  for (const part of parts) if (part) out = out ? `${out} ${part}` : part
  return out
}
