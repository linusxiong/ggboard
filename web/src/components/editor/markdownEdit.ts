/** Pure text edits used by `MarkdownEditor` toolbar actions (testable without a DOM). */

export interface EditResult {
  value: string
  selectionStart: number
  selectionEnd: number
}

/** Wraps the selection with `before`/`after` (or inserts `placeholder` wrapped and selects it). */
export function wrapSelection(
  value: string,
  start: number,
  end: number,
  before: string,
  after: string,
  placeholder = '',
): EditResult {
  const selected = value.slice(start, end) || placeholder
  const next = value.slice(0, start) + before + selected + after + value.slice(end)
  return {
    value: next,
    selectionStart: start + before.length,
    selectionEnd: start + before.length + selected.length,
  }
}

/** Prefixes every line touched by the selection (lists, quotes, headings). */
export function prefixLines(
  value: string,
  start: number,
  end: number,
  prefix: (i: number) => string,
): EditResult {
  const lineStart = value.lastIndexOf('\n', start - 1) + 1
  const lineEndIdx = value.indexOf('\n', end)
  const lineEnd = lineEndIdx === -1 ? value.length : lineEndIdx
  const block = value.slice(lineStart, lineEnd)
  const lines = block.split('\n').map((line, i) => prefix(i) + line)
  const replaced = lines.join('\n')
  return {
    value: value.slice(0, lineStart) + replaced + value.slice(lineEnd),
    selectionStart: lineStart,
    selectionEnd: lineStart + replaced.length,
  }
}

/** Inserts `text` at the cursor, replacing the selection; the cursor ends after the insertion. */
export function insertText(value: string, start: number, end: number, text: string): EditResult {
  const next = value.slice(0, start) + text + value.slice(end)
  const caret = start + text.length
  return { value: next, selectionStart: caret, selectionEnd: caret }
}
