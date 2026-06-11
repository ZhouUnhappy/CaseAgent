import client from './client'

export function listTasks(projectId) {
  return client.get(`/projects/${projectId}/tasks`).then((r) => r.data)
}

export function createTask(projectId, payload) {
  return client.post(`/projects/${projectId}/tasks`, payload).then((r) => r.data)
}

export function getTask(id) {
  return client.get(`/tasks/${id}`).then((r) => r.data)
}

export function getTaskTrace(id) {
  return client.get(`/tasks/${id}/trace`).then((r) => r.data)
}

export function getTaskDiagnostics(id) {
  return client.get(`/tasks/${id}/diagnostics`).then((r) => r.data)
}

export function reviewAffected(id, payload) {
  return client.put(`/tasks/${id}/review`, payload).then((r) => r.data)
}

export function generateCases(id) {
  return client.put(`/tasks/${id}/generate`, {}).then((r) => r.data)
}

export function retryTask(id) {
  return client.post(`/tasks/${id}/retry`, {}).then((r) => r.data)
}
