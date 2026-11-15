/** The parts of a React (or React Aria) keyboard event the check reads. */
interface KeyEventLike {
  keyCode: number
  nativeEvent: { isComposing: boolean }
}

/**
 * `true` when this Enter `keydown` belongs to an IME (Chinese, Japanese, Korean input): it picks a candidate,
 * so it must not submit, add or rename. Call it only once the key is known to be Enter.
 * `keyCode` 229 covers Safari, which ends the composition before that keydown and reports `isComposing: false`
 * (WebKit bug 165004). A handler that needs Ctrl/Cmd+Enter does not need this.
 */
export function isImeEnter(e: KeyEventLike): boolean {
  return e.nativeEvent.isComposing || e.keyCode === 229
}
