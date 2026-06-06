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

export function batchUpdateTestCases(taskId, payload) {
  return client.put(`/tasks/${taskId}/cases/batch`, payload).then((r) => r.data)
}

export function batchSubmitTestCases(taskId, payload) {
  return client.put(`/tasks/${taskId}/cases/batch/submit`, payload).then((r) => r.data)
}

export function createCaseFeedback(taskId, caseId, payload) {
  return client.post(`/tasks/${taskId}/cases/${caseId}/feedback`, payload).then((r) => r.data)
}

export function listTaskFeedback(taskId) {
  return client.get(`/tasks/${taskId}/feedback`).then((r) => r.data)
}
