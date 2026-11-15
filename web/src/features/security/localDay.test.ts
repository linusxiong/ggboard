import { afterEach, describe, expect, it } from 'vitest'
import { localDay } from './localDay'

describe('localDay', () => {
  const zone = process.env.TZ
  afterEach(() => {
    if (zone === undefined) delete process.env.TZ
    else process.env.TZ = zone
  })

  it('is the calendar day where the viewer is, not the UTC day', () => {
    // 01:30 on the 15th in Shanghai is still the 14th in UTC: the backup-codes file used to say the 14th.
    const instant = new Date('2026-11-14T17:30:00Z')
    process.env.TZ = 'Asia/Shanghai'
    expect(localDay(instant)).toBe('2026-11-15')
    process.env.TZ = 'America/Los_Angeles'
    expect(localDay(instant)).toBe('2026-11-14')
    expect(localDay(new Date('2026-01-05T07:59:00Z'))).toBe('2026-01-04')
  })
})
