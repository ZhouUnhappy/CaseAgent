import { defineStore } from 'pinia'
import {
  listKnowledge,
  createKnowledge,
  updateKnowledge,
  reprocessKnowledge,
  deleteKnowledge,
} from '../api/knowledge'

export const useKnowledgeStore = defineStore('knowledge', {
  state: () => ({
    items: [],
    loading: false,
    saving: false,
    typeFilter: '',
    sourceFilter: '',
    expiredFilter: '',
    duplicateFilter: '',
  }),
  actions: {
    async fetch() {
      this.loading = true
      try {
        this.items = await listKnowledge({
          type: this.typeFilter || undefined,
          source: this.sourceFilter || undefined,
          expired: this.expiredFilter || undefined,
          duplicate: this.duplicateFilter || undefined,
        })
      } finally {
        this.loading = false
      }
    },
    setTypeFilter(t) {
      this.typeFilter = t
      return this.fetch()
    },
    setFilters(filters) {
      Object.assign(this, filters)
      return this.fetch()
    },
    async create(payload) {
      this.saving = true
      try {
        const created = await createKnowledge(payload)
        this.items = [created, ...this.items]
        return created
      } finally {
        this.saving = false
      }
    },
    async update(id, payload) {
      this.saving = true
      try {
        const updated = await updateKnowledge(id, payload)
        this.replace(updated)
        return updated
      } finally {
        this.saving = false
      }
    },
    async reprocess(id) {
      const updated = await reprocessKnowledge(id)
      this.replace(updated)
      return updated
    },
    async remove(id) {
      await deleteKnowledge(id)
      this.items = this.items.filter((k) => k.id !== id)
    },
    clear() {
      this.items = []
      this.loading = false
      this.saving = false
      this.typeFilter = ''
      this.sourceFilter = ''
      this.expiredFilter = ''
      this.duplicateFilter = ''
    },
    replace(updated) {
      const idx = this.items.findIndex((k) => k.id === updated.id)
      if (idx >= 0) this.items.splice(idx, 1, updated)
    },
  },
})
