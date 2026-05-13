import client from './client'

export function listProjects() {
  return client.get('/projects').then((res) => res.data)
}

export function createProject(payload) {
  return client.post('/projects', payload).then((res) => res.data)
}

export function getProject(id) {
  return client.get(`/projects/${id}`).then((res) => res.data)
}
