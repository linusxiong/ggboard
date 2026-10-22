/** Splits typed/pasted text on commas, semicolons (ASCII + full-width) and newlines. */
export function splitTagText(text: string): string[] {
  return text
    .split(/[,;\n\r，；、]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}
