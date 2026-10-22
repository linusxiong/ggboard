/** Splits pasted/typed text on commas, semicolons and newlines. */
export function splitTags(text: string): string[] {
  return text
    .split(/[,;\n\r，；]+/)
    .map((s) => s.trim())
    .filter(Boolean)
}
