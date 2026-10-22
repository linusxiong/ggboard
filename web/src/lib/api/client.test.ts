import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { useAuthStore } from '@/stores/auth'
import { ApiError, api, buildQuery, isApiError, request, requestPage, setUnauthorizedHandler } from './client'

const user = { id: 1, email: 'a@example.com' } as never

function jsonResponse(status: number, body: unknown) {
  return new Response(JSON.stringify(body), { status, headers: { 'Content-Type': 'application/json' } })
}

describe('api client', () => {
  const fetchMock = vi.fn<typeof fetch>()

  beforeEach(() => {
    fetchMock.mockReset()
    vi.stubGlobal('fetch', fetchMock)
    useAuthStore.setState({ token: null, user: null })
  })
  afterEach(() => {
    vi.unstubAllGlobals()
    setUnauthorizedHandler(null)
  })

  it('unwraps {data} and sends JSON + bearer token', async () => {
    useAuthStore.setState({ token: 'jwt-123', user })
    fetchMock.mockResolvedValueOnce(jsonResponse(200, { data: { ok: 1 } }))
    const result = await api.post<{ ok: number }>('/user/password', { old_password: 'x', new_password: 'y' })
    expect(result).toEqual({ ok: 1 })
    const [url, init] = fetchMock.mock.calls[0]!
    expect(url).toBe('/api/v1/user/password')
    const headers = init?.headers as Record<string, string>
    expect(init?.method).toBe('POST')
    expect(headers.Authorization).toBe('Bearer jwt-123')
    expect(headers['Content-Type']).toBe('application/json')
    expect(init?.body).toBe(JSON.stringify({ old_password: 'x', new_password: 'y' }))
  })

  it('serialises query params and skips empty values', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, { data: [] }))
    await api.get('/admin/users', { page: 2, email: '', banned: false, ids: [1, 2], x: undefined })
    expect(fetchMock.mock.calls[0]![0]).toBe('/api/v1/admin/users?page=2&banned=false&ids=1%2C2')
    expect(buildQuery({})).toBe('')
  })

  it('normalises list envelopes to Page<T>', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, { data: [{ id: 1 }], total: 42 }))
    await expect(requestPage('/user/orders')).resolves.toEqual({ items: [{ id: 1 }], total: 42 })
    fetchMock.mockResolvedValueOnce(jsonResponse(200, { data: [{ id: 1 }, { id: 2 }] }))
    await expect(api.list('/guest/plans')).resolves.toEqual({ items: [{ id: 1 }, { id: 2 }], total: 2 })
  })

  it('maps error bodies to ApiError with field errors', async () => {
    fetchMock.mockResolvedValueOnce(
      jsonResponse(400, {
        code: 'validation_failed',
        message: 'bad input',
        errors: { email: ['invalid email'] },
      }),
    )
    const error = await request('POST', '/passport/login', { body: {} }).catch((e: unknown) => e)
    expect(isApiError(error)).toBe(true)
    const apiError = error as ApiError
    expect(apiError.status).toBe(400)
    expect(apiError.code).toBe('validation_failed')
    expect(apiError.message).toBe('bad input')
    expect(apiError.fieldError('email')).toBe('invalid email')
    expect(apiError.isValidation).toBe(true)
  })

  it('falls back to a status-derived code for non-JSON errors', async () => {
    fetchMock.mockResolvedValueOnce(new Response('upstream down', { status: 502, statusText: 'Bad Gateway' }))
    const error = (await api.get('/guest/config').catch((e: unknown) => e)) as ApiError
    expect(error.code).toBe('internal')
    expect(error.status).toBe(502)
  })

  it('clears the session and calls the unauthorized handler on 401 with a token', async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    useAuthStore.setState({ token: 'expired', user })
    fetchMock.mockResolvedValueOnce(jsonResponse(401, { code: 'unauthorized', message: 'token expired' }))
    await expect(api.get('/user/me')).rejects.toBeInstanceOf(ApiError)
    expect(useAuthStore.getState().token).toBeNull()
    expect(handler).toHaveBeenCalledOnce()
  })

  it('does not log out on 401 from anonymous requests (e.g. wrong password)', async () => {
    const handler = vi.fn()
    setUnauthorizedHandler(handler)
    useAuthStore.setState({ token: 'still-valid', user })
    fetchMock.mockResolvedValueOnce(jsonResponse(401, { code: 'unauthorized', message: 'wrong password' }))
    await expect(api.post('/passport/login', {}, { anonymous: true })).rejects.toMatchObject({ status: 401 })
    expect(useAuthStore.getState().token).toBe('still-valid')
    expect(handler).not.toHaveBeenCalled()
  })

  it('reports network failures and timeouts as ApiError', async () => {
    fetchMock.mockRejectedValueOnce(new TypeError('Failed to fetch'))
    await expect(api.get('/guest/config')).rejects.toMatchObject({ code: 'network', status: 0 })

    fetchMock.mockImplementationOnce(
      (_url, init) =>
        new Promise((_resolve, reject) => {
          init?.signal?.addEventListener('abort', () => reject(new DOMException('timed out', 'TimeoutError')))
        }),
    )
    await expect(api.get('/guest/config', undefined, { timeout: 10 })).rejects.toMatchObject({
      code: 'timeout',
    })
  })

  it('rejects responses without the data envelope', async () => {
    fetchMock.mockResolvedValueOnce(jsonResponse(200, { ok: true }))
    await expect(api.get('/guest/config')).rejects.toMatchObject({ code: 'bad_response' })
  })
})
