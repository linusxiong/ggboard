import { describe, expect, it } from 'vitest'
import {
  checkMatch,
  goRegexpError,
  normalizeMatchText,
  parseMatchList,
  routeToValues,
  sogaOnlyPrefixes,
  valuesToRouteInput,
} from './routeForm'

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

  describe('pasted lists', () => {
    const want = ['geosite:falungong', 'geosite:boxun', 'domain:rfa.org', 'domain:landofhope.tv']
    const cases: Record<string, string> = {
      plain: 'geosite:falungong\ngeosite:boxun\ndomain:rfa.org\ndomain:landofhope.tv',
      'quotes and commas': '"geosite:falungong",\n"geosite:boxun",\n"domain:rfa.org",\n"domain:landofhope.tv"',
      'single quotes': "'geosite:falungong',\n'geosite:boxun',\n'domain:rfa.org',\n'domain:landofhope.tv'",
      'curly quotes': '“geosite:falungong”，\n“geosite:boxun”,\n“domain:rfa.org”,\n“domain:landofhope.tv”',
      'commas only': 'geosite:falungong,\ngeosite:boxun,\ndomain:rfa.org,\ndomain:landofhope.tv',
      'mixed + CRLF + indent': '\t"geosite:falungong" ,\r\n geosite:boxun,\r\n"domain:rfa.org"\r\n\r\ndomain:landofhope.tv,',
      'one line': '"geosite:falungong", "geosite:boxun","domain:rfa.org" ,"domain:landofhope.tv"',
      'json array': '[\n  "geosite:falungong",\n  "geosite:boxun",\n  "domain:rfa.org",\n  "domain:landofhope.tv"\n]',
      'json array on one line': '["geosite:falungong","geosite:boxun","domain:rfa.org","domain:landofhope.tv"]',
      'repeated paste': 'geosite:falungong\n"geosite:boxun",\n"domain:rfa.org",\ndomain:landofhope.tv\n"geosite:falungong",',
    }
    for (const [name, text] of Object.entries(cases)) {
      it(`reads ${name}`, () => {
        expect(parseMatchList(text, 'block')).toEqual(want)
        expect(parseMatchList(text, 'dns')).toEqual(want)
        expect(normalizeMatchText(text, 'block')).toBe(want.join('\n'))
        expect(checkMatch(text, 'block')).toBeUndefined()
      })
    }

    it('keeps regexps, apostrophes and character classes as typed', () => {
      const regexps = ['(.*\\.)?doubleclick\\.net', '[a-z]+\\.example\\.com', '[abc]', 'a{1,3}\\.example\\.org', "it's.example.com"]
      expect(parseMatchList(regexps.join('\n'), 'block')).toEqual(regexps)
      expect(parseMatchList('[]', 'block')).toEqual([])
    })

    it('leaves raw DNS JSON alone', () => {
      const raw = 'main\n{\n"servers": [\n"1.1.1.1",\n"8.8.8.8"\n]\n}'
      expect(parseMatchList(raw, 'dns')).toEqual(['main', '{', '"servers": [', '"1.1.1.1",', '"8.8.8.8"', ']', '}'])
      expect(parseMatchList('main\n"a.com",', 'block')).toEqual(['main', 'a.com'])
    })

    it('sends the cleaned list to the API', () => {
      expect(
        valuesToRouteInput({ remarks: '', match: '"domain:rfa.org",\n"domain:rsf.org"', action: 'block', action_value: '' })
          .match,
      ).toEqual(['domain:rfa.org', 'domain:rsf.org'])
    })
  })

  it('finds the prefixes only soga understands', () => {
    expect(sogaOnlyPrefixes(['(.*\\.)?example\\.com', 'regexp:.*\\.cn$', 'protocol:bittorrent', 'ads.example.com'])).toEqual([])
    expect(sogaOnlyPrefixes(['domain:rfa.org', 'geosite:bbc', 'GeoIP:cn', 'domain:rsf.org'])).toEqual([
      'geosite:',
      'geoip:',
      'domain:',
    ])
    expect(sogaOnlyPrefixes(['full:a.com', 'ip:10.0.0.0/8', 'port:443'])).toEqual(['full:', 'ip:', 'port:'])
  })
})
