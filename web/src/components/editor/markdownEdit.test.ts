import { describe, expect, it } from 'vitest'
import { insertText, prefixLines, wrapSelection } from './markdownEdit'

describe('markdown edits', () => {
  it('wraps the selection or a placeholder', () => {
    expect(wrapSelection('hello world', 6, 11, '**', '**')).toEqual({
      value: 'hello **world**',
      selectionStart: 8,
      selectionEnd: 13,
    })
    expect(wrapSelection('ab', 1, 1, '*', '*', 'text').value).toBe('a*text*b')
  })

  it('prefixes every selected line', () => {
    expect(prefixLines('one\ntwo\nthree', 0, 7, (i) => `${i + 1}. `).value).toBe('1. one\n2. two\nthree')
    expect(prefixLines('a\nb', 2, 2, () => '> ').value).toBe('a\n> b')
  })

  it('inserts at the caret, replacing the selection', () => {
    expect(insertText('Hi NAME!', 3, 7, '{{siteName}}')).toEqual({
      value: 'Hi {{siteName}}!',
      selectionStart: 15,
      selectionEnd: 15,
    })
  })
})
