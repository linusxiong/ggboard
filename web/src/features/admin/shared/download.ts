/** Save a Blob / text as a file (object URL + temporary link). */
export function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const a = document.createElement('a')
  a.href = url
  a.download = filename
  a.rel = 'noopener'
  document.body.appendChild(a)
  a.click()
  a.remove()
  window.setTimeout(() => URL.revokeObjectURL(url), 1000)
}

export function downloadText(text: string, filename: string, type = 'text/csv;charset=utf-8') {
  // BOM so spreadsheet apps detect UTF-8 (Chinese e-mail remarks etc.).
  downloadBlob(new Blob(['﻿', text], { type }), filename)
}

/** `users-20261022-1530.csv` */
export function stampedName(prefix: string, ext: string, now = new Date()): string {
  const pad = (n: number) => String(n).padStart(2, '0')
  const stamp = `${now.getFullYear()}${pad(now.getMonth() + 1)}${pad(now.getDate())}-${pad(now.getHours())}${pad(now.getMinutes())}`
  return `${prefix}-${stamp}.${ext}`
}
