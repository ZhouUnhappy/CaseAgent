import client from './client'

export function getOpsMetrics(params = {}) {
  return client.get('/ops/metrics', { params }).then((r) => r.data)
}
