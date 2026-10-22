import { describe, expect, it } from 'vitest'
import { checkMatch, goRegexpError, routeToValues, valuesToRouteInput } from './routeForm'

describe('route form', () => {
  it('turns match lines into an array', () => {
    expect(
      valuesToRouteInput({
        remarks: ' ads ',
        match: 'a.com\n\n  b.com  \n',
        action: 'block',
        action_value: '',
      }),
    ).toEqual({
      remarks: 'ads',
      match: ['a.com', 'b.com'],
      action: 'block',
      action_value: '',
    })
    expect(
      routeToValues({
        id: 1,
        remarks: 'x',
        match: ['a', 'b'],
        action: 'dns',
        action_value: '1.1.1.1',
        created_at: '',
        updated_at: '',
      }).match,
    ).toBe('a\nb')
  })

  it('validates block routes as one Go regexp', () => {
    expect(checkMatch('', 'block')).toEqual({ key: 'required' })
    expect(checkMatch('(.*\\.)?example\\.com\ndomain:ads.example.com', 'block')).toBeUndefined()
    expect(checkMatch('(abc', 'block')?.key).toBe('regexp')
    expect(checkMatch('(abc', 'dns')).toBeUndefined() // dns entries are domains, not regexps
    expect(checkMatch(Array.from({ length: 1000 }, (_, i) => `d${i}.com`).join('\n'), 'dns')).toEqual({
      key: 'tooMany',
      max: 999,
    })
  })

  it('accepts Go-only syntax and rejects what RE2 cannot do', () => {
    expect(goRegexpError('(?i)example\\.com')).toBeNull()
    expect(goRegexpError('(?P<host>[a-z]+)\\.com')).toBeNull()
    expect(goRegexpError('foo(?=bar)')).toBe('lookaround')
    expect(goRegexpError('(a)\\1')).toBe('backreference')
  })
})
