/**
 * Fetch wrapper for the GG Bond JSON API (docs/ARCHITECTURE.md §HTTP conventions).
 *
 * - Base path `/api/v1`, JSON in/out, `Authorization: Bearer <JWT>` from the auth store.
 * - Unwraps `{data}` → `T` and `{data, total}` → `Page<T>`.
 * - Errors become `ApiError` (`status`, `code`, `message`, `errors` field map).
 * - A 401 on an authenticated request clears the session and calls the registered
 *   unauthorized handler (the router redirects to `/login?redirect=…`).
 * - Every request has a timeout (default 15s) and honours an optional `AbortSignal`.
 */
import { useAuthStore } from '@/stores/auth'
import { useUiStore } from '@/stores/ui'
import type { ErrorBody, ErrorCode, Page, QueryParams } from './types'

export const API_BASE = '/api/v1'
export const DEFAULT_TIMEOUT = 15_000

export class ApiError extends Error {
  override readonly name = 'ApiError'
  readonly status: number
  readonly code: ErrorCode | (string & {})
  readonly errors: Record<string, string[]>
  /** Seconds from the `Retry-After` header (429 lockouts), `null` when absent. */
  readonly retryAfter: number | null

  constructor(status: number, body: ErrorBody, retryAfter: number | null = null) {
    super(body.message || body.code)
    this.status = status
    this.code = body.code
    this.errors = body.errors ?? {}
    this.retryAfter = retryAfter
  }

  /** First message for a field (`validation_failed`), if any. */
  fieldError(field: string): string | undefined {
    return this.errors[field]?.[0]
  }

  get isValidation() {
    return this.code === 'validation_failed'
  }
  get isUnauthorized() {
    return this.status === 401
  }
  get isNetwork() {
    return this.code === 'network' || this.code === 'timeout'
  }
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError
}

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE'

export interface RequestOptions {
  query?: QueryParams | object
  body?: unknown
  signal?: AbortSignal
  /** ms; `0` disables the timeout. */
  timeout?: number
  /** Skip the bearer token (public endpoints still work with it, this is for tests/special cases). */
  anonymous?: boolean
  headers?: Record<string, string>
  /**
   * FE-1: a 401 from this endpoint is a business error (e.g. a failed WebAuthn verification), not an
   * expired session — keep the session instead of logging out.
   */
  keepSession?: boolean
}

type UnauthorizedHandler = () => void
let unauthorizedHandler: UnauthorizedHandler | null = null

/** Registered once by the router (see `src/router.tsx`). */
export function setUnauthorizedHandler(handler: UnauthorizedHandler | null) {
  unauthorizedHandler = handler
}

// ---- FE-1 ---- admin/staff routes answer 403 `mfa_setup_required` until the account enables 2FA.
let mfaSetupRequiredHandler: (() => void) | null = null
/** Registered once by the router: sends the user to `/profile?tab=security`. */
export function setMfaSetupRequiredHandler(handler: (() => void) | null) {
  mfaSetupRequiredHandler = handler
}

export function buildQuery(query?: QueryParams | object): string {
  if (!query) return ''
  const params = new URLSearchParams()
  for (const [key, raw] of Object.entries(query as Record<string, unknown>)) {
    if (raw === undefined || raw === null || raw === '') continue
    const value = Array.isArray(raw) ? raw.join(',') : typeof raw === 'string' ? raw : JSON.stringify(raw)
    if (value === '') continue
    params.append(key, value)
  }
  const qs = params.toString()
  return qs ? `?${qs}` : ''
}

export function apiUrl(path: string, query?: QueryParams | object): string {
  return `${API_BASE}${path.startsWith('/') ? path : `/${path}`}${buildQuery(query)}`
}

function withTimeout(signal: AbortSignal | undefined, timeout: number): AbortSignal | undefined {
  if (!timeout) return signal
  const timer = AbortSignal.timeout(timeout)
  if (!signal) return timer
  if (typeof AbortSignal.any === 'function') return AbortSignal.any([signal, timer])
  const controller = new AbortController()
  const forward = (source: AbortSignal) => () => controller.abort(source.reason)
  signal.addEventListener('abort', forward(signal), { once: true })
  timer.addEventListener('abort', forward(timer), { once: true })
  return controller.signal
}

async function send(method: HttpMethod, path: string, opts: RequestOptions = {}): Promise<Response> {
  const token = opts.anonymous ? null : useAuthStore.getState().token
  const headers: Record<string, string> = {
    Accept: 'application/json',
    'Accept-Language': useUiStore.getState().locale,
    ...opts.headers,
  }
  let body: BodyInit | undefined
  if (opts.body instanceof FormData || opts.body instanceof Blob || typeof opts.body === 'string') {
    body = opts.body
  } else if (opts.body !== undefined) {
    headers['Content-Type'] = 'application/json'
    body = JSON.stringify(opts.body)
  }
  if (token) headers.Authorization = `Bearer ${token}`

  let res: Response
  try {
    res = await fetch(apiUrl(path, opts.query), {
      method,
      headers,
      body,
      signal: withTimeout(opts.signal, opts.timeout ?? DEFAULT_TIMEOUT),
      credentials: 'same-origin',
    })
  } catch (err) {
    // DOMException is not `instanceof Error` in every runtime; read `name` structurally.
    const name = typeof err === 'object' && err !== null && 'name' in err ? String(err.name) : ''
    if (name === 'TimeoutError') throw new ApiError(0, { code: 'timeout', message: 'Request timed out' })
    if (name === 'AbortError') throw new ApiError(0, { code: 'aborted', message: 'Request aborted' })
    throw new ApiError(0, { code: 'network', message: err instanceof Error ? err.message : 'Network error' })
  }

  if (!res.ok) {
    const error = await toApiError(res)
    if (res.status === 401 && token && !opts.keepSession) {
      useAuthStore.getState().logout()
      unauthorizedHandler?.()
    }
    if (res.status === 403 && error.code === 'mfa_setup_required') mfaSetupRequiredHandler?.()
    throw error
  }
  return res
}

async function toApiError(res: Response): Promise<ApiError> {
  const fallback: ErrorBody = {
    code: statusToCode(res.status),
    message: res.statusText || `HTTP ${res.status}`,
  }
  const retryAfter = parseRetryAfter(res.headers.get('Retry-After'))
  try {
    const text = await res.text()
    if (!text) return new ApiError(res.status, fallback, retryAfter)
    const parsed = JSON.parse(text) as Partial<ErrorBody>
    return new ApiError(
      res.status,
      {
        code: parsed.code ?? fallback.code,
        message: parsed.message ?? fallback.message,
        ...(parsed.errors ? { errors: parsed.errors } : {}),
      },
      retryAfter,
    )
  } catch {
    return new ApiError(res.status, fallback, retryAfter)
  }
}

/** `Retry-After`: delay-seconds or an HTTP date → whole seconds (≥ 0), `null` when absent/invalid. */
export function parseRetryAfter(value: string | null, now = Date.now()): number | null {
  if (!value) return null
  const trimmed = value.trim()
  if (/^\d+$/.test(trimmed)) return Number(trimmed)
  const date = Date.parse(trimmed)
  return Number.isNaN(date) ? null : Math.max(0, Math.ceil((date - now) / 1000))
}

function statusToCode(status: number): ErrorCode {
  switch (status) {
    case 400:
    case 422:
      return 'validation_failed'
    case 401:
      return 'unauthorized'
    case 402:
      return 'payment_failed'
    case 403:
      return 'forbidden'
    case 404:
      return 'not_found'
    case 409:
      return 'conflict'
    case 429:
      return 'rate_limited'
    default:
      return 'internal'
  }
}

async function readJson(res: Response): Promise<Record<string, unknown> | undefined> {
  if (res.status === 204) return undefined
  const text = await res.text()
  if (!text) return undefined
  try {
    return JSON.parse(text) as Record<string, unknown>
  } catch {
    throw new ApiError(res.status, { code: 'bad_response', message: 'Malformed JSON response' })
  }
}

/** `{data: T}` → `T`. */
export async function request<T>(method: HttpMethod, path: string, opts?: RequestOptions): Promise<T> {
  const json = await readJson(await send(method, path, opts))
  if (json === undefined) return undefined as T
  if (!('data' in json)) throw new ApiError(200, { code: 'bad_response', message: 'Missing "data" envelope' })
  return json.data as T
}

/** `{data: T[], total}` → `Page<T>` (also accepts a bare `{data: T[]}`). */
export async function requestPage<T>(path: string, opts?: RequestOptions): Promise<Page<T>> {
  const json = await readJson(await send('GET', path, opts))
  const data = json?.data
  if (!Array.isArray(data)) throw new ApiError(200, { code: 'bad_response', message: 'Expected a list' })
  const total = typeof json?.total === 'number' ? json.total : data.length
  return { items: data as T[], total }
}

/** Raw text body (e.g. `text/plain` subscription preview, `text/csv` exports). */
export async function requestText(method: HttpMethod, path: string, opts?: RequestOptions): Promise<string> {
  const res = await send(method, path, {
    ...opts,
    headers: { Accept: 'text/plain, text/csv, */*', ...opts?.headers },
  })
  return res.text()
}

/** Raw blob body (downloads). */
export async function requestBlob(method: HttpMethod, path: string, opts?: RequestOptions): Promise<Blob> {
  const res = await send(method, path, { ...opts, headers: { Accept: '*/*', ...opts?.headers } })
  return res.blob()
}

/** Shorthand used by the endpoint modules. */
export const api = {
  get: <T>(path: string, query?: RequestOptions['query'], opts?: RequestOptions) =>
    request<T>('GET', path, { ...opts, query }),
  list: <T>(path: string, query?: RequestOptions['query'], opts?: RequestOptions) =>
    requestPage<T>(path, { ...opts, query }),
  post: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    request<T>('POST', path, { ...opts, body }),
  put: <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('PUT', path, { ...opts, body }),
  patch: <T>(path: string, body?: unknown, opts?: RequestOptions) =>
    request<T>('PATCH', path, { ...opts, body }),
  delete: <T = true>(path: string, opts?: RequestOptions) => request<T>('DELETE', path, opts),
}
