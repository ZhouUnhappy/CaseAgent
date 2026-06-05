import client from './client'

export function listWorkflows(params = {}) {
  return client.get('/workflows', { params }).then((r) => r.data)
}
