import client from './client'

export function listJobs(params = {}) {
  return client.get('/jobs', { params }).then((r) => r.data)
}
