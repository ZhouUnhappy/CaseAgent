import client from './client'

export function listKnowledge(type) {
  const params = type ? { type } : {}
  return client.get('/knowledge', { params }).then((r) => r.data)
}

export function getKnowledge(id) {
  return client.get(`/knowledge/${id}`).then((r) => r.data)
}

export function createKnowledge(payload) {
  return client.post('/knowledge', payload).then((r) => r.data)
}

export function updateKnowledge(id, payload) {
  return client.put(`/knowledge/${id}`, payload).then((r) => r.data)
}

export function reprocessKnowledge(id) {
  return client.post(`/knowledge/${id}/reprocess`).then((r) => r.data)
}

export function deleteKnowledge(id) {
  return client.delete(`/knowledge/${id}`).then((r) => r.data)
}
