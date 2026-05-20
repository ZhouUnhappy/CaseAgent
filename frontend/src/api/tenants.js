import client from './client'

export function listTenants() {
  return client.get('/tenants').then((res) => res.data)
}

export function createTenant(payload) {
  return client.post('/tenants', payload).then((res) => res.data)
}
