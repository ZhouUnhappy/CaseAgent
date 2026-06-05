import client from './client'

export function listTenants(params = {}) {
  return client.get('/tenants', { params }).then((res) => res.data)
}

export function createTenant(payload) {
  return client.post('/tenants', payload).then((res) => res.data)
}

export function updateTenant(slug, payload) {
  return client.put(`/tenants/${slug}`, payload).then((res) => res.data)
}

export function archiveTenant(slug) {
  return client.post(`/tenants/${slug}/archive`, {}).then((res) => res.data)
}

export function unarchiveTenant(slug) {
  return client.post(`/tenants/${slug}/unarchive`, {}).then((res) => res.data)
}
