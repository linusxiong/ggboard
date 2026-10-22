/** Form-level error (e.g. "invalid credentials"), announced to screen readers. Dependency-free. */
export function FormError({ message, className }: { message?: string | null | undefined; className?: string }) {
  if (!message) return null
  return (
    <p role="alert" className={className ?? 'rounded-[10px] bg-danger-soft px-3 py-2 text-footnote text-label-red'}>
      {message}
    </p>
  )
}
