import { describe, expect, it } from 'vitest'
import type { Node } from '@/lib/api/types'
import { tagSuggestions } from './nodeForm'

const withTags = (...tags: string[]) => ({ tags }) as Node

describe('tagSuggestions', () => {
  it('lists each tag once, most used first, ties in first-seen order', () => {
    const nodes = [withTags('b', 'a'), withTags('a'), withTags('c', 'a', 'c')]
    expect(tagSuggestions(nodes)).toEqual(['a', 'c', 'b'])
  })

  it('stops at the limit', () => {
    expect(tagSuggestions([withTags('a', 'b', 'c')], 2)).toEqual(['a', 'b'])
  })
})
