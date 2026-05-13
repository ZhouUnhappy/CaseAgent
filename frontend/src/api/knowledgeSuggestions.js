import client from './client'

export function listKnowledgeSuggestions(status) {
  const params = status ? { status } : {}
  return client.get('/knowledge-suggestions', { params }).then((r) => r.data)
}

export function updateKnowledgeSuggestion(id, status) {
  return client.put(`/knowledge-suggestions/${id}`, { status }).then((r) => r.data)
}
