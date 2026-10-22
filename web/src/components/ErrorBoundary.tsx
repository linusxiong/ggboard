import { Component, type ErrorInfo, type ReactNode } from 'react'

interface Props {
  children: ReactNode
  /** Rendered when a descendant throws. */
  fallback?: (error: Error, reset: () => void) => ReactNode
}
interface State {
  error: Error | null
}

/**
 * Last-resort boundary around the whole app (route errors are handled by the router's
 * `errorComponent`). Plain text on purpose: dictionaries may be what failed.
 */
export class ErrorBoundary extends Component<Props, State> {
  override state: State = { error: null }

  static getDerivedStateFromError(error: Error): State {
    return { error }
  }

  override componentDidCatch(error: Error, info: ErrorInfo) {
    if (import.meta.env.DEV) console.error(error, info.componentStack)
  }

  reset = () => this.setState({ error: null })

  override render() {
    const { error } = this.state
    if (!error) return this.props.children
    if (this.props.fallback) return this.props.fallback(error, this.reset)
    return (
      <main className="flex min-h-dvh flex-col items-center justify-center gap-3 px-6 text-center">
        <h1 className="text-title2 text-label">出错了 · Something went wrong</h1>
        <p className="max-w-md text-subhead text-label-secondary">{error.message}</p>
        <button type="button" className="button button--primary button--md" onClick={() => window.location.reload()}>
          重新加载 · Reload
        </button>
      </main>
    )
  }
}
