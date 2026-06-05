import client from './client'

export function listJobs(params = {}) {
  return client.get('/jobs', { params }).then((r) => r.data)
}

export function retryJob(id) {
  return client.post(`/jobs/${id}/retry`, {}).then((r) => r.data)
}

export function cancelJob(id) {
  return client.post(`/jobs/${id}/cancel`, {}).then((r) => r.data)
}

export function replayJob(id) {
  return client.post(`/jobs/${id}/replay`, {}).then((r) => r.data)
}
