import client from './client'

export function getOpsMetrics(params = {}) {
  return client.get('/ops/metrics', { params }).then((r) => r.data)
}

export function getOpsPreflight() {
  return client.get('/ops/preflight').then((r) => r.data)
}

export function getFeedbackSummary(params = {}) {
  return client.get('/ops/feedback-summary', { params }).then((r) => r.data)
}

export function getQualityOverview(params = {}) {
  return client.get('/ops/quality', { params }).then((r) => r.data)
}
