import client from './client'

export function listDocuments(projectId) {
  return client.get(`/projects/${projectId}/documents`).then((r) => r.data)
}

export function getDocument(id) {
  return client.get(`/documents/${id}`).then((r) => r.data)
}

export function uploadDocument(projectId, { name, type, source, file, fileId }) {
  const form = new FormData()
  form.append('name', name)
  form.append('type', type)
  form.append('source', source)
  if (file) form.append('file', file)
  if (fileId) form.append('file_id', fileId)
  return client
    .post(`/projects/${projectId}/documents`, form, {
      headers: { 'Content-Type': 'multipart/form-data' },
    })
    .then((r) => r.data)
}

export function reprocessDocument(id) {
  return client.post(`/documents/${id}/reprocess`).then((r) => r.data)
}

export function deleteDocument(id) {
  return client.delete(`/documents/${id}`).then((r) => r.data)
}
