import client from './client'

export function listJobs(params = {}) {
  return client.get('/jobs', { params }).then((r) => r.data)
}

export function retryJob(id, payload = {}) {
  return client.post(`/jobs/${id}/retry`, payload).then((r) => r.data)
}

export function cancelJob(id, payload = {}) {
  return client.post(`/jobs/${id}/cancel`, payload).then((r) => r.data)
}

export function replayJob(id, payload = {}) {
  return client.post(`/jobs/${id}/replay`, payload).then((r) => r.data)
}
