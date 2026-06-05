import { defineStore } from 'pinia'
import {
  archiveTenant,
  createTenant,
  listTenants,
  unarchiveTenant,
  updateTenant,
} from '../api/tenants'
import { clearTenantScopedStores } from './tenantScope'

const STORAGE_KEY = 'caseagent.tenant_slug'
const DEFAULT_STORAGE_KEY = 'caseagent.default_tenant_slug'

export const useTenantStore = defineStore('tenant', {
  state: () => ({
    currentSlug: localStorage.getItem(STORAGE_KEY) || '',
    defaultSlug: localStorage.getItem(DEFAULT_STORAGE_KEY) || '',
    items: [],
    loading: false,
    creating: false,
    savingSlug: '',
    version: 0,
  }),
  getters: {
    hasCurrent: (state) => state.currentSlug !== '',
    activeItems: (state) => state.items.filter((t) => !t.archived_at),
    current: (state) => state.items.find((t) => t.slug === state.currentSlug && !t.archived_at) || null,
    defaultTenant: (state) => state.items.find((t) => t.slug === state.defaultSlug && !t.archived_at) || null,
  },
  actions: {
    setCurrent(slug) {
      if (slug === this.currentSlug) return
      clearTenantScopedStores()
      this.currentSlug = slug
      this.version += 1
      if (slug) {
        localStorage.setItem(STORAGE_KEY, slug)
      } else {
        localStorage.removeItem(STORAGE_KEY)
      }
    },
    setDefault(slug) {
      this.defaultSlug = slug
      if (slug) {
        localStorage.setItem(DEFAULT_STORAGE_KEY, slug)
      } else {
        localStorage.removeItem(DEFAULT_STORAGE_KEY)
      }
    },
    async fetch(options = {}) {
      this.loading = true
      try {
        this.items = await listTenants({ include_archived: options.includeArchived ? 'true' : undefined })
        const active = this.activeItems
        if (this.defaultSlug && !active.some((t) => t.slug === this.defaultSlug)) {
          this.setDefault('')
        }
        // Drop stale local slug if the tenant has been deleted or archived server-side.
        if (this.currentSlug && !active.some((t) => t.slug === this.currentSlug)) {
          this.setCurrent('')
        }
        // Auto-pick first tenant when none is selected, so users aren't
        // greeted by a 400 from the next data request.
        if (!this.currentSlug && active.length > 0) {
          const preferred = active.find((t) => t.slug === this.defaultSlug) || active[0]
          this.setCurrent(preferred.slug)
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
    async update(slug, payload) {
      this.savingSlug = slug
      try {
        const updated = await updateTenant(slug, payload)
        this.replace(updated)
        return updated
      } finally {
        this.savingSlug = ''
      }
    },
    async archive(slug) {
      this.savingSlug = slug
      try {
        const updated = await archiveTenant(slug)
        this.replace(updated)
        if (this.currentSlug === slug) {
          const next = this.activeItems.find((t) => t.slug !== slug)
          this.setCurrent(next?.slug || '')
        }
        if (this.defaultSlug === slug) {
          this.setDefault('')
        }
        return updated
      } finally {
        this.savingSlug = ''
      }
    },
    async unarchive(slug) {
      this.savingSlug = slug
      try {
        const updated = await unarchiveTenant(slug)
        this.replace(updated)
        return updated
      } finally {
        this.savingSlug = ''
      }
    },
    replace(updated) {
      const idx = this.items.findIndex((t) => t.slug === updated.slug)
      if (idx >= 0) {
        this.items.splice(idx, 1, updated)
      } else {
        this.items = [updated, ...this.items]
      }
    },
  },
})
