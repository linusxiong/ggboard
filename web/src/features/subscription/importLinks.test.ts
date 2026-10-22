import { describe, expect, it } from 'vitest'
import { base64Url, importLink, IMPORT_CLIENTS, withSubscribeQuery } from './importLinks'

const URL_ = 'https://panel.example.com/s/abc123?flag=x&y=1'

describe('import deep links', () => {
  it('builds the documented scheme for every client', () => {
    const enc = encodeURIComponent(URL_)
    expect(importLink('clash', URL_, 'GG Bond')).toBe(`clash://install-config?url=${enc}&name=GG%20Bond`)
    expect(importLink('clash', URL_)).toBe(`clash://install-config?url=${enc}`)
    expect(importLink('stash', URL_)).toBe(`stash://install-config?url=${enc}`)
    expect(importLink('singbox', URL_, 'GG Bond')).toBe(
      `sing-box://import-remote-profile?url=${enc}#GG%20Bond`,
    )
    expect(importLink('surge', URL_)).toBe(`surge:///install-config?url=${enc}`)
    const loon = new URL(decodeURIComponent(importLink('loon', URL_).slice('loon://import?sub='.length)))
    expect(loon.searchParams.get('flag')).toBe('loon')
    expect(loon.searchParams.get('list')).toBe('1')
    expect(loon.searchParams.get('y')).toBe('1')
  })

  it('Shadowrocket: base64url (no padding) of the URL + remark', () => {
    const link = importLink('shadowrocket', URL_, 'GG Bond')
    expect(link).toMatch(/^shadowrocket:\/\/add\/sub:\/\/[A-Za-z0-9_-]+\?remark=GG%20Bond$/)
    const encoded = link.slice('shadowrocket://add/sub://'.length, link.indexOf('?remark='))
    expect(encoded).not.toMatch(/[+/=]/)
    expect(atob(encoded.replace(/-/g, '+').replace(/_/g, '/'))).toBe(URL_)
  })

  it('Quantumult X: url-encoded JSON {server_remote:[url&flag=quantumultx&list=1, tag=name]}', () => {
    // [server_remote] takes proxy lines only: the backend renders them with list=1 (full config otherwise).
    const list = 'https://panel.example.com/s/abc123?flag=quantumultx&y=1&list=1'
    const link = importLink('quantumultx', URL_, 'GG Bond')
    const prefix = 'quantumult-x:///add-resource?remote-resource='
    expect(link.startsWith(prefix)).toBe(true)
    const json = JSON.parse(decodeURIComponent(link.slice(prefix.length))) as { server_remote: string[] }
    expect(json).toEqual({ server_remote: [`${list}, tag=GG Bond`] })
    const bare = JSON.parse(
      decodeURIComponent(importLink('quantumultx', URL_).slice(prefix.length)),
    ) as unknown
    expect(bare).toEqual({ server_remote: [list] })
    const plain = 'https://panel.example.com/api/v1/client/subscribe?token=t0k'
    expect(
      (
        JSON.parse(decodeURIComponent(importLink('quantumultx', plain).slice(prefix.length))) as {
          server_remote: string[]
        }
      ).server_remote[0],
    ).toBe(`${plain}&flag=quantumultx&list=1`)
  })

  it('base64Url is UTF-8 safe', () => {
    expect(base64Url('猪猪侠')).toBe(
      btoa(String.fromCharCode(...new TextEncoder().encode('猪猪侠'))).replace(/=+$/, ''),
    )
  })

  it('lists every one-click client once', () => {
    expect(new Set(IMPORT_CLIENTS.map((c) => c.id)).size).toBe(IMPORT_CLIENTS.length)
  })
})

describe('withSubscribeQuery', () => {
  it('returns the URL untouched without overrides', () => {
    expect(withSubscribeQuery(URL_)).toBe(URL_)
    expect(withSubscribeQuery(URL_, { profile: '' })).toBe(URL_)
  })

  it('adds/replaces override params and keeps the rest', () => {
    const out = new URL(
      withSubscribeQuery(URL_, { profile: 'minimal', cats: ['openai', 'telegram'], mirror: 1 }),
    )
    expect(out.searchParams.get('profile')).toBe('minimal')
    expect(out.searchParams.get('cats')).toBe('openai,telegram')
    expect(out.searchParams.get('mirror')).toBe('1')
    expect(out.searchParams.get('flag')).toBe('x')
    expect(withSubscribeQuery(`${URL_}&profile=full`, { profile: 'none' })).toContain('profile=none')
  })

  it('keeps relative short links relative', () => {
    expect(withSubscribeQuery('/s/abc', { profile: 'standard' })).toBe('/s/abc?profile=standard')
  })
})
