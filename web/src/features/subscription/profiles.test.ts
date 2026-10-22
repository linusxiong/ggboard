import { describe, expect, it } from 'vitest'
import type { RuleProfileSummary, UserSubscription } from '@/lib/api/types'
import { ageClients, ageSnippet, clientLabel, recentIpCount, sortAccess } from './accessLog'
import {
  orderProfiles,
  pickActive,
  profileIssues,
  profileUrl,
  slugify,
  suggestSlug,
  VIRTUAL_DEFAULT,
} from './profiles'

const p = (slug: string, name: string, is_default = false, id = 1): RuleProfileSummary => ({
  id,
  slug,
  name,
  is_default,
  updated_at: '2026-10-22T00:00:00Z',
})
const LIST = [p('work', 'Work', false, 2), p('default', '默认', true, 1), p('game', 'Game', false, 3)]
const TEMPLATES = ['none', 'minimal', 'standard', 'full']

describe('subscription profiles', () => {
  it('puts the default first and falls back to a virtual default', () => {
    expect(orderProfiles(LIST).map((x) => x.slug)).toEqual(['default', 'work', 'game'])
    expect(orderProfiles([])).toEqual([VIRTUAL_DEFAULT])
    expect(orderProfiles(undefined)).toEqual([VIRTUAL_DEFAULT])
    expect(pickActive(orderProfiles(LIST), 'game').slug).toBe('game')
    expect(pickActive(orderProfiles(LIST), 'gone').slug).toBe('default')
  })

  it('builds per-profile links (server link first, else ?profile=<slug>)', () => {
    const sub = { subscribe_url: 'https://panel.example.com/s/tok' } as UserSubscription
    expect(profileUrl(sub, LIST[1]!)).toBe('https://panel.example.com/s/tok')
    expect(profileUrl(sub, LIST[0]!)).toBe('https://panel.example.com/s/tok?profile=work')
    const listed = {
      ...sub,
      profiles: [{ slug: 'work', name: 'Work', subscribe_url: 'https://cdn.example.com/s/tok?profile=work' }],
    }
    expect(profileUrl(listed, LIST[0]!)).toBe('https://cdn.example.com/s/tok?profile=work')
  })

  it('suggests unique slugs that never shadow a template id', () => {
    expect(slugify('My Phone (iOS)')).toBe('my-phone-ios')
    expect(slugify('游戏')).toBe('')
    expect(suggestSlug('Work', LIST, TEMPLATES)).toBe('work-2')
    expect(suggestSlug('Standard', LIST, TEMPLATES)).toBe('standard-2')
    expect(suggestSlug('Travel', LIST, TEMPLATES)).toBe('travel')
  })

  it('validates names and slugs', () => {
    expect(profileIssues({ name: ' ', slug: 'Bad Slug' }, LIST, TEMPLATES)).toEqual({
      name: 'nameRequired',
      slug: 'slugFormat',
    })
    expect(profileIssues({ name: 'x'.repeat(65) }, LIST, TEMPLATES)).toEqual({ name: 'nameTooLong' })
    expect(profileIssues({ name: 'A', slug: 'full' }, LIST, TEMPLATES)).toEqual({ slug: 'slugReserved' })
    expect(profileIssues({ name: 'A', slug: 'work' }, LIST, TEMPLATES)).toEqual({ slug: 'slugTaken' })
    expect(profileIssues({ name: 'A', slug: 'work' }, LIST, TEMPLATES, LIST[0])).toEqual({})
    expect(profileIssues({ name: 'A', slug: 'trip-' }, LIST, TEMPLATES)).toEqual({}) // server: ^[a-z0-9][a-z0-9_-]{0,31}$
    expect(profileIssues({ name: 'A', slug: '-trip' }, LIST, TEMPLATES)).toEqual({ slug: 'slugFormat' })
  })
})

describe('access log helpers', () => {
  const log = [
    { ip: '203.0.113.*', client: 'clash-verge/v2.2.3', format: 'clash-meta', at: '2026-10-21T10:00:00Z' },
    { ip: '198.51.100.*', client: '', format: 'v2ray', at: '2026-10-22T09:00:00Z' },
    { ip: '203.0.113.*', client: 'mihomo', format: 'clash-meta', at: '2026-10-22T08:00:00Z' },
  ]
  it('sorts newest first and counts distinct IPs in the last 24 h', () => {
    expect(sortAccess(log).map((e) => e.at)).toEqual([
      '2026-10-22T09:00:00Z',
      '2026-10-22T08:00:00Z',
      '2026-10-21T10:00:00Z',
    ])
    expect(recentIpCount(log, Date.parse('2026-10-22T10:00:00Z'))).toBe(2)
    expect(recentIpCount(log, Date.parse('2026-10-23T08:30:00Z'))).toBe(1)
    expect(clientLabel('x'.repeat(60))).toHaveLength(48)
    expect(clientLabel('unknown')).toBe('')
  })

  it('lists the formats served encrypted (default: mihomo only)', () => {
    expect(ageClients(undefined)).toEqual(['Clash Meta (mihomo)'])
    expect(ageClients(['clash-meta'])).toEqual(['Clash Meta (mihomo)'])
    expect(
      ageClients(undefined, [
        { id: 'clash-meta', name: 'Clash Meta', supports_age: true },
        { id: 'singbox', name: 'sing-box', supports_age: false },
      ]),
    ).toEqual(['Clash Meta'])
    expect(ageSnippet('https://panel.example.com/s/tok')).toContain('url: "https://panel.example.com/s/tok"')
    expect(ageSnippet(undefined)).toContain('age-secret-key: AGE-SECRET-KEY-1…')
  })
})
