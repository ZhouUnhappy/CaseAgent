import client from './client'

export function listKnowledgeSuggestions(status) {
  const params = status ? { status } : {}
  return client.get('/knowledge-suggestions', { params }).then((r) => r.data)
}

export function createKnowledgeSuggestion(payload) {
  return client.post('/knowledge-suggestions', payload).then((r) => r.data)
}

export function draftKnowledgeSuggestion(id) {
  return client.post(`/knowledge-suggestions/${id}/draft`).then((r) => r.data)
}

export function updateKnowledgeSuggestion(id, status, resolvedKnowledgeId) {
  const payload = { status }
  if (resolvedKnowledgeId) payload.resolved_knowledge_id = resolvedKnowledgeId
  return client.put(`/knowledge-suggestions/${id}`, payload).then((r) => r.data)
}
