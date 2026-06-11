import client from './client'

function cleanParams(params = {}) {
  return Object.fromEntries(
    Object.entries(params).filter(([, value]) => value !== undefined && value !== null && value !== ''),
  )
}

export function listKnowledge(params = {}) {
  return client.get('/knowledge', { params: cleanParams(params) }).then((r) => r.data)
}

export function getKnowledge(id) {
  return client.get(`/knowledge/${id}`).then((r) => r.data)
}

export function listKnowledgeImpactedTasks(id) {
  return client.get(`/knowledge/${id}/impacted-tasks`).then((r) => r.data.items || [])
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
