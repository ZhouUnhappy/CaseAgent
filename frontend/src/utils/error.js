import { ElMessage } from 'element-plus'

// notifyApiError surfaces a normalized API error to the user via Element Plus
// message. The shape (status / message / retryable) is set in api/client.js
// normalizeError. Distinguishing retryable vs non-retryable is required for
// I3-T3 (区分可重试 / 不可重试) — keep this behavior even before the
// generation page wires explicit retry buttons.
export function notifyApiError(err) {
  if (!err || !err.message) return
  const type = err.retryable ? 'warning' : 'error'
  const prefix = err.status > 0 ? `[${err.status}] ` : ''
  ElMessage({
    type,
    message: prefix + err.message,
    showClose: true,
    duration: err.retryable ? 4000 : 6000,
  })
}

// notifySuccess is a thin convenience wrapper so views don't import ElMessage
// directly. Keeping the abstraction makes it cheap to swap the surface later
// (e.g. toast vs notification center).
export function notifySuccess(message) {
  ElMessage({ type: 'success', message, duration: 2500 })
}
