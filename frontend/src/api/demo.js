import client from './client'

function withFrontendURL(payload = {}) {
  return {
    frontend_url: window.location.origin,
    ...payload,
  }
}

export function resetDemo() {
  return client.post('/demo/reset', {}).then((r) => r.data)
}

export function bootstrapDemo(payload = {}) {
  return client.post('/demo/bootstrap', withFrontendURL(payload)).then((r) => r.data)
}

export function freshDemo(payload = {}) {
  return client.post('/demo/fresh', withFrontendURL(payload)).then((r) => r.data)
}
