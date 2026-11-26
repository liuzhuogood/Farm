/** 双击/双触检测：命中后立即复位，避免连续三击重复触发。 */
export function createDoubleTapDetector(intervalMs: number, onDoubleTap: () => void) {
  let lastTapAt = 0
  return {
    tap(now = Date.now()): boolean {
      if (lastTapAt !== 0 && now - lastTapAt <= intervalMs) {
        lastTapAt = 0
        onDoubleTap()
        return true
      }
      lastTapAt = now
      return false
    },
    reset() {
      lastTapAt = 0
    },
  }
}
