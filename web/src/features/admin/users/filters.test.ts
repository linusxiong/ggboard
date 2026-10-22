import { describe, expect, it } from 'vitest'
import { buildQuery } from '@/lib/api/client'
import { activeFilterCount, clearFilters, parseUserSearch, toFilterParams, toUserFilters } from './filters'

describe('users URL search → API query', () => {
  it('sanitises raw search params', () => {
    expect(parseUserSearch({})).toEqual({ page: 1, page_size: 20 })
    expect(
      parseUserSearch({
        page: '3',
        page_size: 500,
        sort: '-balance; drop',
        email: '  a@example.com ',
        plan_id: '2',
        group_id: 'x',
        banned: 'true',
        role: 'root',
        expired: 'never',
        invite_user_id: 7,
        id: '-1',
        uuid: '3b1b6f5e-7c2a-4d8e-9f10-2a3b4c5d6e7f',
        remarks: '',
        unknown: 'dropped',
      }),
    ).toEqual({
      page: 3,
      page_size: 20,
      email: 'a@example.com',
      plan_id: 2,
      banned: true,
      expired: 'never',
      invite_user_id: 7,
      uuid: '3b1b6f5e-7c2a-4d8e-9f10-2a3b4c5d6e7f',
    })
    expect(parseUserSearch({ sort: '-balance' }).sort).toBe('-balance')
    expect(parseUserSearch({ sort: 'password_hash' }).sort).toBeUndefined()
    expect(parseUserSearch({ banned: false }).banned).toBe(false)
  })

  it.each([
    [{ page: 1, page_size: 20 }, '?page=1&page_size=20'],
    [
      { page: 2, page_size: 50, sort: '-created_at', email: 'a@example.com', banned: true },
      '?page=2&page_size=50&sort=-created_at&email=a%40example.com&banned=true',
    ],
    [{ page: 1, page_size: 20, role: 'admin' as const }, '?page=1&page_size=20&is_admin=true'],
    [{ page: 1, page_size: 20, role: 'staff' as const }, '?page=1&page_size=20&is_staff=true'],
    [{ page: 1, page_size: 20, role: 'user' as const }, '?page=1&page_size=20&is_admin=false&is_staff=false'],
    [
      {
        page: 1,
        page_size: 20,
        plan_id: 2,
        group_id: 1,
        expired: 'expired' as const,
        invite_user_id: 9,
        id: 4,
        token: 'abc',
        remarks: 'vip',
      },
      '?page=1&page_size=20&plan_id=2&group_id=1&expired=expired&invite_user_id=9&id=4&token=abc&remarks=vip',
    ],
  ])('%j → %s', (search, query) => {
    expect(buildQuery(toUserFilters(search))).toBe(query)
  })

  it('export / mail filters drop paging and sort', () => {
    const search = { page: 4, page_size: 100, sort: 'id', email: 'x', banned: false, role: 'user' as const }
    expect(toFilterParams(search)).toEqual({ email: 'x', banned: false, is_admin: false, is_staff: false })
    expect(activeFilterCount(search)).toBe(3)
    expect(clearFilters(search)).toEqual({ page: 1, page_size: 100, sort: 'id' })
  })
})
