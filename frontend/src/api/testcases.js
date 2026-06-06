import client from './client'

export function listTestCases(taskId) {
  return client.get(`/tasks/${taskId}/cases`).then((r) => r.data)
}

export function updateTestCase(taskId, caseId, payload) {
  return client.put(`/tasks/${taskId}/cases/${caseId}`, payload).then((r) => r.data)
}

export function submitTestCase(taskId, caseId) {
  return client.put(`/tasks/${taskId}/cases/${caseId}/submit`).then((r) => r.data)
}

export function createCaseFeedback(taskId, caseId, payload) {
  return client.post(`/tasks/${taskId}/cases/${caseId}/feedback`, payload).then((r) => r.data)
}

export function listTaskFeedback(taskId) {
  return client.get(`/tasks/${taskId}/feedback`).then((r) => r.data)
}
