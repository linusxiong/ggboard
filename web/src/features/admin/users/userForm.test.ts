import { describe, expect, it } from 'vitest'
import type { AdminUser } from '@/lib/api/types'
import { createValuesToInput, EMPTY_CREATE, NONE, userToValues, valuesToPatch } from './userForm'

const GB = 1024 ** 3

const user: AdminUser = {
  id: 7,
  email: 'a@example.com',
  uuid: '3b1b6f5e-7c2a-4d8e-9f10-2a3b4c5d6e7f',
  plan_id: 2,
  plan: { id: 2, name: 'Pro' },
  group_id: 1,
  transfer_enable: 100 * GB,
  u: 1.5 * GB,
  d: 3 * GB,
  expired_at: '2027-01-01T00:00:00Z',
  next_reset_at: null,
  speed_limit: null,
  device_limit: 3,
  balance: 1250,
  commission_balance: 0,
  commission_rate: null,
  discount: null,
  remind_expire: true,
  remind_traffic: true,
  is_admin: false,
  is_staff: false,
  banned: false,
  avatar_url: '',
  created_at: '2026-01-01T00:00:00Z',
  last_login_at: null,
  remarks: '',
  invite_user_id: null,
  last_login_ip: '',
  telegram_id: null,
  commission_type: 0,
}

describe('user edit form payload', () => {
  it('maps the user into form units (GB, currency units)', () => {
    const v = userToValues(user, 'CNY')
    expect(v).toMatchObject({
      transfer_enable: 100,
      u: 1.5,
      d: 3,
      balance: 12.5,
      plan_id: '2',
      group_id: '1',
      password: '',
    })
    expect(v.expired_at).toMatch(/^2027-01-01T\d\d:\d\d$|^2026-12-31T\d\d:\d\d$/)
  })

  it('sends nothing when nothing changed', () => {
    const v = userToValues(user, 'CNY')
    expect(valuesToPatch(v, v, 'CNY')).toEqual({})
  })

  it('sends only changed fields, converting GB → bytes and units → cents', () => {
    const initial = userToValues(user, 'CNY')
    const patch = valuesToPatch(
      {
        ...initial,
        transfer_enable: 200,
        d: 0,
        balance: 99.99,
        speed_limit: 100,
        device_limit: null,
        plan_id: NONE,
        expired_at: '',
        remarks: 'vip',
        banned: true,
        password: 'new-password-1',
      },
      initial,
      'CNY',
    )
    expect(patch).toEqual({
      password: 'new-password-1',
      plan_id: null,
      transfer_enable: 200 * GB,
      d: 0,
      expired_at: null,
      speed_limit: 100,
      device_limit: null,
      balance: 9999,
      remarks: 'vip',
      banned: true,
    })
    expect('u' in patch).toBe(false) // untouched traffic is never overwritten
  })

  it('handles fractional GB and zero-decimal currencies', () => {
    const initial = userToValues(user, 'JPY')
    expect(initial.balance).toBe(1250)
    const patch = valuesToPatch({ ...initial, transfer_enable: 0.5, balance: 3000 }, initial, 'JPY')
    expect(patch).toEqual({ transfer_enable: GB / 2, balance: 3000 })
  })

  it('create payload keeps GB (POST /admin/users contract) and omits empty optionals', () => {
    expect(
      createValuesToInput({ ...EMPTY_CREATE, email: ' b@example.com ', password: 'password-123' }, 'CNY'),
    ).toEqual({
      email: 'b@example.com',
      password: 'password-123',
    })
    expect(
      createValuesToInput(
        {
          ...EMPTY_CREATE,
          email: 'c@example.com',
          password: 'password-123',
          plan_id: '2',
          group_id: '1',
          transfer_enable: 50,
          balance: 10,
          is_staff: true,
          remarks: ' note ',
        },
        'CNY',
      ),
    ).toEqual({
      email: 'c@example.com',
      password: 'password-123',
      plan_id: 2,
      group_id: 1,
      transfer_enable: 50,
      balance: 1000,
      is_staff: true,
      remarks: 'note',
    })
  })
})
