/**
 * `vite preview` that behaves like the Go static server (internal/static):
 *  - serves precompressed `.br` / `.gz` assets with `immutable` caching for hashed files;
 *  - `index.html` is `no-cache`;
 *  - in `--mode lighthouse` it also answers `/api/v1/guest/*`, the passport endpoints and
 *    `/user/me` from fixtures (demo login: any email/password) so Lighthouse and manual checks
 *    can exercise real pages without the Go backend.
 *
 * Never part of the production bundle — it only hooks `configurePreviewServer`.
 */
import { existsSync, readFileSync, statSync } from 'node:fs'
import path from 'node:path'
import type { IncomingMessage, ServerResponse } from 'node:http'
import type { Plugin } from 'vite'

const MIME: Record<string, string> = {
  '.js': 'text/javascript; charset=utf-8',
  '.mjs': 'text/javascript; charset=utf-8',
  '.css': 'text/css; charset=utf-8',
  '.svg': 'image/svg+xml',
  '.json': 'application/json; charset=utf-8',
  '.webmanifest': 'application/manifest+json; charset=utf-8',
  '.txt': 'text/plain; charset=utf-8',
  '.html': 'text/html; charset=utf-8',
}

const fixturesDir = path.resolve(import.meta.dirname, 'fixtures')
/** Unsigned demo JWT (exp 2100-01-01) — mock mode only, never accepted by the real backend. */
const MOCK_TOKEN = `eyJhbGciOiJub25lIn0.${Buffer.from(JSON.stringify({ sub: 1, exp: 4102444800 })).toString('base64url')}.mock`

function sendJson(res: ServerResponse, status: number, body: unknown) {
  res.statusCode = status
  res.setHeader('Content-Type', 'application/json; charset=utf-8')
  res.setHeader('Cache-Control', 'no-store')
  res.end(JSON.stringify(body))
}

function mockApi(req: IncomingMessage, res: ServerResponse): boolean {
  const url = new URL(req.url ?? '/', 'http://localhost')
  if (!url.pathname.startsWith('/api/v1/')) return false
  const fixture = (name: string) => JSON.parse(readFileSync(path.join(fixturesDir, name), 'utf8')) as unknown
  switch (url.pathname) {
    case '/api/v1/guest/config':
      sendJson(res, 200, { data: fixture('guest-config.json') })
      return true
    case '/api/v1/guest/plans':
      sendJson(res, 200, { data: fixture('guest-plans.json') })
      return true
    case '/api/v1/passport/login':
    case '/api/v1/passport/register':
      sendJson(res, 200, { data: { token: MOCK_TOKEN, user: fixture('user.json') } })
      return true
    case '/api/v1/user/logout':
    case '/api/v1/passport/email/send-code':
    case '/api/v1/passport/forgot':
      sendJson(res, 200, { data: true })
      return true
    case '/api/v1/user/me':
      if (req.headers.authorization === `Bearer ${MOCK_TOKEN}`)
        sendJson(res, 200, { data: fixture('user.json') })
      else sendJson(res, 401, { code: 'unauthorized', message: 'mock backend: not signed in' })
      return true
    default:
      sendJson(res, 401, { code: 'unauthorized', message: 'mock backend: not signed in' })
      return true
  }
}

function serveAsset(distDir: string, req: IncomingMessage, res: ServerResponse): boolean {
  const pathname = decodeURIComponent(new URL(req.url ?? '/', 'http://localhost').pathname)
  if (!pathname.startsWith('/assets/')) return false
  const file = path.join(distDir, pathname)
  if (!file.startsWith(distDir) || !existsSync(file) || !statSync(file).isFile()) return false
  const accept = String(req.headers['accept-encoding'] ?? '')
  const ext = path.extname(file)
  let body = file
  let encoding: string | undefined
  if (/\bbr\b/.test(accept) && existsSync(`${file}.br`)) {
    body = `${file}.br`
    encoding = 'br'
  } else if (/\bgzip\b/.test(accept) && existsSync(`${file}.gz`)) {
    body = `${file}.gz`
    encoding = 'gzip'
  }
  res.statusCode = 200
  res.setHeader('Content-Type', MIME[ext] ?? 'application/octet-stream')
  res.setHeader('Cache-Control', 'public, max-age=31536000, immutable')
  res.setHeader('Vary', 'Accept-Encoding')
  if (encoding) res.setHeader('Content-Encoding', encoding)
  res.end(readFileSync(body))
  return true
}

export function productionPreview(): Plugin {
  return {
    name: 'ggboard:production-preview',
    configurePreviewServer(server) {
      const distDir = path.resolve(server.config.root, server.config.build.outDir)
      const mock = server.config.mode === 'lighthouse'
      server.middlewares.use((req, res, next) => {
        if (mock && mockApi(req, res)) return
        if (serveAsset(distDir, req, res)) return
        if (req.url && !req.url.startsWith('/assets/') && !path.extname(req.url.split('?')[0] ?? '')) {
          res.setHeader('Cache-Control', 'no-cache')
        }
        next()
      })
    },
  }
}
