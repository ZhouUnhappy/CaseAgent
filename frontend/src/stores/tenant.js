import { defineStore } from 'pinia'
import { listTenants, createTenant } from '../api/tenants'

const STORAGE_KEY = 'caseagent.tenant_slug'

export const useTenantStore = defineStore('tenant', {
  state: () => ({
    currentSlug: localStorage.getItem(STORAGE_KEY) || '',
    items: [],
    loading: false,
    creating: false,
  }),
  getters: {
    hasCurrent: (state) => state.currentSlug !== '',
    current: (state) => state.items.find((t) => t.slug === state.currentSlug) || null,
  },
  actions: {
    setCurrent(slug) {
      this.currentSlug = slug
      if (slug) {
        localStorage.setItem(STORAGE_KEY, slug)
      } else {
        localStorage.removeItem(STORAGE_KEY)
      }
    },
    async fetch() {
      this.loading = true
      try {
        this.items = await listTenants()
        // Drop stale local slug if the tenant has been deleted server-side.
        if (this.currentSlug && !this.items.some((t) => t.slug === this.currentSlug)) {
          this.setCurrent('')
        }
        // Auto-pick first tenant when none is selected, so users aren't
        // greeted by a 400 from the next data request.
        if (!this.currentSlug && this.items.length > 0) {
          this.setCurrent(this.items[0].slug)
        }
      } finally {
        this.loading = false
      }
    },
    async create(payload) {
      this.creating = true
      try {
        const created = await createTenant(payload)
        this.items = [...this.items, created]
        return created
      } finally {
        this.creating = false
      }
    },
  },
})
